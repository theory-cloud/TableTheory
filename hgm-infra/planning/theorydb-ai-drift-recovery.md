# AI/Agent Drift: Failure Modes + Recovery (theorydb)

Document common failure modes when using LLMs/agents and the guardrails to prevent “green by dilution.”

## Common failure modes
- **Green by exclusion:** making checks pass by shrinking scope (skip tests, exclude directories, loosen thresholds).
- **Toolchain drift:** CI runs different versions than `go.mod`/config (non-repeatable results).
- **Scope dodging:** only validating the root module while nested modules/examples are broken.
- **Evidence drift:** docs claim controls exist but no verifier/evidence path backs them.
- **Metadata-only tags/flags:** security affordances (e.g., `encrypted`, `redacted`) without enforced semantics.
- **Public boundary drift:** exported helpers diverge from canonical semantics; contract tests absent.
- **Maintainability erosion:** “god files” and duplicate implementations that make future fixes risky.

## Guardrails (what to enforce)
- Use the versioned rubric as the source of truth; bump version on any rubric change.
- Keep Completeness/anti-drift checks in CI: toolchain pins, schema-valid configs, coverage/security threshold floors, multi-module health, logging/operational standards.
- Prefer narrow suppressions (`//nolint`, `#nosec`) with justification; avoid blanket excludes.
- Enforce semantics for security affordance tags/flags (fail closed if misconfigured).
- Maintain public API contract parity (exported helpers vs canonical semantics).
- Keep maintainability budgets (file size/complexity) and convergence plans current.

## Recovery playbook
1. Re-run the full rubric surface and capture outputs.
2. Fix failing gates before reducing scope; if scope must shrink, time-box and document explicitly.
3. If a verifier is wrong/flaky, fix the verifier and bump rubric version (no silent loosening).
4. Update roadmap + evidence plan when controls move or new risks appear.
5. Refresh the signature bundle (`hgm sign`) after material changes to controls/rubric/roadmap.

## Turning discoveries into durable gates (recommended)
When a new class of failure is discovered (security, quality, compliance, or AI drift), treat it as “candidate rubric
surface”, not a one-off note:
1) **Propose** a new verifier (what it checks, why it matters, how it avoids false-green/false-red).
2) **Implement** the verifier behind a standalone command.
3) **Adopt** it by bumping the rubric version, wiring it into CI, and adding it to the evidence plan.
