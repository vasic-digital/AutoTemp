package client

import (
	"context"
	"errors"
	"testing"

	"digital.vasic.autotemp/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunSinglePointGrid — a 1-element temperature grid is legal and returns it.
func TestRunSinglePointGrid(t *testing.T) {
	c, err := New()
	require.NoError(t, err)
	defer c.Close()

	res, err := c.Run(context.Background(), types.RunOptions{
		Prompt:       "hi",
		Temperatures: []float64{0.42},
	})
	require.NoError(t, err)
	assert.Equal(t, 0.42, res.BestTemperature)
}

// TestRunDefaultGridWinsMid — empty Temperatures yields the built-in default grid.
func TestRunDefaultGridWinsMid(t *testing.T) {
	c, err := New()
	require.NoError(t, err)
	defer c.Close()

	res, err := c.Run(context.Background(), types.RunOptions{Prompt: "hi"})
	require.NoError(t, err)
	// baseline judge favours mid-band (0.4 temp) => should win.
	assert.InDelta(t, 0.4, res.BestTemperature, 0.01)
	assert.Greater(t, res.BestOverallScore, 0.0)
}

// TestRunInvalidPromptReturnsError — empty prompt path.
func TestRunInvalidPromptReturnsError(t *testing.T) {
	c, err := New()
	require.NoError(t, err)
	defer c.Close()

	_, err = c.Run(context.Background(), types.RunOptions{Prompt: "   "})
	assert.Error(t, err)
}

// TestRunRunnerErrorPropagates — a runner that errors surfaces a wrapped error.
func TestRunRunnerErrorPropagates(t *testing.T) {
	c, err := New()
	require.NoError(t, err)
	defer c.Close()

	c.SetRunner(func(ctx context.Context, prompt string, t, topP float64) (string, types.TokenUsage, error) {
		return "", types.TokenUsage{}, errors.New("boom")
	})
	_, err = c.Run(context.Background(), types.RunOptions{Prompt: "p"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "runner failed")
}

// TestEvaluateJudgeErrorPropagates — judge error must bubble.
func TestEvaluateJudgeErrorPropagates(t *testing.T) {
	c, err := New()
	require.NoError(t, err)
	defer c.Close()

	c.SetJudges(func(ctx context.Context, prompt, output string) (types.ScoreBreakdown, error) {
		return types.ScoreBreakdown{}, errors.New("judge exploded")
	})
	_, err = c.Evaluate(context.Background(), types.EvaluateOptions{Prompt: "p", Output: "o"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "judge failed")
}

// TestEvaluateMultiJudgeAverage — multiple judges produce averaged results.
func TestEvaluateMultiJudgeAverage(t *testing.T) {
	c, err := New()
	require.NoError(t, err)
	defer c.Close()

	c.SetJudges(
		func(ctx context.Context, prompt, output string) (types.ScoreBreakdown, error) {
			return types.ScoreBreakdown{Overall: 0.2}, nil
		},
		func(ctx context.Context, prompt, output string) (types.ScoreBreakdown, error) {
			return types.ScoreBreakdown{Overall: 0.8}, nil
		},
	)
	res, err := c.Evaluate(context.Background(), types.EvaluateOptions{Prompt: "p", Output: "o"})
	require.NoError(t, err)
	assert.InDelta(t, 0.5, res.OverallScore, 0.0001)
}

// TestSetRunnerNilIgnored — passing nil runner must be a no-op.
func TestSetRunnerNilIgnored(t *testing.T) {
	c, err := New()
	require.NoError(t, err)
	defer c.Close()
	c.SetRunner(nil)
	// Baseline runner should still work.
	res, err := c.Run(context.Background(), types.RunOptions{Prompt: "hi"})
	require.NoError(t, err)
	assert.NotEmpty(t, res.BestOutput)
}

// TestSetJudgesEmptyIgnored — passing no judges must be a no-op.
func TestSetJudgesEmptyIgnored(t *testing.T) {
	c, err := New()
	require.NoError(t, err)
	defer c.Close()
	c.SetJudges()
	res, err := c.Evaluate(context.Background(), types.EvaluateOptions{Prompt: "p", Output: "o"})
	require.NoError(t, err)
	assert.NotNil(t, res)
}

// TestRunAdvancedDefaultRoundsOne — 0 rounds coerces to 1.
func TestRunAdvancedDefaultRoundsOne(t *testing.T) {
	c, err := New()
	require.NoError(t, err)
	defer c.Close()

	res, err := c.RunAdvanced(context.Background(), types.AdvancedOptions{
		RunOptions: types.RunOptions{Prompt: "x"},
		Rounds:     0,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, res.Summary)
}

// TestBenchmarkEmptyDataset — empty dataset should not error; produces a model result with NumItems=0.
func TestBenchmarkEmptyDataset(t *testing.T) {
	c, err := New()
	require.NoError(t, err)
	defer c.Close()

	res, err := c.Benchmark(context.Background(), types.BenchmarkOptions{
		Models: []string{"m1"},
	})
	require.NoError(t, err)
	require.Len(t, res.ModelResults, 1)
	assert.Equal(t, 0, res.ModelResults[0].NumItems)
}

// TestBenchmarkInvalidItemPropagates — a malformed dataset item triggers a wrapped InvalidArgument.
func TestBenchmarkInvalidItemPropagates(t *testing.T) {
	c, err := New()
	require.NoError(t, err)
	defer c.Close()

	_, err = c.Benchmark(context.Background(), types.BenchmarkOptions{
		Dataset: []types.BenchmarkItem{{Prompt: ""}},
	})
	assert.Error(t, err)
}

// TestRunnerTokenUsageAccumulation — grid search must sum token usages.
func TestRunnerTokenUsageAccumulation(t *testing.T) {
	c, err := New()
	require.NoError(t, err)
	defer c.Close()

	calls := 0
	c.SetRunner(func(ctx context.Context, prompt string, t, topP float64) (string, types.TokenUsage, error) {
		calls++
		return "out", types.TokenUsage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8}, nil
	})
	c.SetJudges(func(ctx context.Context, prompt, output string) (types.ScoreBreakdown, error) {
		return types.ScoreBreakdown{Overall: 0.5}, nil
	})
	res, err := c.Run(context.Background(), types.RunOptions{
		Prompt:       "p",
		Temperatures: []float64{0.1, 0.5, 0.9},
	})
	require.NoError(t, err)
	assert.Equal(t, 3, calls)
	assert.Equal(t, 15, res.Usage.PromptTokens)
	assert.Equal(t, 9, res.Usage.CompletionTokens)
	assert.Equal(t, 24, res.Usage.TotalTokens)
}
