package client

import (
	"context"
	"testing"

	"digital.vasic.autotemp/pkg/types"
)

func BenchmarkRun(b *testing.B) {
	c, err := New()
	if err != nil {
		b.Fatal(err)
	}
	defer c.Close()
	ctx := context.Background()
	opts := types.RunOptions{Prompt: "benchmark prompt", Temperatures: []float64{0.1, 0.4, 0.7, 1.0}}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := c.Run(ctx, opts); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEvaluate(b *testing.B) {
	c, err := New()
	if err != nil {
		b.Fatal(err)
	}
	defer c.Close()
	ctx := context.Background()
	opts := types.EvaluateOptions{Prompt: "p", Output: "o"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := c.Evaluate(ctx, opts); err != nil {
			b.Fatal(err)
		}
	}
}
