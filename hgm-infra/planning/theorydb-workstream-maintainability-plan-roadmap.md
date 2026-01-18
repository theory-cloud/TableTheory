# theorydb: maintainability-plan Roadmap (Rubric v0.1)

This document exists because maintainability-plan is currently blocking one or more rubric items. It is the execution
plan for making the workstream verifiers reliable and keeping them reliable over time.

The rubric remains the source of truth:
- the definition of “passing” does not move unless the rubric version is bumped,
- missing verifiers are **BLOCKED** (never treated as green),
- “green by dilution” fixes are not allowed (no blanket excludes; no lowered thresholds).

## Scope and blockers
- **Workstream:** maintainability-plan
- **Goal:** establish a repo-local maintainability convergence plan under `hgm-infra/planning/` and wire a deterministic verifier (MAI-2).
- **Blocking rubric IDs:** MAI-2
- **Primary verifier:** `TODO: create verifier (e.g., bash hgm-infra/verifiers/verify-maintainability-roadmap.sh)`
- **Primary evidence:** `hgm-infra/evidence/MAI-2-output.log`

## Baseline (start of remediation)
Snapshot (2026-01-17):
- Current status: BLOCKED (no maintainability roadmap/verifier under `hgm-infra/` yet)
- Failure mode(s): missing artifact + missing deterministic check
- Notes: repo currently contains maintainability plans under `docs/development/planning/`, but HGM governance is scoped to `hgm-infra/` only.

## Guardrails (no “green by dilution”)
- Do not shrink scope to make numbers look better (no new excludes, no coverage denominator games).
- Keep tool versions pinned; treat any `latest` usage as a blocker.
- Prefer fixes that reduce future drift (make the verifier deterministic, fast, and CI-friendly).
- If an exception is truly needed, scope it narrowly and document why it is low-signal.

## Progress snapshots
- Baseline (2026-01-17): MAI-2 is BLOCKED
- After WS-1 (TBD): TBD
- After WS-2 (TBD): TBD

## Milestones (small, reviewable change sets)

### WS-1 — Define maintainability plan artifact
Acceptance criteria:
- Add `hgm-infra/planning/theorydb-maintainability-roadmap.md` with at least:
  - Baseline snapshot
  - Hotspots
  - Convergence workstreams
  - References to MAI-1/MAI-2/MAI-3

### WS-2 — Implement deterministic verifier
Acceptance criteria:
- Add a verifier wired into `hgm-infra/verifiers/hgm-verify-rubric.sh` that checks:
  - file exists
  - required headings/sections exist
  - no unrendered tokens

### WS-3 — Enforce in CI
Acceptance criteria:
- CI runs `bash hgm-infra/verifiers/hgm-verify-rubric.sh` and uploads `hgm-infra/evidence/*`.

## Notes
- Update `hgm-infra/planning/theorydb-10of10-roadmap.md` once MAI-2 changes from BLOCKED to PASS.
