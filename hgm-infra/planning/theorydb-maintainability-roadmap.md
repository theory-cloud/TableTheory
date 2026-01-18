# theorydb Maintainability Roadmap (Rubric v0.1)

Goal: keep Theorydb reviewable and convergent over time by preventing “god files” and duplicated semantics.

This document exists to satisfy MAI-2: it is a versioned, repo-local convergence plan that stays current as the code
changes.

## Baseline (start of MAI work)

Snapshot (2026-01-17):
- Maintainability gates in the rubric:
  - MAI-1: `bash scripts/verify-file-size.sh`
  - MAI-2: this document + HGM verifier
  - MAI-3: `bash scripts/verify-query-singleton.sh`

## Hotspots

Primary “drift risk” hotspots (update as they change):
- Large/complex files that are hard to review safely.
- Duplicate implementations of the same semantics (especially query/expression building and marshaling paths).

## Workstreams

### WS-1 — Keep file-size budgets enforced (MAI-1)

Goal: keep review surface bounded by enforcing file-size/complexity budgets.

Gate:
- `bash scripts/verify-file-size.sh`

### WS-2 — Maintainability plan stays current (MAI-2)

Goal: keep a current plan as refactors land, so maintainability doesn’t drift silently.

Gate:
- `check_maintainability_roadmap` (via `bash hgm-infra/verifiers/hgm-verify-rubric.sh`)

Update triggers:
- Major refactors (file moves/splits, query/executor rewrites, marshaling architecture changes).
- New languages or public surfaces added to the repo.

### WS-3 — One canonical Query implementation (MAI-3)

Goal: avoid parallel semantics drift by keeping a single canonical query path.

Gate:
- `bash scripts/verify-query-singleton.sh`

## MAI rubric mapping

- MAI-1: enforced by `bash scripts/verify-file-size.sh`
- MAI-2: enforced by `check_maintainability_roadmap` (this doc must exist and include required sections)
- MAI-3: enforced by `bash scripts/verify-query-singleton.sh`

