# AutoTemp — Test-Type Coverage Matrix

**Authority**: CONST-050(B) "100%-Test-Type-Coverage" mandate (cascaded from HelixConstitution submodule §11.4.27).
**Scope**: this document is the AutoTemp submodule's coverage ledger. It enumerates every test type CONST-050(B) recognises and records the current status against AutoTemp's surface (`pkg/client` + `pkg/types` + `pkg/i18n`).

A row may be `covered`, `planned`, or `n/a (out of scope for a library of this shape)`. `n/a` rows MUST justify themselves — silent omission is a CONST-048 violation per §11.4.25.

---

## Coverage Ledger

| Test type        | Status   | Artefact / location                                                                                              | Notes |
|------------------|----------|------------------------------------------------------------------------------------------------------------------|-------|
| Unit             | covered  | `pkg/client/client_test.go`, `client_extra_test.go`, `client_bench_test.go`, `pkg/types/types_test.go`, `pkg/i18n/translator_test.go` | Mocks permitted per CONST-050(A); race-detector enforced; `echoTestRunner` is the canonical unit-test Runner stub; `TestRunWithoutInjectedRunner_ReturnsSentinel` asserts the round-23 §11.4 fix stays in place. |
| Integration      | planned  | recommend: real-Runner wire-up against a local Ollama / Llama.cpp instance                                       | Consumer is HelixCode's `internal/llm/autotemp_adapter` — exercise full Run / RunAdvanced / Benchmark against a real model (`llama3.2:1b` for test speed). Owner: HelixCode integration layer, NOT AutoTemp itself (CONST-051(B) — AutoTemp stays project-not-aware). |
| E2E              | covered  | `challenges/autotemp_runner_challenge.sh`                                                                        | Bash-orchestrated full round-trip — vet + build + unit + Runner-contract probe (sentinel error + working-injection both verified) + paired anti-bluff mutation that re-installs a silent echo Runner and asserts the probe FAILs. |
| Full automation  | planned  | recommend: re-run the Challenge under every supported Go minor (1.22, 1.25, 1.26) on every host platform (linux/darwin/windows) | CONST-048 coverage matrix dimension is feature × platform × invariant; AutoTemp is pure Go so platform coverage = Go-supported set. |
| Security         | planned  | recommend: input-fuzz on `RunOptions.Prompt` (extreme length, null bytes, embedded control chars); judge-error propagation safety; nil-handling of `SetRunner(nil)` and `SetJudges()` empty input (currently silently ignored — verify by test) | Threat model: a hostile prompt MUST NOT crash AutoTemp, leak memory, or bypass the Runner-injection invariant. |
| DDoS             | n/a      | —                                                                                                                | AutoTemp is an in-process library — no network surface, no request fan-in. The consuming service exposes the DDoS surface, not AutoTemp. |
| Scaling          | planned  | recommend: benchmark `Run` under N goroutines (N ∈ {1, 10, 100, 1000}) verifying mutex contention stays bounded; benchmark `Benchmark` with `len(Dataset)` ∈ {10, 100, 1000, 10000} | Pure-CPU scaling test; not a network-tier scaling test. The `client_bench_test.go` file already seeds the micro-benchmark surface. |
| Chaos            | planned  | recommend: chaos-style assertion that a Runner returning context-canceled mid-grid propagates correctly; Judge returning panic vs error; partial-failure aggregation behaviour | Failure-injection scope is narrow because the surface is narrow — but a complete CONST-050(B) ledger still names it. |
| Stress           | planned  | recommend: `go test -count=10000 ./pkg/client` to surface flakiness in the concurrent SetRunner / SetJudges / Run interleaving | Stress = sustained load above advertised tier; for an in-process primitive that means iteration count. |
| Performance      | covered (micro) / planned (macro) | `pkg/client/client_bench_test.go` (micro: per-Run, per-Evaluate latency with `b.ReportAllocs()`); macro recommended on HelixCode side | The library's value prop is "grid search adds bounded overhead over N×Runner calls" — benchmark MUST prove the per-temperature scoring path stays at sub-µs scoring overhead modulo the Runner call. |
| Benchmarking     | covered (micro) / planned (macro) | `pkg/client/client_bench_test.go` + historical p95-drift detection planned in HelixCode's release-gate sweep | Macro tier lives outside AutoTemp (CONST-051(B)). |
| UI               | n/a      | —                                                                                                                | AutoTemp ships no UI. |
| UX               | planned  | recommend: when `pkg/i18n` is wired to `Client.Describe`-style methods (future round), the bilingual round-trip pattern from Lazy's `lazy_describe_challenge.sh` will apply here too | Currently AutoTemp's user-facing strings are limited to `Summary` (English-only); CONST-046 work to dynamically generate locale-aware summaries is a future round. |
| Challenges       | covered  | `challenges/autotemp_runner_challenge.sh` (added round 212)                                                      | Incorporates the `vasic-digital/Challenges` pattern; captures stdout/stderr as wire evidence per §11.4.2; paired mutation per §1.1 / CONST-055 meta-test. |
| HelixQA          | planned  | recommend: register AutoTemp as a target in HelixQA's autonomous QA bank                                         | HelixQA submodule (`HelixDevelopment/HelixQA`) is incorporated at HelixCode root per CONST-050; AutoTemp enrolment is a HelixCode-meta-repo task, not an AutoTemp-internal task. |

---

## Anti-Bluff Posture

Every `covered` row above carries captured runtime evidence:

- **Unit**: `go test ./... -count=1 -race` exits 0; `TestRunWithoutInjectedRunner_ReturnsSentinel` proves the round-23 §11.4 fix is enforced (default Runner returns `ErrBaselineRunnerNotConfigured`); coverage measured by `go test -cover`.
- **E2E (Challenge)**: `challenges/autotemp_runner_challenge.sh` writes `challenges/.last-run/` artefacts containing stdout + stderr + assertion log + mutation-rejection proof. The mutation leg deliberately swaps in a silent echo Runner and asserts the contract probe FAILs — proving the probe would catch a real regression.
- **Performance (micro)**: `go test -bench=. -benchmem ./pkg/client` produces ns/op + allocs/op numbers; future macro-benchmarks will diff against historical baseline.

Rows marked `planned` are **deliverables for future rounds**, NOT bluffs — CONST-048 (Six Invariants) tolerates documented gaps in the ledger only when the gap is explicit, dated, and owner-assigned. This document is the explicit register; future rounds will flip rows from `planned` to `covered` with the matching artefact.

---

## Four-Layer Floor (CONST-048 invariant 6)

Per §1 of the constitution, every test artefact MUST sit on the four-layer floor:

| Layer       | AutoTemp artefact today                                                                       |
|-------------|-----------------------------------------------------------------------------------------------|
| Pre-build   | `go vet ./...`, `go build ./...` — invoked by `challenges/autotemp_runner_challenge.sh` step 1 |
| Post-build  | `go test ./... -count=1 -race` — invoked by Challenge step 2                                  |
| Runtime     | Runner-contract probe (sentinel-error + working-injection) — Challenge step 4                 |
| Paired mut. | re-install silent echo Runner via Go program, assert probe FAILs — Challenge step 5           |

A future round that adds a new test type to a `covered` row MUST extend the Challenge to keep the four-layer floor intact.

---

## Historical Bluff Removed (round-23 §11.4 audit)

This module's most consequential anti-bluff fix:

- **Before**: `baselineRunner` echoed `[band] prompt` and returned success. Forgetting `SetRunner` produced fabricated benchmark data scored against the baseline judge — every PASS in a consumer pipeline was meaningless.
- **After**: `baselineRunner` returns `ErrBaselineRunnerNotConfigured`. Absence of a real LLM Runner is LOUD; consumer code surfaces the error within one `Run` call.
- **Test guard**: `TestRunWithoutInjectedRunner_ReturnsSentinel` (`pkg/client/client_test.go`) asserts the sentinel error is returned. The round-212 Challenge mutation leg re-installs the silent echo and asserts the runtime probe FAILs — guaranteeing the fix cannot silently regress.

This is the canonical "CONST-035 PASS-bluff at the library-default layer" case study referenced in `CLAUDE.md`.

---

## Owner / Cadence

- **Owner**: AutoTemp submodule maintainer (vasic-digital). HelixCode consumers MAY contribute upstream but MUST NOT inject HelixCode-specific context (CONST-051(B)).
- **Cadence**: ledger reviewed at every governance-cascade round; planned → covered transitions land as their own commits with verbatim mandate quotes per CONST-049 §11.4.17.
