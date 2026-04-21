package client

import (
	"context"
	"testing"

	"digital.vasic.autotemp/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	c, err := New()
	require.NoError(t, err)
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
	c, err := New()
	require.NoError(t, err)
	defer c.Close()

	res, err := c.Run(context.Background(), types.RunOptions{Prompt: "x"})
	require.NoError(t, err)
	assert.NotEmpty(t, res.BestOutput)
}

func TestRunInvalid(t *testing.T) {
	c, err := New()
	require.NoError(t, err)
	defer c.Close()
	_, err = c.Run(context.Background(), types.RunOptions{})
	assert.Error(t, err)
}

func TestSetRunnerAndJudges(t *testing.T) {
	c, err := New()
	require.NoError(t, err)
	defer c.Close()

	c.SetRunner(func(_ context.Context, prompt string, t, _ float64) (string, types.TokenUsage, error) {
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
	c, err := New()
	require.NoError(t, err)
	defer c.Close()

	res, err := c.RunAdvanced(context.Background(), types.AdvancedOptions{
		RunOptions: types.RunOptions{Prompt: "hello", Temperatures: []float64{0.2, 0.5, 0.9}},
		Rounds:     3,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, res.BestOutput)
}

func TestEvaluate(t *testing.T) {
	c, err := New()
	require.NoError(t, err)
	defer c.Close()

	res, err := c.Evaluate(context.Background(), types.EvaluateOptions{
		Prompt: "prompt",
		Output: "[mid] prompt",
	})
	require.NoError(t, err)
	assert.Greater(t, res.OverallScore, 0.0)
}

func TestBenchmark(t *testing.T) {
	c, err := New()
	require.NoError(t, err)
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
