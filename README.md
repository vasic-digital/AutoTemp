# AutoTemp

**Module**: `digital.vasic.autotemp` · **Status**: i18n-migrated (round 132) + deep-doc + test-matrix (round 212) · **Production LOC**: ~350 across 3 packages (`pkg/client`, `pkg/types`, `pkg/i18n`)

Temperature auto-tuning for LLM interactions. AutoTemp runs a prompt at multiple temperatures and selects the best output using a pluggable multi-judge scoring pipeline. Part of the Plinius Go service family used by HelixAgent / HelixCode for ensemble inference tuning.

---

## Status Banner

- **2026-05-19 (round 212)** — deep-doc + test-matrix enrichment per operator's broader directive; this README expanded, `docs/test-coverage.md` added (CONST-050(B) ledger), `challenges/autotemp_runner_challenge.sh` added (build + unit + Runner/Judge contract probe + paired anti-bluff mutation).
- **2026-05-18 (round 23 §11.4)** — **silent-echo PASS-bluff removed.** `baselineRunner` previously echoed the prompt with a temperature-band tag and returned success, producing fabricated benchmark data when callers forgot `SetRunner`. It now returns `ErrBaselineRunnerNotConfigured` so absence of a real LLM Runner is loud, not silent. See `pkg/client/client.go` § "baselines".
- **2026-05-16 (round 132)** — i18n migration kickoff: `pkg/i18n` Translator contract seeded with EN bundle (`pkg/i18n/bundles/active.en.yaml`); structurally typed `Translator` interface mirrors `digital.vasic.lazy/pkg/i18n` so consumers can wire any real i18n stack (go-i18n / gotext / in-house).
- **2026-05-15** — CONST-047..061 governance cascade landed (see `CLAUDE.md` / `CONSTITUTION.md`).
- **2026-04-21** — extracted from HelixAgent internal research tree; graduated to FUNCTIONAL alongside 7 sibling Plinius modules.

---

## Purpose

AutoTemp solves one specific class of problem: **given a prompt and a candidate LLM, which sampling temperature yields the best output for THIS prompt under THESE judges?** It does not assume the answer is constant — different prompts (deterministic factoid vs creative continuation) prefer different temperatures, and judge functions encode what "best" means for the consumer's domain.

| API | Use case |
|-----|----------|
| `Run` | Single-prompt grid search across `Temperatures`; returns best (output, score, temperature). |
| `RunAdvanced` | Multi-round grid search (UCB-lite shape); useful when one round's signal is noisy. |
| `Evaluate` | Score one (prompt, output) pair without sampling — useful for offline judge calibration. |
| `Benchmark` | Dataset sweep across one or more model versions; aggregates per-model mean overall score. |

All four flow through the same `Runner` + `Judge` injection seams — no special-cased code paths, no hidden defaults that hit live LLM endpoints.

---

## Integration Seams

AutoTemp is **project-not-aware** (CONST-051(B)). Consumers integrate via three seams:

1. **`Runner`** — function-typed alias `func(ctx, prompt, temperature, topP) (output, TokenUsage, error)`. The consumer wires this to a real LLM provider (OpenAI / Anthropic / Bedrock / Ollama / Llama.cpp / HelixLLM). Absence of injection (i.e. forgetting `SetRunner`) surfaces the sentinel `ErrBaselineRunnerNotConfigured` — **never silent fabrication**.
2. **`Judge`** — function-typed alias `func(ctx, prompt, output) (ScoreBreakdown, error)`. The consumer supplies one or more judges. Each returns a 7-axis `ScoreBreakdown` (Relevance / Clarity / Utility / Creativity / Coherence / Safety / Overall) in `[0,1]`; AutoTemp averages `Overall` across judges to rank candidates.
3. **`Translator` (optional, via `pkg/i18n`)** — same structural contract as `digital.vasic.lazy/pkg/i18n.Translator`. Reserved for future locale-aware diagnostic output (`Describe`, error messages, status banners). Default `NoopTranslator` returns message IDs verbatim — safe for logs only, **NOT** end-user output (CONST-046).

No reverse coupling: AutoTemp never reaches into a consumer's tree, never imports a project-specific package, never assumes a HelixCode-specific layout.

---

## API Surface

### `pkg/client`

```go
// Function-typed injection seams.
type Runner func(ctx context.Context, prompt string, temperature, topP float64) (string, TokenUsage, error)
type Judge  func(ctx context.Context, prompt, output string) (ScoreBreakdown, error)

// Sentinel: returned by the default Runner if SetRunner was never called.
var ErrBaselineRunnerNotConfigured = fmt.Errorf("autotemp: baseline Runner has not been replaced ...")

// Client.
func New(opts ...config.Option) (*Client, error)
func NewFromConfig(cfg *config.Config) (*Client, error)
func (c *Client) Close() error
func (c *Client) Config() *config.Config
func (c *Client) SetRunner(r Runner)
func (c *Client) SetJudges(js ...Judge)

// Core operations.
func (c *Client) Run(ctx, opts RunOptions) (*RunResult, error)
func (c *Client) RunAdvanced(ctx, opts AdvancedOptions) (*RunResult, error)
func (c *Client) Evaluate(ctx, opts EvaluateOptions) (*EvaluateResult, error)
func (c *Client) Benchmark(ctx, opts BenchmarkOptions) (*BenchmarkResult, error)
```

### `pkg/types`

| Type | Role |
|------|------|
| `RunOptions` | `Prompt`, `Temperatures []float64`, `TopP`, `AutoSelect`, `Judges`, `ModelVersion`. |
| `AdvancedOptions` | embeds `RunOptions`, adds `Rounds`, `ExplorationC`. |
| `EvaluateOptions` | `Prompt`, `Output`, `Temperature`, `TopP`, `Judges`, `ModelVersion`. |
| `BenchmarkOptions` | `Dataset []BenchmarkItem`, `Temperatures`, `TopP`, `Advanced`, `Rounds`, `Judges`, `Models`. |
| `BenchmarkItem` | `Prompt`, `Reference`. |
| `RunResult` | `BestOutput`, `BestTemperature`, `BestOverallScore`, `Summary`, `Usage`. |
| `EvaluateResult` | `OverallScore`, `Scores ScoreBreakdown`, `Usage`. |
| `BenchmarkResult` | `[]ModelBenchmark`, `Summary`. |
| `ModelBenchmark` | `ModelName`, `MeanOverall`, `NumItems`, `Tokens`. |
| `ScoreBreakdown` | 7 floats in `[0,1]`: Relevance, Clarity, Utility, Creativity, Coherence, Safety, Overall. |
| `TokenUsage` | `PromptTokens`, `CompletionTokens`, `TotalTokens`. |

### `pkg/i18n`

```go
type Translator interface {
    T(ctx context.Context, messageID string, templateData map[string]any) (string, error)
    TPlural(ctx context.Context, messageID string, count int, templateData map[string]any) (string, error)
}

type NoopTranslator struct{}      // message-id passthrough; SAFETY DEFAULT, not production-ready
```

Reserved for future `Describe(ctx)`-style locale-aware methods on `Client`. Today's surface still emits English-only `Summary` strings; the i18n migration round-132 seeded the contract so subsequent rounds can route those through a real Translator without breaking the API.

---

## Usage Examples

### 1. Minimal grid search with an injected Runner + custom Judge

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

// REQUIRED — without SetRunner, Run returns ErrBaselineRunnerNotConfigured.
c.SetRunner(func(ctx context.Context, prompt string, temperature, topP float64) (string, types.TokenUsage, error) {
    return provider.Complete(ctx, prompt, temperature, topP)  // real LLM call
})

// REQUIRED for meaningful scoring — without SetJudges, the baseline judge
// is used (which favours outputs prefixed "[mid]" — a unit-test convention,
// not production-grade quality scoring).
c.SetJudges(myStructuredJudge)

res, err := c.Run(context.Background(), types.RunOptions{
    Prompt:       "Write a haiku about autumn.",
    Temperatures: []float64{0.2, 0.5, 0.7, 1.0},
})
if err != nil { log.Fatal(err) }
log.Printf("best temp=%.2f score=%.3f output=%q", res.BestTemperature, res.BestOverallScore, res.BestOutput)
```

### 2. Multi-round UCB-lite search

```go
res, err := c.RunAdvanced(ctx, types.AdvancedOptions{
    RunOptions: types.RunOptions{
        Prompt:       "Summarize the following article: ...",
        Temperatures: []float64{0.1, 0.3, 0.5, 0.7, 0.9},
    },
    Rounds:       4,
    ExplorationC: 1.4,
})
```

### 3. Cross-model benchmark sweep

```go
res, err := c.Benchmark(ctx, types.BenchmarkOptions{
    Dataset: []types.BenchmarkItem{
        {Prompt: "What is 2+2?", Reference: "4"},
        {Prompt: "Capital of Japan?", Reference: "Tokyo"},
    },
    Temperatures: []float64{0.0, 0.5, 1.0},
    Models:       []string{"llama3.2:8b", "mistral:7b"},
})
for _, mb := range res.ModelResults {
    log.Printf("%s mean=%.3f items=%d", mb.ModelName, mb.MeanOverall, mb.NumItems)
}
```

### 4. Single-pair offline scoring (Evaluate)

```go
res, err := c.Evaluate(ctx, types.EvaluateOptions{
    Prompt: "Explain quantum entanglement in one sentence.",
    Output: "Two particles share a state such that measuring one instantly determines the other.",
})
log.Printf("overall=%.3f relevance=%.3f clarity=%.3f", res.OverallScore, res.Scores.Relevance, res.Scores.Clarity)
```

---

## Runner / Judge Injection Pattern — Anti-Bluff Contract

The single most important invariant of this library: **the default Runner is intentionally non-functional.** This is by design, fixed in round-23 §11.4 audit (2026-05-18) to remove a silent PASS-bluff.

```go
// pkg/client/client.go
func baselineRunner(_ context.Context, prompt string, _, _ float64) (string, TokenUsage, error) {
    _ = prompt
    return "", TokenUsage{}, ErrBaselineRunnerNotConfigured
}
```

Why: a previous revision returned a synthetic echo string. Callers who forgot to wire a real LLM received fabricated outputs that scored against the baseline judge — every PASS in their pipeline was meaningless. The sentinel error makes the absence of a real Runner LOUD.

**Consumer rules**:

1. Production code MUST call `SetRunner` with a real LLM-dispatching adapter before `Run` / `RunAdvanced` / `Evaluate` / `Benchmark`.
2. Unit tests MAY use an in-process deterministic stub (CONST-050(A)) — see `pkg/client/client_test.go` `echoTestRunner`.
3. Integration / E2E tests MUST exercise a real LLM endpoint (CONST-050(B)) — Ollama / Llama.cpp / a sandboxed real provider.

**Judge rules**:

1. Default `baselineJudge` is a unit-test convenience — it gives `[mid]`-prefixed outputs a score of 0.9 so grid search converges deterministically. NOT for production scoring.
2. Production judges should encode the consumer's actual quality criteria (LLM-as-judge with structured rubric, classifier model, BLEU/ROUGE for translation, etc.).
3. Judges returning errors are silently skipped in the aggregate (see `scoreWithJudges`); a Judge that fatally errs MUST log and surface it through its own observability — AutoTemp will not propagate.

---

## Testing

```bash
# Resource-capped run (per CLAUDE.md)
GOMAXPROCS=2 nice -n 19 ionice -c 3 go test ./... -count=1 -p 1 -race

# Coverage report
go test ./... -cover -count=1

# Full Challenge — build + vet + unit + Runner/Judge contract probe + paired anti-bluff mutation
bash challenges/autotemp_runner_challenge.sh
```

The Challenge exits non-zero if any of:

- `go vet ./...` fails
- `go build ./...` fails
- the unit suite fails or skips silently
- `New()` returns no error but the default Runner does NOT return `ErrBaselineRunnerNotConfigured` (regression of the round-23 fix)
- a real `SetRunner` injection still produces empty `BestOutput` (Run pipeline broken)
- the anti-bluff mutation (deliberately re-introduce a silent echo Runner) is NOT caught by the contract probe

See [`docs/test-coverage.md`](docs/test-coverage.md) for the full test-type matrix (CONST-050(B) compliance ledger).

---

## Governance

- Anti-bluff prime directive: [`CLAUDE.md`](CLAUDE.md) preamble + Article XI §11.9 anchor.
- CONST-033 (host power), CONST-035 (anti-bluff), CONST-036 (no user-session termination), CONST-046 (no hardcoded content), CONST-047..061 cascade — see [`CONSTITUTION.md`](CONSTITUTION.md).
- Canonical-root inheritance (CONST-059): governance text in this submodule is the consumer-side extension; universal rules live in the [HelixConstitution submodule](https://github.com/HelixDevelopment/HelixConstitution).

---

## Module Identity

| Field            | Value                              |
|------------------|------------------------------------|
| Go module        | `digital.vasic.autotemp`           |
| Go version       | 1.22                               |
| Direct deps      | `digital.vasic.pliniuscommon`, `github.com/stretchr/testify` (test-only), `gopkg.in/yaml.v3` (i18n bundle parse) |
| Upstream remote  | `vasic-digital/AutoTemp` (GitHub)  |
| License          | Apache-2.0 (see `LICENSE`)         |

---

## Development layout

This module's `go.mod` declares `digital.vasic.autotemp` and uses a relative `replace` directive pointing at `../PliniusCommon`. To build locally, clone the sibling repos next to this one:

```
workspace/
  PliniusCommon/
  AutoTemp/
  ... other Plinius siblings ...
```

Inside HelixCode's monorepo layout, that translates to:

```
dependencies/vasic-digital/
  PliniusCommon/
  AutoTemp/
  Lazy/
  ... other sibling modules ...
```

---

## Lineage

Extracted from internal HelixAgent research tree on 2026-04-21. Graduated to functional status the same day. Historical research corpus (unused) remains at `docs/research/go-elder-plinius-v3/go-elder-plinius/go-autotemp/` inside the HelixAgent repository.
