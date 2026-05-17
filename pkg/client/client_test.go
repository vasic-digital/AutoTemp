package client

import (
	"context"
	"fmt"
	"testing"

	"digital.vasic.autotemp/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// echoTestRunner is a deterministic unit-test stand-in for a real LLM Runner.
// CONST-050(A) permits mocks/stubs in unit tests only — production code MUST
// receive a real LLM-dispatching Runner via SetRunner, otherwise New()'s
// default returns ErrBaselineRunnerNotConfigured (round-23 §11.4 audit fix).
func echoTestRunner(_ context.Context, prompt string, temperature, _ float64) (string, types.TokenUsage, error) {
	band := "mid"
	switch {
	case temperature < 0.3:
		band = "low"
	case temperature > 0.8:
		band = "high"
	}
	out := fmt.Sprintf("[%s] %s", band, prompt)
	u := types.TokenUsage{PromptTokens: len(prompt), CompletionTokens: len(out), TotalTokens: len(prompt) + len(out)}
	return out, u, nil
}

// newTestClient builds a client with the echo stub installed so unit tests
// have deterministic behaviour without depending on a real LLM provider.
func newTestClient(t *testing.T) *Client {
	t.Helper()
	c, err := New()
	require.NoError(t, err)
	c.SetRunner(echoTestRunner)
	return c
}

func TestNew(t *testing.T) {
	client, err := New()
	require.NoError(t, err)
	assert.NotNil(t, client)
	assert.NoError(t, client.Close())
}

func TestDoubleClose(t *testing.T) {
	client, err := New()
	require.NoError(t, err)
	assert.NoError(t, client.Close())
	assert.NoError(t, client.Close())
}

func TestConfig(t *testing.T) {
	client, err := New()
	require.NoError(t, err)
	defer client.Close()
	assert.NotNil(t, client.Config())
}

func TestRunBaseline(t *testing.T) {
	c := newTestClient(t)
	defer c.Close()

	res, err := c.Run(context.Background(), types.RunOptions{
		Prompt:       "hello world",
		Temperatures: []float64{0.1, 0.5, 0.9},
	})
	require.NoError(t, err)
	assert.Equal(t, 0.5, res.BestTemperature) // baselineJudge favours "mid"
	assert.Greater(t, res.BestOverallScore, 0.0)
	assert.NotEmpty(t, res.BestOutput)
	assert.Greater(t, res.Usage.TotalTokens, 0)
}

func TestRunDefaultGrid(t *testing.T) {
	c := newTestClient(t)
	defer c.Close()

	res, err := c.Run(context.Background(), types.RunOptions{Prompt: "x"})
	require.NoError(t, err)
	assert.NotEmpty(t, res.BestOutput)
}

func TestRunInvalid(t *testing.T) {
	c := newTestClient(t)
	defer c.Close()
	_, err := c.Run(context.Background(), types.RunOptions{})
	assert.Error(t, err)
}

// TestRunWithoutInjectedRunner_ReturnsSentinel asserts the round-23 §11.4
// audit fix: New()'s default Runner returns ErrBaselineRunnerNotConfigured
// when SetRunner is not called, instead of the previous silent echo-back
// that produced fabricated benchmark data.
func TestRunWithoutInjectedRunner_ReturnsSentinel(t *testing.T) {
	c, err := New()
	require.NoError(t, err)
	defer c.Close()

	_, err = c.Run(context.Background(), types.RunOptions{
		Prompt:       "hello world",
		Temperatures: []float64{0.5},
	})
	require.Error(t, err, "Run without injected Runner MUST surface the sentinel error, not return fabricated data")
	require.ErrorIs(t, err, ErrBaselineRunnerNotConfigured)
}

func TestSetRunnerAndJudges(t *testing.T) {
	c, err := New()
	require.NoError(t, err)
	defer c.Close()

	c.SetRunner(func(_ context.Context, _ string, _, _ float64) (string, types.TokenUsage, error) {
		return "custom", types.TokenUsage{TotalTokens: 1}, nil
	})
	c.SetJudges(func(_ context.Context, _, _ string) (types.ScoreBreakdown, error) {
		return types.ScoreBreakdown{Overall: 0.42}, nil
	})

	res, err := c.Run(context.Background(), types.RunOptions{
		Prompt:       "y",
		Temperatures: []float64{0.2},
	})
	require.NoError(t, err)
	assert.Equal(t, "custom", res.BestOutput)
	assert.InDelta(t, 0.42, res.BestOverallScore, 1e-6)
}

func TestRunAdvanced(t *testing.T) {
	c := newTestClient(t)
	defer c.Close()

	res, err := c.RunAdvanced(context.Background(), types.AdvancedOptions{
		RunOptions: types.RunOptions{Prompt: "hello", Temperatures: []float64{0.2, 0.5, 0.9}},
		Rounds:     3,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, res.BestOutput)
}

func TestEvaluate(t *testing.T) {
	c := newTestClient(t)
	defer c.Close()

	res, err := c.Evaluate(context.Background(), types.EvaluateOptions{
		Prompt: "prompt",
		Output: "[mid] prompt",
	})
	require.NoError(t, err)
	assert.Greater(t, res.OverallScore, 0.0)
}

func TestBenchmark(t *testing.T) {
	c := newTestClient(t)
	defer c.Close()

	res, err := c.Benchmark(context.Background(), types.BenchmarkOptions{
		Dataset: []types.BenchmarkItem{
			{Prompt: "p1", Reference: "r1"},
			{Prompt: "p2", Reference: "r2"},
		},
		Temperatures: []float64{0.3, 0.6},
	})
	require.NoError(t, err)
	assert.Len(t, res.ModelResults, 1)
	assert.Equal(t, 2, res.ModelResults[0].NumItems)
	assert.Greater(t, res.ModelResults[0].MeanOverall, 0.0)
}
