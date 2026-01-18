# K3: Coverage Roadmap (Package-by-Package)

This document is a dedicated execution plan for raising Go coverage in K3 **package-by-package** with **hard, enforceable**
acceptance criteria at each step.

This roadmap is intentionally separate from `docs/planning/k3-10of10-roadmap.md`: its job is to make the coverage push
concrete and unavoidable (no “we’ll do 90% later”).

## Guardrails (no hiding)

Coverage work must not be “made easier” by shrinking the measurement surface.

Hard rules:

- **Do not reduce the denominator** by excluding production packages from coverage (`-coverpkg` must continue to include the root-module production packages).
- **Do not hide packages** by removing them from targets files.
- **Any new production package must be added** to the current targets file in the same PR (or the verifier will fail).

Enforcement mechanism:

- `bash scripts/verify-coverage-packages.sh` fails if:
  - a package appears in `coverage.out` but is missing from the targets file (prevents omission), or
  - a package is in the targets file but missing from `coverage.out` (catches coverpkg drift).

## How we measure (source of truth)

1) Generate the coverage profile (unit + integration):
```bash
bash scripts/coverage.sh
```

2) Verify package-level milestone thresholds:
```bash
bash scripts/verify-coverage-packages.sh --targets docs/planning/coverage-targets/k3-cov-1.tsv
```

3) Final rubric gate (overall threshold, currently 90%):
```bash
bash scripts/verify-coverage.sh
```

## Current state (snapshot)

Current state (2026-01-07):

- Total coverage: **90.64%** (covered **9,796** / total **10,807** statements)
- Package floors: ✅ `docs/planning/coverage-targets/k3-cov-6.tsv` passes (all packages ≥ 90%)
- Overall gate: ✅ `bash scripts/verify-coverage.sh`

## Baseline (snapshot, pre-coverage push)

Starting point (2026-01-06):

- Total coverage: **29.5%** (covered **3,253** / total **11,018** statements)
- Largest uncovered statement gaps:
  - `internal/processors/rapid_connect` uncovered **1,510**
  - `internal/services/impl` uncovered **1,449**
  - `internal/handlers` uncovered **900**
  - `internal/processors/finix` uncovered **571**
  - `pkg/cryptography` uncovered **493**

## Milestones (hard gates)

Each milestone is “done” only when its targets file passes.

### COV-1 — No-zero baseline (all packages have coverage)

**Goal:** eliminate “0% islands” so no package is untested.

**Status:** ✅ complete (2026-01-06)

**Targets (summary)**
- `cmd/*`: ≥ 5%
- everything else: ≥ 10% (except `internal/constants`: ≥ 90%)

**Acceptance criteria**
```bash
bash scripts/coverage.sh
bash scripts/verify-coverage-packages.sh --targets docs/planning/coverage-targets/k3-cov-1.tsv
```

---

### COV-2 — Broad floor (25%+ across non-cmd packages)

**Goal:** get every production package out of the “token tests” zone.

**Status:** ✅ complete (2026-01-06)

**Targets (summary)**
- `cmd/*`: ≥ 10%
- everything else: ≥ 25% (except `internal/constants`: ≥ 90%)

**Acceptance criteria**
```bash
bash scripts/coverage.sh
bash scripts/verify-coverage-packages.sh --targets docs/planning/coverage-targets/k3-cov-2.tsv
```

---

### COV-3 — Meaningful safety net (50%+ across non-cmd packages)

**Goal:** make refactors meaningfully safer by covering most branches in core code paths.

**Status:** ✅ complete (2026-01-07)

**Targets (summary)**
- `cmd/*`: ≥ 25%
- everything else: ≥ 50% (except `internal/constants`: ≥ 90%)

**Acceptance criteria**
```bash
bash scripts/coverage.sh
bash scripts/verify-coverage-packages.sh --targets docs/planning/coverage-targets/k3-cov-3.tsv
```

---

### COV-4 — High confidence (70%+ across non-cmd packages)

**Goal:** drive coverage deep enough that common regressions are caught quickly.

**Status:** ✅ complete (2026-01-07)

**Targets (summary)**
- `cmd/*`: ≥ 50%
- everything else: ≥ 70% (except `internal/constants`: ≥ 90%)

**Acceptance criteria**
```bash
bash scripts/coverage.sh
bash scripts/verify-coverage-packages.sh --targets docs/planning/coverage-targets/k3-cov-4.tsv
```

---

### COV-5 — Pre-10/10 readiness (80%+ across non-cmd packages)

**Goal:** get close enough to 90% that the final push is tractable and time-boxable.

**Status:** ✅ complete (2026-01-07)

**Targets (summary)**
- `cmd/*`: ≥ 70%
- everything else: ≥ 80% (except `internal/constants`: ≥ 90%)

**Acceptance criteria**
```bash
bash scripts/coverage.sh
bash scripts/verify-coverage-packages.sh --targets docs/planning/coverage-targets/k3-cov-5.tsv
```

---

### COV-6 — Finish line (90% package floors + overall 90% gate)

**Goal:** reach the final bar with no weak packages remaining.

**Status:** ✅ complete (2026-01-07)

**Targets (summary)**
- all packages: ≥ 90%

**Acceptance criteria**
```bash
bash scripts/coverage.sh
bash scripts/verify-coverage-packages.sh --targets docs/planning/coverage-targets/k3-cov-6.tsv
bash scripts/verify-coverage.sh
```
