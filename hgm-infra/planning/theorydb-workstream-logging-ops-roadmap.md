# theorydb: logging-ops Roadmap (Rubric v0.1)

This document exists because logging-ops is currently blocking one or more rubric items. It is the execution
plan for making the workstream verifiers reliable and keeping them reliable over time.

The rubric remains the source of truth:
- the definition of “passing” does not move unless the rubric version is bumped,
- missing verifiers are **BLOCKED** (never treated as green),
- “green by dilution” fixes are not allowed (no blanket excludes; no lowered thresholds).

## Scope and blockers
- **Workstream:** logging-ops
- **Goal:** define the logging/operational standards that apply to Theorydb as a library, then implement deterministic gates where applicable.
- **Blocking rubric IDs:** COM-6
- **Primary verifier:** `TODO: add logging/operational standards verifier`
- **Primary evidence:** `hgm-infra/evidence/COM-6-output.log`

## Baseline (start of remediation)
Snapshot (2026-01-17):
- Current status: BLOCKED (standard not defined; verifier missing)
- Failure mode(s): unclear requirements; library vs examples vs tests scoping needs to be explicit
- Notes: Theorydb includes examples and optional helpers (e.g., Lambda/multi-account). Logging requirements should be scoped to avoid false positives while still preventing sensitive data leakage.

## Guardrails (no “green by dilution”)
- Don’t “pass” by excluding large directories (examples/tests) unless you explicitly define them as out-of-scope for the control.
- Prefer explicit allowlists and narrow suppressions over blanket ignores.
- If logging is intentionally present (e.g., warnings), ensure it does not emit secrets/PII/CHD-like payloads.

## Milestones

### WS-1 — Define the standard
Acceptance criteria:
- Add `hgm-infra/planning/theorydb-logging-ops-standards.md` describing:
  - allowed logging APIs (if any)
  - prohibited patterns (logging raw structs/attribute maps)
  - rules for examples vs library code

### WS-2 — Implement deterministic verifier
Acceptance criteria:
- Add a verifier that checks:
  - root library code avoids prohibited logging patterns
  - any allowed logging is structured/redacted

### WS-3 — Wire into CI
Acceptance criteria:
- Verifier runs in the HGM rubric surface and produces evidence under `hgm-infra/evidence/`.
