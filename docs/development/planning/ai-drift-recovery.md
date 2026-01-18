# AI Drift: Failure Modes + Recovery (TableTheory)

This document records common failure modes when iterating on an AI-generated codebase and the guardrails we use to
recover without creating false confidence.

## Common failure modes

- **Green by exclusion:** “fixing” failures by excluding directories, disabling linters, lowering thresholds, or skipping tests.
- **Toolchain drift:** CI running different Go/tool versions than local, producing non-repeatable results.
- **Scope dodging:** only validating the root module while leaving nested modules/examples broken.
- **Evidence drift:** docs claim a control exists but there is no durable verifier/evidence path.

## Guardrails (what we enforce)

- Use the versioned rubric as the source of truth: `docs/development/planning/theorydb-10of10-rubric.md`.
- Keep “meta” checks that verify the verifiers (COM-*), not just the code.
- Prefer narrow suppressions (`//nolint` / `#nosec`) with a reason over broad excludes.

## Recovery playbook

1. Re-run the full rubric surface locally (or in CI) and capture output.
2. If a gate is failing, fix the underlying issue first; avoid scope reduction unless explicitly time-boxed.
3. If a verifier is wrong or flaky, fix the verifier and bump the rubric version (no silent changes).
4. Update roadmap + evidence plan when controls move or new risk surfaces appear.

