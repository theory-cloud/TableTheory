# TableTheory: Lint Green Roadmap

Goal: get to a green `make lint` pass using the repo’s strict golangci-lint v2 configuration (`.golangci-v2.yml`), without weakening thresholds or adding blanket exclusions.

## Baseline (start of remediation)

Snapshot (2026-01-09):

- Total issues: **728**
- By linter:
  - `errcheck`: 326
  - `govet`: 171 (mostly `fieldalignment`)
  - `revive`: 87 (mostly `unused-parameter`)
  - `goimports`: 52
  - `gocognit`: 27
  - `gocyclo`: 25
  - `dupl`: 15
  - `misspell`: 8
  - `gocritic`: 7
  - `goconst`: 5
  - `prealloc`: 3
  - `gosec`: 2 (`G115` in `tests/stress/concurrent_test.go`)

Known warning:

- `Found unknown linters in //nolint directives: unusedparams, unusedwrite` (stale directives; must be fixed or removed).

## Progress snapshots

- Baseline (2026-01-09): **728** issues
- After Milestone 1 (2026-01-09): **666** issues (removed `goimports`, `misspell`, `gosec`, and stale `//nolint` warnings)
- After Milestone 2 (2026-01-09): **579** issues (removed all `revive` findings; deferred context signature reorders via narrow `//nolint:revive` for compatibility)
- After Milestone 3 (2026-01-09): **253** issues (removed all `errcheck` findings)
- After Milestone 4 (2026-01-09): **84** issues (removed all `govet` findings, primarily `fieldalignment`)
- After Milestone 5 (2026-01-10): **0** issues (`make lint` green)

Current remaining (after Milestone 5): none.

## Guardrails (no “green by exclusion”)

- Do not loosen `.golangci-v2.yml` thresholds to make the numbers go down.
- Do not add new blanket excludes (directory-wide or linter-wide) unless we document why the scope is truly out-of-signal.
- If an error is intentionally ignored, annotate it with a **line-scoped** `//nolint:errcheck` and a concrete reason.

## Milestones (small, reviewable change sets)

### Milestone 1 — Hygiene and mechanical fixes

Focus: reduce noise fast with low behavior risk.

- Fix `goimports` formatting (run `golangci-lint fmt` or apply `goimports`/`gofmt` across flagged files).
- Fix `misspell`.
- Fix/replace stale `//nolint` tags (`unusedparams`, `unusedwrite`).
- Fix the two `gosec` `G115` findings in `tests/stress/concurrent_test.go` (clamp or validate before converting).

Done when:
- `make lint` issue count drops meaningfully without changing linter policy.

### Milestone 2 — `revive` cleanup (API-safe)

Focus: rules that are usually mechanical and low risk.

- `unused-parameter`: rename unused parameters to `_` (or remove if not part of an interface).
- `context-as-argument`: reorder signatures to put `context.Context` first **when it is not a breaking API change** (otherwise justify and suppress narrowly).
- `if-return`: simplify redundant `if err != nil { return err }` patterns.

Done when:
- `revive` findings reach 0 (or only narrowly-justified suppressions remain).

### Milestone 3 — `errcheck` (stop ignoring errors)

Focus: correctness and durability.

- Handle returned errors from SDK calls, writers/closers, and test helpers.
- If a call is intentionally ignored, document why with line-scoped `//nolint:errcheck`.

Done when:
- `errcheck` findings reach 0.

### Milestone 4 — `govet:fieldalignment` (struct layout)

Focus: mechanical reorderings with compatibility review.

- Start with non-exported structs.
- Review exported struct reorders for any reflect-/encoding-sensitive behavior.

Done when:
- `govet` findings reach 0.

### Milestone 5 — Refactors for duplication and complexity

Focus: highest behavior risk; do last.

- `dupl`: extract shared helpers.
- `gocognit` / `gocyclo`: reduce complexity by extracting helpers and using early returns.
- Remaining `gocritic` / `goconst`: apply safe rewrites.

Done when:
- `make lint` is green (0 issues).

## Helpful commands

```bash
golangci-lint config verify -c .golangci-v2.yml
make lint

# JSON output for tracking progress between milestones
golangci-lint run --timeout=5m --config .golangci-v2.yml --output.json.path /tmp/golangci.json ./... || true
```
