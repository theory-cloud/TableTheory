# theorydb: Coverage Roadmap (to 90%) (Rubric v0.1)

Goal: raise and maintain meaningful coverage to **≥ 90%** as measured by `bash scripts/verify-coverage.sh`, without
reducing the measurement surface.

This exists as a standalone roadmap because coverage improvements are usually multi-PR efforts that need clear
intermediate milestones, guardrails, and repeatable measurement.

## Prerequisites
- Lint is green (or has a dedicated lint roadmap) so coverage work does not accumulate unreviewed lint debt.
- The coverage verifier is deterministic and uses a stable default threshold (no “lower it to pass” override).

## Current state
Snapshot (2026-01-17):
- Coverage gate: `bash scripts/verify-coverage.sh`
- Current result: unknown (run `bash hgm-infra/verifiers/hgm-verify-rubric.sh` to record evidence)
- Measurement surface: as implemented by `scripts/coverage.sh` and `scripts/verify-coverage.sh`

## Progress snapshots
- Baseline (2026-01-17): evidence to be captured in `hgm-infra/evidence/QUA-3-output.log`
- After COV-1 (TBD): TBD
- After COV-2 (TBD): TBD

## Guardrails (no denominator games)
- Do not exclude additional production code from the coverage denominator to “hit the number”.
- Do not move logic into excluded areas (examples/tests/generated) to claim progress.
- If package/module floors are needed, add explicit target-based verification rather than weakening the global gate.

## How we measure
Suggested flow:
1) Generate/refresh the coverage artifact with the canonical command: `bash scripts/verify-coverage.sh`
2) Re-run the full quality loop (tests + lint) as a regression gate: `bash scripts/verify-unit-tests.sh`, `bash scripts/verify-integration-tests.sh`, `bash scripts/verify-lint.sh`

## Proposed milestones (incremental, reviewable)
- COV-1: remove “0% islands” (every in-scope package/module has tests)
- COV-2: broad floor (25%+ across in-scope packages/modules)
- COV-3: meaningful safety net (50%+)
- COV-4: high confidence (70%+)
- COV-5: pre-finish (80%+)
- COV-6: finish line (≥ 90% and gate is green)

## Workstreams (target the highest-leverage paths first)
- Hotspots: identify via `go test -coverprofile=...` + package churn/usage
- Common gap patterns: error paths, option/zero-value handling, boundary validation, retry/timeouts, serialization/deserialization

## Helpful commands
```bash
bash scripts/verify-coverage-threshold.sh
bash scripts/verify-coverage.sh
bash scripts/verify-unit-tests.sh
bash scripts/verify-integration-tests.sh
bash scripts/verify-lint.sh
```
