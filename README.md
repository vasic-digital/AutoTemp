# AutoTemp

Temperature auto-tuning for LLM interactions. AutoTemp runs a prompt at
multiple temperatures and selects the best output using a pluggable
multi-judge scoring pipeline. Part of the Plinius Go service family used
by HelixAgent.

## Status

- Compiles: `go build ./...` exits 0.
- Tests pass under `-race`: 2 packages (types, client), all green.
- Baseline runner + judge seeded by default so the client is immediately
  testable without an LLM backend.
- Integration-ready: consumable Go library for the HelixAgent ensemble.

## Purpose

- `pkg/types` — value types: `RunOptions`, `RunResult`, `AdvancedOptions`,
  `EvaluateOptions`, `EvaluateResult`, `ScoreBreakdown`,
  `BenchmarkOptions`, `BenchmarkItem`, `BenchmarkResult`,
  `ModelBenchmark`, `TokenUsage`.
- `pkg/client` — grid-search + multi-judge scoring:
  - `Run` — grid search; picks highest-scoring temperature
  - `RunAdvanced` — multi-round UCB-lite grid search
  - `Evaluate` — score a single (prompt, output) pair
  - `Benchmark` — dataset sweep across one or more models
  - `SetRunner(Runner)` / `SetJudges(...Judge)` — dependency injection
    for real LLM providers and structured-output judges

## Usage

```go
import (
    "context"
    "log"

    autotemp "digital.vasic.autotemp/pkg/client"
    "digital.vasic.autotemp/pkg/types"
)

c, err := autotemp.New()
if err != nil { log.Fatal(err) }
defer c.Close()

// Wire in your LLM provider and judges:
c.SetRunner(func(ctx context.Context, prompt string, temperature, topP float64) (string, types.TokenUsage, error) {
    // call into OpenAI / Anthropic / HelixLLM / …
    return "...", types.TokenUsage{}, nil
})

res, err := c.Run(context.Background(), types.RunOptions{
    Prompt:       "Write a haiku about autumn.",
    Temperatures: []float64{0.2, 0.5, 0.7, 1.0},
})
if err != nil { log.Fatal(err) }
log.Printf("best temp=%.2f score=%.3f", res.BestTemperature, res.BestOverallScore)
```

## Module path

```go
import "digital.vasic.autotemp"
```

## Lineage

Extracted from internal HelixAgent research tree on 2026-04-21.
Graduated to functional status on the same day alongside its 7 sibling
Plinius modules.

Historical research corpus (unused) remains at
`docs/research/go-elder-plinius-v3/go-elder-plinius/go-autotemp/` inside
the HelixAgent repository.

## Development layout

This module's `go.mod` declares the module as `digital.vasic.autotemp`
and uses a relative `replace` directive pointing at `../PliniusCommon`.
To build locally, clone the sibling repos next to this one:

```
workspace/
  PliniusCommon/
  AutoTemp/
  ... other siblings ...
```

## License

Apache-2.0
