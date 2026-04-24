# CLAUDE.md -- digital.vasic.autotemp


## Definition of Done

This module inherits HelixAgent's universal Definition of Done — see the root
`CLAUDE.md` and `docs/development/definition-of-done.md`. In one line: **no
task is done without pasted output from a real run of the real system in the
same session as the change.** Coverage and green suites are not evidence.

### Acceptance demo for this module

<!-- TODO: replace this block with the exact command(s) that exercise this
     module end-to-end against real dependencies, and the expected output.
     The commands must run the real artifact (built binary, deployed
     container, real service) — no in-process fakes, no mocks, no
     `httptest.NewServer`, no Robolectric, no JSDOM as proof of done. -->

```bash
# TODO
```

Module-specific guidance for Claude Code.

## Status

**FUNCTIONAL.** 2 packages (types, client) ship tested implementations;
`go test -race ./...` all green. Baseline runner + judge seeded on
`New()` so the client is immediately usable in tests; production
deployments wire real LLM providers via `SetRunner` and structured
judges via `SetJudges`.

## Hard rules

1. **NO CI/CD pipelines** -- no `.github/workflows/`, `.gitlab-ci.yml`,
   `Jenkinsfile`, `.travis.yml`, `.circleci/`, or any automated
   pipeline. No Git hooks either. Permanent.
2. **SSH-only for Git** -- `git@github.com:...` / `git@gitlab.com:...`.
   Never HTTPS, even for public clones.
3. **Conventional Commits** -- `feat(autotemp): ...`, `fix(...)`,
   `docs(...)`, `test(...)`, `refactor(...)`.
4. **Code style** -- `gofmt`, `goimports`, 100-char line ceiling,
   errors always checked and wrapped (`fmt.Errorf("...: %w", err)`).
5. **Resource cap for tests** --
   `GOMAXPROCS=2 nice -n 19 ionice -c 3 go test -count=1 -p 1 -race ./...`

## Purpose

Benchmark-driven temperature auto-tuning. Provides a grid-search core
(`Run`), a multi-round variant (`RunAdvanced`), single-pair scoring
(`Evaluate`), and dataset-level sweeps (`Benchmark`). The `Runner` and
`Judge` abstractions are injected by the consumer.

## Primary consumer

HelixAgent (`dev.helix.agent`) — ensemble inference tuning.

## Testing

```
GOMAXPROCS=2 nice -n 19 ionice -c 3 go test -count=1 -p 1 -race ./...
```

Must stay all-green on every commit.

## API Cheat Sheet

**Module path:** `digital.vasic.autotemp`.

```go
type Runner func(ctx, prompt string, temperature, topP float64) (string, TokenUsage, error)
type Judge  func(ctx, prompt, output string) (ScoreBreakdown, error)

type RunOptions struct {
    Prompt       string
    Temperatures []float64
    TopP         float64
}
type RunResult struct {
    BestOutput, Summary string
    BestTemperature, BestOverallScore float64
    Usage TokenUsage
}

type Client struct { /* baseline grid-search tuner */ }

func New(opts ...config.Option) (*Client, error)
func (c *Client) SetRunner(r Runner)
func (c *Client) SetJudges(js ...Judge)
func (c *Client) Run(ctx, opts RunOptions) (*RunResult, error)
func (c *Client) RunAdvanced(ctx, opts AdvancedOptions) (*RunResult, error)
func (c *Client) Evaluate(ctx, opts EvaluateOptions) (*EvaluateResult, error)
func (c *Client) Benchmark(ctx, opts BenchmarkOptions) (*BenchmarkResult, error)
func (c *Client) Close() error
```

**Typical usage:**
```go
c, _ := autotemp.New()
defer c.Close()
c.SetRunner(func(ctx context.Context, prompt string, t, p float64) (string, TokenUsage, error) {
    return provider.Complete(prompt, t, p)
})
c.SetJudges(myJudge)
res, _ := c.Run(ctx, autotemp.RunOptions{Prompt: "summarize X", Temperatures: []float64{0.2, 0.7, 1.0}})
```

**Injection points:** `Runner` (LLM adapter), `Judge` (scoring function).
**Defaults on `New`:** deterministic baseline runner + baseline judge; default temps `[0.1, 0.4, 0.7, 1.0]`.

## Integration Seams

| Direction | Sibling modules |
|-----------|-----------------|
| Upstream (this module imports) | PliniusCommon |
| Downstream (these import this module) | root only |

*Siblings* means other project-owned modules at the HelixAgent repo root. The root HelixAgent app and external systems are not listed here — the list above is intentionally scoped to module-to-module seams, because drift *between* sibling modules is where the "tests pass, product broken" class of bug most often lives. See root `CLAUDE.md` for the rules that keep these seams contract-tested.
