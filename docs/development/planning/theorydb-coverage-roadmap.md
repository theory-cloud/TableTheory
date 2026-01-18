# TableTheory: Coverage Roadmap (to 90% library coverage)

Goal: raise TableTheory “library coverage” to **≥ 90%** as measured by `bash scripts/verify-coverage.sh` (which uses `bash scripts/coverage.sh` and excludes `examples/` + `tests/` from the denominator).

Prerequisite: `make lint` is green (finish the lint roadmap first). Once coverage work starts, keep `make lint` green after every coverage pass so tests do not accumulate unreviewed lint debt.

## Current state

Snapshot (2026-01-10):

- Prerequisite check: ✅ `make lint` is green (M1 complete)
- `bash scripts/verify-coverage.sh`: ✅ **90.0%** vs threshold **90%** (passes)
- `bash scripts/verify-coverage-packages.sh --targets docs/development/planning/coverage-targets/theorydb-cov-5.tsv`: ✅ (80% floors)

## Progress snapshots

- Baseline (2026-01-10): **51.0%** vs threshold **90%** (fails)
- After COV-1 (2026-01-10): **52.6%** vs threshold **90%** (fails); removed 0%-coverage packages (`internal/reflectutil`, `pkg/testing`)
- After COV-2 (2026-01-10): **53.1%** vs threshold **90%** (fails); package floors enforced via `bash scripts/verify-coverage-packages.sh --targets docs/development/planning/coverage-targets/theorydb-cov-2.tsv`
- After COV-3 (2026-01-10): **64.3%** vs threshold **90%** (fails); package floors raised to 50% via `bash scripts/verify-coverage-packages.sh --targets docs/development/planning/coverage-targets/theorydb-cov-3.tsv`
- After COV-4 (2026-01-10): **75.8%** vs threshold **90%** (fails); package floors raised to 70% via `bash scripts/verify-coverage-packages.sh --targets docs/development/planning/coverage-targets/theorydb-cov-4.tsv`
- After COV-5 (2026-01-10): **82.8%** vs threshold **90%** (fails); package floors raised to 80% via `bash scripts/verify-coverage-packages.sh --targets docs/development/planning/coverage-targets/theorydb-cov-5.tsv`
- After COV-6 (2026-01-10): ✅ **90.0%** vs threshold **90%** (passes); global gate now green

## Guardrails (no denominator games)

- Do not reduce the measurement surface by excluding additional production packages from `scripts/coverage.sh`.
- Do not claim coverage progress by moving logic into `examples/` or `tests/`.
- Keep `COVERAGE_THRESHOLD` as a *raise-only* override (the verifier rejects lowering below default).
- Keep formatting and lint green after each pass: `bash scripts/fmt-check.sh` and `make lint`.

## How we measure

Generate a coverage profile:

```bash
bash scripts/coverage.sh
```

Verify package-level milestone floors (prevents “we forgot to test pkg X”):

```bash
bash scripts/verify-coverage-packages.sh --targets docs/development/planning/coverage-targets/theorydb-cov-4.tsv
```

Final rubric gate (overall threshold, default 90%):

```bash
bash scripts/verify-coverage.sh
```

Regression checks (run after each coverage change set):

```bash
bash scripts/fmt-check.sh
make lint
```

## Workstreams

### 1) Stabilize the test harness (make adding tests cheap)

- Prefer table-driven unit tests for pure/near-pure packages.
- For DynamoDB behavior, keep integration tests focused and deterministic (fixtures, teardown, no sleeps).

### 2) Target the highest-leverage code paths first

Initial hotspots to prioritize (high churn / high surface area):

- `theorydb.go`
- `pkg/core`
- `pkg/query`
- `pkg/marshal`
- `pkg/schema`

### 3) Close common gap patterns

- Error paths (DynamoDB conditional failures, throttling/retry paths, marshaling failures).
- Option handling (zero values, `omitempty`, conditional clauses, return values).
- Edge cases (empty collections, nested structs, pointer fields).

## Proposed milestones

This repo currently has a single hard gate at 90%. To make progress reviewable, adopt incremental milestones (modeled after K3) such as:

- ✅ COV-1: remove “0% islands” (every production package has tests)
- ✅ COV-2: broad floor (25%+ across production packages)
- ✅ COV-3: meaningful safety net (50%+)
- ✅ COV-4: high confidence (70%+)
- ✅ COV-5: pre-finish (80%+)
- ✅ COV-6: finish line (90%+ + pass `bash scripts/verify-coverage.sh`)
