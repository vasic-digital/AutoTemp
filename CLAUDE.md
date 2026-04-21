# CLAUDE.md -- digital.vasic.autotemp

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
