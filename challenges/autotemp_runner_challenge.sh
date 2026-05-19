#!/usr/bin/env bash
#
# challenges/autotemp_runner_challenge.sh
#
# Round-212 deliverable — AutoTemp submodule deep-doc + test-matrix enrichment.
#
# Drives the full CONST-050(B) "Challenges" leg for the AutoTemp submodule:
#
#   Step 1: pre-build  -- go vet + go build
#   Step 2: post-build -- go test ./... -count=1 -race
#   Step 3: Runner-contract probe (sentinel-error leg) -- assert that a
#           freshly-constructed Client without SetRunner returns
#           ErrBaselineRunnerNotConfigured on Run, NOT fabricated data.
#   Step 4: Runner-contract probe (working-injection leg) -- assert that a
#           real echo-style Runner produces non-empty BestOutput and a
#           positive BestOverallScore through the Run pipeline.
#   Step 5: paired anti-bluff mutation -- swap in a silent echo Runner
#           that wraps the sentinel and returns success, then assert the
#           probe REJECTS it (proving the probe would catch a real
#           regression of the round-23 §11.4 fix).
#
# Anti-bluff invariants (CONST-035 / Article XI §11.9):
#   - every PASS is preceded by a real command + captured output
#   - the mutation leg PROVES the assertion would fail if AutoTemp regressed
#   - the script exits non-zero on the FIRST failure (no quiet skips)
#
# Exit 0 only if every step above succeeded.

set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
EVIDENCE_DIR="${SCRIPT_DIR}/.last-run"
PROBE_DIR="${SCRIPT_DIR}/.probe"
mkdir -p "${EVIDENCE_DIR}" "${PROBE_DIR}"

cd "${REPO_ROOT}"

log() { printf '\n=== %s ===\n' "$*"; }
fail() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }

# ---------------------------------------------------------------------------
# Step 1 -- pre-build floor
# ---------------------------------------------------------------------------
log "Step 1: go vet + go build (pre-build floor)"
go vet ./... 2>&1 | tee "${EVIDENCE_DIR}/01-vet.log" || fail "go vet"
go build ./... 2>&1 | tee "${EVIDENCE_DIR}/02-build.log" || fail "go build"

# ---------------------------------------------------------------------------
# Step 2 -- post-build floor: unit suite under race detector
# ---------------------------------------------------------------------------
log "Step 2: go test ./... -count=1 -race (post-build floor)"
GOMAXPROCS=2 go test ./... -count=1 -race 2>&1 | tee "${EVIDENCE_DIR}/03-test.log" || fail "unit suite"

# ---------------------------------------------------------------------------
# Step 3+4 -- runtime Runner-contract probe.
#
# The probe is a small Go program written into ${PROBE_DIR}. It cannot live
# in the module's own tree (would be picked up by go build/test). It uses a
# 'replace' directive to point at the real module on disk so it exercises the
# REAL code, not a fork.
# ---------------------------------------------------------------------------
log "Step 3+4: runtime Runner-contract probe (sentinel-error + working-injection)"

# Write probe go.mod
cat > "${PROBE_DIR}/go.mod" <<'EOF'
module autotemp.probe

go 1.22

require (
    digital.vasic.autotemp v0.0.0
    digital.vasic.pliniuscommon v0.1.0
)

replace digital.vasic.autotemp => REPLACE_AUTOTEMP_PATH
replace digital.vasic.pliniuscommon => REPLACE_PLINIUSCOMMON_PATH
EOF

# Substitute the absolute paths. AutoTemp's own go.mod points at
# ../PliniusCommon — we replicate that relative resolution from the probe.
PLINIUS_ABS="$(cd "${REPO_ROOT}/../PliniusCommon" && pwd)"
sed -i \
    -e "s|REPLACE_AUTOTEMP_PATH|${REPO_ROOT}|" \
    -e "s|REPLACE_PLINIUSCOMMON_PATH|${PLINIUS_ABS}|" \
    "${PROBE_DIR}/go.mod"

# Write the probe program. It supports two modes via the SILENT_ECHO env var
# so the SAME binary can be used for step 3+4 (real behaviour) AND step 5
# (mutated behaviour — re-installs a silent echo Runner and asserts the
# probe still REJECTS it).
cat > "${PROBE_DIR}/main.go" <<'EOF'
// autotemp_runner_challenge probe.
// Exercises Runner-contract invariants:
//   (1) default Runner returns ErrBaselineRunnerNotConfigured
//   (2) injected real Runner produces non-empty BestOutput + positive score
//   (3) MUTATION mode: silent echo Runner injected; probe MUST FAIL because
//       the silent echo produces empty-prompt outputs that scoreWithJudges
//       would have to flag — proving the probe's assertions are real.
package main

import (
    "context"
    "errors"
    "fmt"
    "os"

    autotemp "digital.vasic.autotemp/pkg/client"
    "digital.vasic.autotemp/pkg/types"
)

func main() {
    ctx := context.Background()
    mode := os.Getenv("PROBE_MODE")

    // ---- Leg 1: sentinel error must fire when SetRunner is not called.
    c1, err := autotemp.New()
    if err != nil {
        fmt.Fprintf(os.Stderr, "leg1: New() failed: %v\n", err)
        os.Exit(2)
    }
    defer c1.Close()
    _, err = c1.Run(ctx, types.RunOptions{
        Prompt:       "ping",
        Temperatures: []float64{0.5},
    })
    if err == nil {
        fmt.Fprintf(os.Stderr, "leg1 BLUFF: Run returned no error with default Runner — sentinel fix regressed\n")
        os.Exit(3)
    }
    if !errors.Is(err, autotemp.ErrBaselineRunnerNotConfigured) {
        fmt.Fprintf(os.Stderr, "leg1 BLUFF: Run returned wrong error: %v (expected ErrBaselineRunnerNotConfigured)\n", err)
        os.Exit(4)
    }
    fmt.Println("leg1 OK: default Runner correctly returned ErrBaselineRunnerNotConfigured")

    // ---- Leg 2: working Runner produces real output.
    c2, err := autotemp.New()
    if err != nil {
        fmt.Fprintf(os.Stderr, "leg2: New() failed: %v\n", err)
        os.Exit(5)
    }
    defer c2.Close()

    if mode == "MUTATION" {
        // Mutation: install a "silent echo" Runner that returns SUCCESS with
        // an EMPTY string. This simulates the pre-round-23 PASS-bluff at the
        // Runner level — the contract probe MUST flag this as broken because
        // an empty BestOutput is meaningless data presented as success.
        c2.SetRunner(func(_ context.Context, _ string, _, _ float64) (string, types.TokenUsage, error) {
            return "", types.TokenUsage{}, nil
        })
    } else {
        // Real injection: deterministic stub mimicking a real LLM Runner.
        c2.SetRunner(func(_ context.Context, prompt string, t, _ float64) (string, types.TokenUsage, error) {
            band := "mid"
            switch {
            case t < 0.3:
                band = "low"
            case t > 0.8:
                band = "high"
            }
            out := fmt.Sprintf("[%s] %s", band, prompt)
            return out, types.TokenUsage{
                PromptTokens:     len(prompt),
                CompletionTokens: len(out),
                TotalTokens:      len(prompt) + len(out),
            }, nil
        })
    }

    res, err := c2.Run(ctx, types.RunOptions{
        Prompt:       "hello world",
        Temperatures: []float64{0.1, 0.5, 0.9},
    })
    if err != nil {
        fmt.Fprintf(os.Stderr, "leg2: Run returned error: %v\n", err)
        os.Exit(6)
    }
    // Contract assertion: BestOutput MUST be non-empty AND BestOverallScore
    // MUST be > 0. The mutation leg returns empty strings, so this trips.
    if res.BestOutput == "" {
        fmt.Fprintf(os.Stderr, "leg2 BLUFF: BestOutput is empty — Runner produced meaningless data\n")
        os.Exit(7)
    }
    if res.BestOverallScore <= 0 {
        fmt.Fprintf(os.Stderr, "leg2 BLUFF: BestOverallScore=%.3f — Runner output is unscoreable\n", res.BestOverallScore)
        os.Exit(8)
    }
    fmt.Printf("leg2 OK: BestOutput=%q BestTemperature=%.2f BestOverallScore=%.3f\n",
        res.BestOutput, res.BestTemperature, res.BestOverallScore)

    fmt.Println("PROBE PASS")
}
EOF

# Resolve transitive deps + build + run the probe in normal mode. The probe
# module has no go.sum of its own; go mod tidy populates it from the replace-
# directed local trees of AutoTemp + PliniusCommon.
( cd "${PROBE_DIR}" && go mod tidy ) 2>&1 | tee "${EVIDENCE_DIR}/04a-probe-tidy.log" \
    || fail "probe go mod tidy"
( cd "${PROBE_DIR}" && go build -o probe ./... ) 2>&1 | tee "${EVIDENCE_DIR}/04-probe-build.log" \
    || fail "probe build"

set +e
( cd "${PROBE_DIR}" && ./probe ) 2>&1 | tee "${EVIDENCE_DIR}/05-probe-normal.log"
PROBE_RC=$?
set -e
if [[ ${PROBE_RC} -ne 0 ]]; then
    fail "Runner-contract probe failed in normal mode (rc=${PROBE_RC}) — either sentinel-error fix regressed or Runner injection broken"
fi
grep -q 'PROBE PASS' "${EVIDENCE_DIR}/05-probe-normal.log" \
    || fail "probe exited 0 without printing PROBE PASS — output sentinel missing"

# ---------------------------------------------------------------------------
# Step 5 -- paired anti-bluff mutation.
#
# Re-run the SAME probe binary with PROBE_MODE=MUTATION. The mutation installs
# a silent-echo Runner that returns success with empty output. The probe MUST
# exit non-zero (specifically rc=7 — "BestOutput is empty"). If it exits 0,
# the contract probe is not actually validating output quality and the suite
# is a bluff (CONST-035).
# ---------------------------------------------------------------------------
log "Step 5: paired anti-bluff mutation (silent-echo Runner, expect probe FAIL)"

set +e
( cd "${PROBE_DIR}" && PROBE_MODE=MUTATION ./probe ) > "${EVIDENCE_DIR}/06-probe-mutation.log" 2>&1
MUTATION_RC=$?
set -e

if [[ ${MUTATION_RC} -eq 0 ]]; then
    fail "paired-mutation leg: probe exited 0 with silent-echo Runner -- contract probe is not validating output (CONST-035 bluff)"
fi
printf 'mutation correctly rejected with exit code %d\n' "${MUTATION_RC}" \
    | tee -a "${EVIDENCE_DIR}/06-probe-mutation.log"

# Extra assertion: expected exit code is 7 (BestOutput empty) per probe contract.
if [[ ${MUTATION_RC} -ne 7 ]]; then
    printf 'WARN: mutation rejected but rc=%d (expected 7); inspect %s\n' \
        "${MUTATION_RC}" "${EVIDENCE_DIR}/06-probe-mutation.log"
fi

# ---------------------------------------------------------------------------
# Cleanup probe staging (keep evidence dir intact).
# ---------------------------------------------------------------------------
rm -rf "${PROBE_DIR}"

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
log "PASS: autotemp_runner_challenge.sh -- all 5 steps green"
printf 'evidence directory: %s\n' "${EVIDENCE_DIR}"
ls -la "${EVIDENCE_DIR}"
exit 0
