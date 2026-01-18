# theorydb: Lint Green Roadmap (Rubric v0.1)

Goal: get to a green `bash scripts/verify-lint.sh` pass using the repo’s strict lint configuration, **without** weakening thresholds or
adding blanket exclusions.

This exists as a standalone roadmap because lint issues often require large, mechanical change sets that should be kept
reviewable and should not block unrelated remediation work (coverage/security/etc).

## Why this is a dedicated roadmap
- A failing linter blocks claiming CON-* and often blocks later work (tests/coverage work tends to generate lint debt).
- “Green by dilution” (disabling rules, widening excludes) is not an acceptable solution.

## Baseline (start of remediation)
Snapshot (2026-01-17):
- Primary command: `bash scripts/verify-lint.sh`
- Current status: unknown (run `bash hgm-infra/verifiers/hgm-verify-rubric.sh` to record evidence)
- Top failure sources: golangci-lint / eslint / ruff (as applicable)

## Progress snapshots
- Baseline (2026-01-17): evidence to be captured in `hgm-infra/evidence/CON-2-output.log`
- After LINT-1 (TBD): TBD
- After LINT-2 (TBD): TBD

## Guardrails (no “green by dilution”)
- Do not add blanket excludes (directory-wide or linter-wide) unless the scope is demonstrably out-of-signal.
- Prefer line-scoped suppressions with justification over disablements.
- Keep tool versions pinned (no `latest`) and verify config schema validity where supported.
- Keep formatter checks enabled so “fixes” don’t drift into style churn.

## Milestones (small, reviewable change sets)

### LINT-1 — Hygiene and mechanical fixes
Focus: reduce noise fast with low behavior risk.

Examples:
- Auto-fix formatting/imports.
- Fix typos/lint directives.
- Remove/replace stale suppressions.

Done when:
- `bash scripts/verify-lint.sh` issue count drops meaningfully without changing linter policy.

### LINT-2 — Low-risk rule families (API-safe)
Focus: rules that are typically mechanical.

Examples:
- Unused parameter renames to `_` / `_unused`.
- Simplify repetitive patterns flagged by the linter.

Done when:
- The dominant “mechanical” linter families are cleared.

### LINT-3 — Correctness and error handling
Focus: stop ignoring errors and restore durable invariants.

Done when:
- “Ignored error” findings are eliminated or narrowly justified.

### LINT-4 — Refactors for duplication and complexity
Focus: highest behavior risk; do last.

Done when:
- `bash scripts/verify-lint.sh` is green (0 issues) under the strict config.

## Helpful commands
```bash
bash scripts/verify-lint.sh

golangci-lint config verify -c .golangci-v2.yml
```
