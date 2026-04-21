// Package client provides the Go client for the AutoTemp library.
//
// AutoTemp performs temperature-grid search over an LLM-like callable and
// selects the temperature whose sampled output scores highest across one
// or more judge functions. The Go port provides a pluggable Runner
// (LLM adapter) and Judges (scoring functions) so that unit tests can
// inject deterministic stand-ins while production deployments wire in
// real LLM providers and structured-output judges.
//
// Basic usage:
//
//	import autotemp "digital.vasic.autotemp/pkg/client"
//
//	c, err := autotemp.New()
//	if err != nil { log.Fatal(err) }
//	defer c.Close()
//
//	c.SetRunner(func(ctx context.Context, prompt string, temp, topP float64) (string, autotemptypes.TokenUsage, error) {
//	    // call your LLM …
//	})
//	res, err := c.Run(ctx, autotemptypes.RunOptions{Prompt: "hello", Temperatures: []float64{0.2, 0.7, 1.0}})
package client

import (
	"context"
	"fmt"
	"sync"

	"digital.vasic.pliniuscommon/pkg/config"
	"digital.vasic.pliniuscommon/pkg/errors"

	. "digital.vasic.autotemp/pkg/types"
)

// Runner generates a completion for the given prompt at the given temperature.
// Production code wires this to a real LLM provider; tests can inject a stub.
type Runner func(ctx context.Context, prompt string, temperature, topP float64) (string, TokenUsage, error)

// Judge scores a candidate output for a prompt. Score is expected in [0,1].
type Judge func(ctx context.Context, prompt, output string) (ScoreBreakdown, error)

// Client is the Go client for AutoTemp.
type Client struct {
	cfg    *config.Config
	mu     sync.RWMutex
	closed bool

	runner Runner
	judges []Judge
}

// New creates a new AutoTemp client with a deterministic baseline runner and
// judge. Tests and consumers can override with SetRunner / SetJudges.
func New(opts ...config.Option) (*Client, error) {
	cfg := config.New("autotemp", opts...)
	if err := cfg.Validate(); err != nil {
		return nil, errors.Wrap(errors.ErrCodeInvalidArgument, "autotemp",
			"invalid configuration", err)
	}
	return &Client{
		cfg:    cfg,
		runner: baselineRunner,
		judges: []Judge{baselineJudge},
	}, nil
}

// NewFromConfig creates a client from a config object.
func NewFromConfig(cfg *config.Config) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Wrap(errors.ErrCodeInvalidArgument, "autotemp",
			"invalid configuration", err)
	}
	return &Client{
		cfg:    cfg,
		runner: baselineRunner,
		judges: []Judge{baselineJudge},
	}, nil
}

// Close gracefully closes the client.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	return nil
}

// Config returns the client configuration.
func (c *Client) Config() *config.Config { return c.cfg }

// SetRunner swaps the LLM runner used by Run / RunAdvanced / Benchmark.
func (c *Client) SetRunner(r Runner) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if r != nil {
		c.runner = r
	}
}

// SetJudges swaps the judge set. Nil/empty input is ignored.
func (c *Client) SetJudges(js ...Judge) {
	if len(js) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.judges = append([]Judge(nil), js...)
}

// defaultTemperatures returns a small grid used when the caller omits one.
func defaultTemperatures() []float64 {
	return []float64{0.1, 0.4, 0.7, 1.0}
}

// Run performs grid search across Temperatures and returns the best scoring output.
func (c *Client) Run(ctx context.Context, opts RunOptions) (*RunResult, error) {
	if err := opts.Validate(); err != nil {
		return nil, errors.Wrap(errors.ErrCodeInvalidArgument, "autotemp",
			"invalid parameters", err)
	}
	opts.Defaults()

	temps := opts.Temperatures
	if len(temps) == 0 {
		temps = defaultTemperatures()
	}

	c.mu.RLock()
	runner := c.runner
	judges := append([]Judge(nil), c.judges...)
	c.mu.RUnlock()

	best := &RunResult{BestOverallScore: -1}
	totalUsage := TokenUsage{}
	for _, t := range temps {
		out, usage, err := runner(ctx, opts.Prompt, t, opts.TopP)
		if err != nil {
			return nil, errors.Wrap(errors.ErrCodeUnavailable, "autotemp",
				"runner failed", err)
		}
		totalUsage.PromptTokens += usage.PromptTokens
		totalUsage.CompletionTokens += usage.CompletionTokens
		totalUsage.TotalTokens += usage.TotalTokens

		score := scoreWithJudges(ctx, judges, opts.Prompt, out)
		if score > best.BestOverallScore {
			best.BestOutput = out
			best.BestTemperature = t
			best.BestOverallScore = score
			best.Summary = fmt.Sprintf("best temp %.2f with score %.3f over %d candidates",
				t, score, len(temps))
		}
	}
	best.Usage = totalUsage
	return best, nil
}

// RunAdvanced performs a UCB-lite multi-round grid search. With the
// baseline runner this degenerates to repeated grid search; the winning
// temperature across rounds is returned.
func (c *Client) RunAdvanced(ctx context.Context, opts AdvancedOptions) (*RunResult, error) {
	if err := opts.Validate(); err != nil {
		return nil, errors.Wrap(errors.ErrCodeInvalidArgument, "autotemp",
			"invalid parameters", err)
	}
	opts.Defaults()
	rounds := opts.Rounds
	if rounds <= 0 {
		rounds = 1
	}
	best := &RunResult{BestOverallScore: -1}
	for i := 0; i < rounds; i++ {
		r, err := c.Run(ctx, opts.RunOptions)
		if err != nil {
			return nil, err
		}
		if r.BestOverallScore > best.BestOverallScore {
			*best = *r
		}
		best.Usage.PromptTokens += r.Usage.PromptTokens
		best.Usage.CompletionTokens += r.Usage.CompletionTokens
		best.Usage.TotalTokens += r.Usage.TotalTokens
	}
	best.Summary = fmt.Sprintf("advanced: best temp %.2f across %d rounds",
		best.BestTemperature, rounds)
	return best, nil
}

// Evaluate scores a single (prompt, output) pair with the configured judges.
func (c *Client) Evaluate(ctx context.Context, opts EvaluateOptions) (*EvaluateResult, error) {
	if err := opts.Validate(); err != nil {
		return nil, errors.Wrap(errors.ErrCodeInvalidArgument, "autotemp",
			"invalid parameters", err)
	}
	opts.Defaults()

	c.mu.RLock()
	judges := append([]Judge(nil), c.judges...)
	c.mu.RUnlock()

	agg := ScoreBreakdown{}
	overall := 0.0
	for _, j := range judges {
		sb, err := j(ctx, opts.Prompt, opts.Output)
		if err != nil {
			return nil, errors.Wrap(errors.ErrCodeUnavailable, "autotemp",
				"judge failed", err)
		}
		agg.Relevance += sb.Relevance
		agg.Clarity += sb.Clarity
		agg.Utility += sb.Utility
		agg.Creativity += sb.Creativity
		agg.Coherence += sb.Coherence
		agg.Safety += sb.Safety
		agg.Overall += sb.Overall
		overall += sb.Overall
	}
	if n := float64(len(judges)); n > 0 {
		agg.Relevance /= n
		agg.Clarity /= n
		agg.Utility /= n
		agg.Creativity /= n
		agg.Coherence /= n
		agg.Safety /= n
		agg.Overall /= n
		overall /= n
	}
	return &EvaluateResult{OverallScore: overall, Scores: agg}, nil
}

// Benchmark runs a dataset across one or more models (or a single run if
// Models is empty).
func (c *Client) Benchmark(ctx context.Context, opts BenchmarkOptions) (*BenchmarkResult, error) {
	if err := opts.Validate(); err != nil {
		return nil, errors.Wrap(errors.ErrCodeInvalidArgument, "autotemp",
			"invalid parameters", err)
	}
	opts.Defaults()
	models := opts.Models
	if len(models) == 0 {
		models = []string{"default"}
	}
	result := &BenchmarkResult{}
	for _, m := range models {
		mb := ModelBenchmark{ModelName: m, NumItems: len(opts.Dataset)}
		meanAccum := 0.0
		for _, item := range opts.Dataset {
			if err := item.Validate(); err != nil {
				return nil, errors.Wrap(errors.ErrCodeInvalidArgument, "autotemp",
					"invalid dataset item", err)
			}
			r, err := c.Run(ctx, RunOptions{
				Prompt:       item.Prompt,
				Temperatures: opts.Temperatures,
				TopP:         opts.TopP,
				Judges:       opts.Judges,
				ModelVersion: m,
			})
			if err != nil {
				return nil, err
			}
			meanAccum += r.BestOverallScore
			mb.Tokens.PromptTokens += r.Usage.PromptTokens
			mb.Tokens.CompletionTokens += r.Usage.CompletionTokens
			mb.Tokens.TotalTokens += r.Usage.TotalTokens
		}
		if len(opts.Dataset) > 0 {
			mb.MeanOverall = meanAccum / float64(len(opts.Dataset))
		}
		result.ModelResults = append(result.ModelResults, mb)
	}
	result.Summary = fmt.Sprintf("benchmark: %d models, %d items",
		len(models), len(opts.Dataset))
	return result, nil
}

// --- baselines ---

// baselineRunner is a deterministic stand-in used when no production LLM
// runner has been injected. It echoes the prompt with a temperature-band
// tag so that baselineJudge can produce a temperature-sensitive score.
func baselineRunner(_ context.Context, prompt string, temperature, _ float64) (string, TokenUsage, error) {
	band := "mid"
	switch {
	case temperature < 0.3:
		band = "low"
	case temperature > 0.8:
		band = "high"
	}
	out := fmt.Sprintf("[%s] %s", band, prompt)
	u := TokenUsage{PromptTokens: len(prompt), CompletionTokens: len(out), TotalTokens: len(prompt) + len(out)}
	return out, u, nil
}

// baselineJudge favours mid-temperature outputs so that grid search in
// tests converges to a predictable winner when used with baselineRunner.
func baselineJudge(_ context.Context, _, output string) (ScoreBreakdown, error) {
	score := 0.5
	switch {
	case len(output) == 0:
		score = 0.0
	case containsTag(output, "[mid]"):
		score = 0.9
	case containsTag(output, "[low]"):
		score = 0.6
	case containsTag(output, "[high]"):
		score = 0.7
	}
	return ScoreBreakdown{
		Relevance:  score,
		Clarity:    score,
		Utility:    score,
		Creativity: score,
		Coherence:  score,
		Safety:     1.0,
		Overall:    score,
	}, nil
}

func containsTag(s, tag string) bool {
	if len(s) < len(tag) {
		return false
	}
	return s[:len(tag)] == tag
}

func scoreWithJudges(ctx context.Context, judges []Judge, prompt, output string) float64 {
	if len(judges) == 0 {
		return 0
	}
	total := 0.0
	for _, j := range judges {
		sb, err := j(ctx, prompt, output)
		if err != nil {
			continue
		}
		total += sb.Overall
	}
	return total / float64(len(judges))
}
