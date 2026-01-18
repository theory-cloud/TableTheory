# K3 Lint Green Roadmap

Goal: get to a green `make lint` pass (golangci-lint config: `.golangci-v2.yml`) via milestone-driven, small, reviewable change sets.

## Status

- [x] Milestone 1: Hygiene fixes (format, spelling, context, trivial perf)
- [x] Milestone 2: `goconst` (remove repeated literal strings)
- [x] Milestone 3: `revive` cleanup (mostly unused params)
- [x] Milestone 4: `errcheck` (stop ignoring errors)
- [x] Milestone 5: `govet` (`shadow`)
- [x] Milestone 6: `govet` (`fieldalignment`)
- [x] Milestone 7: Refactors for duplication + complexity (`dupl`, `gocognit`, `gocyclo`, remaining `gocritic`)

Milestone 7 phases:

- [x] 7A: `dupl` → 0
- [x] 7B: `gocritic` → 0
- [x] 7C: `gocognit` + `gocyclo` → 0 (current: 0 issues)

## Baseline (start of remediation)

- Total issues: 621
- By linter:
  - `govet`: 274 (225 `fieldalignment`, 48 `shadow`, 1 `nilness`)
  - `revive`: 85 (76 `unused-parameter`, 5 `indent-error-flow`, 2 `context-as-argument`, 1 `redefines-builtin-id`, 1 `if-return`)
  - `errcheck`: 78
  - `goconst`: 53
  - `gocognit`: 34
  - `gocyclo`: 24
  - `dupl`: 21
  - `godox`: 12
  - `gocritic`: 12
  - `goimports`: 11
  - `misspell`: 8
  - `noctx`: 6
  - `unconvert`: 2
  - `prealloc`: 1
- Hotspot files (most issues):
  - `internal/processors/rapid_connect/builders.go`
  - `internal/services/impl/token_service.go`
  - `cmd/lambda/main.go`
  - `internal/handlers/transaction_handler.go`
  - `pkg/cryptography/types.go`
  - `pkg/tesouro/types.go`
  - `pkg/cryptography/testing.go`

## Progress snapshots

- Baseline: total `621` issues (see above)
- After Milestone 1: total `374` issues (`revive` 85, `errcheck` 78, `govet` 72, `goconst` 48, `gocognit` 34, `gocyclo` 24, `dupl` 21, `gocritic` 12)
- After Milestone 2: total `326` issues (`revive` 85, `errcheck` 78, `govet` 72, `gocognit` 34, `gocyclo` 24, `dupl` 21, `gocritic` 12)
- After Milestone 3: total `240` issues (`errcheck` 78, `govet` 72, `gocognit` 34, `gocyclo` 24, `dupl` 21, `gocritic` 11)
- After Milestone 4: total `161` issues (`govet` 71, `gocognit` 34, `gocyclo` 24, `dupl` 21, `gocritic` 11)
- After Milestone 5: total `114` issues (`govet` 24, `gocognit` 34, `gocyclo` 24, `dupl` 21, `gocritic` 11)
- After Milestone 6: total `90` issues (`dupl` 21, `gocognit` 33, `gocyclo` 25, `gocritic` 11)
- Milestone 7 (phase A - `dupl`): total `68` issues (`gocognit` 32, `gocyclo` 25, `gocritic` 11)
- Milestone 7 (phase B - `gocritic`): total `57` issues (`gocognit` 32, `gocyclo` 25)
- Milestone 7 (phase C - cmd/config): total `51` issues (`gocognit` 29, `gocyclo` 22)
- Milestone 7 (phase C - card brand): total `49` issues (`gocognit` 29, `gocyclo` 20)
- Milestone 7 (phase C - rapid_connect client): total `46` issues (`gocognit` 28, `gocyclo` 18)
- Milestone 7 (phase C - tesouro client): total `44` issues (`gocognit` 27, `gocyclo` 17)
- Milestone 7 (phase C - tesouro processor): total `42` issues (`gocognit` 25, `gocyclo` 17)
- Milestone 7 (phase C - finix auth/capture): total `40` issues (`gocognit` 24, `gocyclo` 16)
- Milestone 7 (phase C - finix secrets): total `39` issues (`gocognit` 24, `gocyclo` 15)
- Milestone 7 (phase C - crypto config): total `38` issues (`gocognit` 24, `gocyclo` 14)
- Milestone 7 (phase C - finix reversal): total `37` issues (`gocognit` 24, `gocyclo` 13)
- Milestone 7 (phase C - finix bank completion): total `36` issues (`gocognit` 24, `gocyclo` 12)
- Milestone 7 (phase C - finix tokenization): total `35` issues (`gocognit` 23, `gocyclo` 12)
- Milestone 7 (phase C - rapid_connect TOR): total `33` issues (`gocognit` 23, `gocyclo` 10)
- Milestone 7 (phase C - rapid_connect auth void OrigAuthGrp): total `32` issues (`gocognit` 23, `gocyclo` 9)
- Milestone 7 (phase C - rapid_connect COF): total `31` issues (`gocognit` 23, `gocyclo` 8)
- Milestone 7 (phase C - rapid_connect refund request): total `30` issues (`gocognit` 23, `gocyclo` 7)
- Milestone 7 (phase C - rapid_connect L2/L3 validation): total `29` issues (`gocognit` 23, `gocyclo` 6)
- Milestone 7 (phase C - rapid_connect health expense groups): total `28` issues (`gocognit` 23, `gocyclo` 5)
- Milestone 7 (phase C - handlers auth void): total `27` issues (`gocognit` 23, `gocyclo` 4)
- Milestone 7 (phase C - handlers validation record): total `26` issues (`gocognit` 23, `gocyclo` 3)
- Milestone 7 (phase C - handlers validate ownership): total `25` issues (`gocognit` 23, `gocyclo` 2)
- Milestone 7 (phase C - token_service gocyclo): total `23` issues (`gocognit` 23, `gocyclo` 0)
- Milestone 7 (phase C - avs_service settings parsing): total `22` issues (`gocognit` 22, `gocyclo` 0)
- Milestone 7 (phase C - hash_service normalization): total `21` issues (`gocognit` 21, `gocyclo` 0)
- Milestone 7 (phase C - rapid_connect auth request refactor): total `20` issues (`gocognit` 20, `gocyclo` 0)
- Milestone 7 (phase C - rapid_connect completion call refactor): total `19` issues (`gocognit` 19, `gocyclo` 0)
- Milestone 7 (phase C - lambda entrypoint refactor): total `17` issues (`gocognit` 17, `gocyclo` 0)
- Milestone 7 (phase C - rapid_connect builders refactor): total `15` issues (`gocognit` 15, `gocyclo` 0)
- Milestone 7 (phase C - rapid_connect auth call refactor): total `14` issues (`gocognit` 14, `gocyclo` 0)
- Milestone 7 (phase C - rapid_connect reversal call refactor): total `13` issues (`gocognit` 13, `gocyclo` 0)
- Milestone 7 (phase C - rapid_connect GMF conversion refactor): total `12` issues (`gocognit` 12, `gocyclo` 0)
- Milestone 7 (phase C - rapid_connect completion request refactor): total `11` issues (`gocognit` 11, `gocyclo` 0)
- Milestone 7 (phase C - handlers reversal refactor): total `10` issues (`gocognit` 10, `gocyclo` 0)
- Milestone 7 (phase C - handlers gocognit refactor): total `5` issues (`gocognit` 5, `gocyclo` 0)
- Milestone 7 (phase C - token_service gocognit refactor): total `0` issues (green `make lint`)

## Principles

- Prefer real fixes over `//nolint`; only suppress when behavior is intentionally non-standard (and include a reason and tracking link).
- Keep each milestone’s change set focused by linter/category; avoid “mega diffs”.
- Treat `fieldalignment` and complexity refactors as higher risk; do them after the low-risk milestones.
- Use `make lint-path` to keep iteration tight, then validate with full `make lint`.

## Guardrails (no shortcuts)

- Don’t “make lint green” by changing `.golangci-v2.yml` to disable linters, raise thresholds, or add broad excludes.
- Don’t use `//nolint` as a primary strategy; any `//nolint:<linter>` must be narrowly scoped, justified, and traceable.
- Don’t hide debt by gating with `golangci-lint --new*` until after the repository is fully green.
- At the end of each milestone, run `make lint` and ensure targeted categories are at 0 and no unrelated category regressed.

## Milestones (sequence)

### Milestone 1: Hygiene fixes (format, spelling, context, trivial perf)

Focus: remove low-risk, mostly-mechanical issues first.

- Run `make lint-fix` (applies supported fixes, gofmt/goimports where possible).
- Fix `misspell` (ex: `CANCELLED` → `CANCELED`).
- Fix `noctx` (use `http.NewRequestWithContext`).
- Fix `unconvert` (remove unnecessary `string(...)` conversions).
- Fix `prealloc` (test helper slice preallocation).
- Address `goimports` findings (should largely be covered by `make lint-fix`).
- Address `godox`: move TODOs to the tracker and delete the marker (optionally keep a plain reference comment, e.g., `// Tracked in JIRA-1234`).

Acceptance criteria:

- `make lint` reports 0 issues for `misspell`, `noctx`, `unconvert`, `prealloc`, `goimports`, and `godox`.
- No new linter categories appear; no non-targeted category increases.

Status: complete.

### Milestone 2: `goconst` (remove repeated literal strings)

Focus: eliminate `goconst` without creating messy/global constants.

- Prefer existing constants when the linter points out an existing one.
- Keep constants scoped to the smallest reasonable package/file (avoid “constants dumping grounds”).
- Use typed enums where appropriate instead of stringly-typed values.

Acceptance criteria:

- `make lint` reports 0 issues for `goconst`.
- Constants introduced are package-appropriate (no global catch-all constants file) and don’t change externally visible behavior (e.g., wire formats).

### Milestone 3: `revive` cleanup (mostly unused params)

Focus: interface compliance + readability without behavior change.

- `unused-parameter`: rename unused params to `_` where the signature must remain.
- `indent-error-flow`: drop `else` after `return` / `continue`.
- `context-as-argument`: reorder function signatures to put `ctx context.Context` first (where feasible without API breakage).

Acceptance criteria:

- `make lint` reports 0 issues for `revive`.
- No exported API breaks without an explicit decision (and, if needed, staged refactors).

### Milestone 4: `errcheck` (stop ignoring errors)

Focus: correct, consistent error handling (especially around I/O and JSON).

- Add error handling for common patterns:
  - `Close()` (e.g., `resp.Body.Close()`, log/trace close errors when meaningful)
  - `json.Marshal` / `json.MarshalIndent`
  - other returned errors currently ignored
- If a call is intentionally ignored, annotate with `//nolint:errcheck` and a brief reason.

Acceptance criteria:

- `make lint` reports 0 issues for `errcheck`.
- Any `//nolint:errcheck` is line-scoped and includes a concrete reason (not “false positive”).

### Milestone 5: `govet` (`shadow`)

Focus: resolve correctness hazards before layout optimizations.

- Fix `shadow`:
  - replace accidental `:=` with `=`
  - rename inner-scope vars where needed

Acceptance criteria:

- `make lint` reports 0 `govet:shadow` findings.
- No behavior changes introduced solely to appease the linter (fix the scope/assignment, not the logic).

### Milestone 6: `govet` (`fieldalignment`)

Focus: reduce GC scan / struct size without breaking wire formats or reflect-based contracts.

- Start with internal / non-exported structs first.
- Explicitly review any exported struct reorder (can change JSON field order and other reflect-driven behavior).
- Use the analyzer tool to generate and apply edits: `fieldalignment -diff ./...` then `fieldalignment -fix ./...`.

Acceptance criteria:

- `make lint` reports 0 `govet:fieldalignment` findings.
- Any exported struct field reorder is reviewed for API/wire compatibility and explicitly called out in review notes.

### Milestone 7: Refactors for duplication + complexity (`dupl`, `gocognit`, `gocyclo`, remaining `gocritic`)

Focus: refactors that carry the most behavior-risk; do last.

- `dupl`: extract shared helpers (ex: repeated card-data extraction in `internal/handlers/transaction_handler.go`).
- `gocognit` / `gocyclo`: reduce complexity by:
  - extracting branch handlers to helpers
  - using early returns
  - switching to table-driven maps/switches where appropriate
- `gocritic`: apply remaining safe mechanical rewrites (e.g., `elseif`, `assignOp`, `ifElseChain`) where they don’t obscure intent.

Acceptance criteria:

- `make lint` is green (0 issues).
- No linter thresholds were loosened and no new blanket excludes were added to achieve green.

#### Milestone 7C backlog (complexity)

Tracking list (from `make lint`; update as we burn down):

- `gocognit` (0)
  - [x] `cmd/debug-event/main.go`: `DebugHandler` (31)
  - [x] `cmd/lambda/main.go`: `handleSQSTOREvent` (35), `main` (137)
  - [x] `cmd/stream-processor/main.go`: `(*StreamProcessor).processRecord` (36)
  - [x] `internal/config/validation.go`: `validateSecretsManagerRequirement` (33)
  - [x] `internal/handlers/payment_handler.go`: `processMigrationPaymentMethod` (43)
  - [x] `internal/handlers/reversal_handler.go`: `(*PaymentHandler).ReversalPayment` (36)
  - [x] `internal/handlers/transaction_handler.go`: `SalePayment` (72), `AuthPayment` (145), `CapturePayment` (82)
  - [x] `internal/handlers/transaction_handler_main.go`: `ProcessTransaction` (68)
  - [x] `internal/processors/finix/capture.go`: `MakePendingCaptureCall` (33)
  - [x] `internal/processors/finix/tokenization.go`: `MakeTokenizationCall` (66)
  - [x] `internal/processors/rapid_connect/builders.go`: `buildAuthRequest` (31), `buildCompletionRequest` (147), `addCardBrandData` (43), `normalizeAdditionalPurchaseData` (44)
  - [x] `internal/processors/rapid_connect/client.go`: `convertGMFToResponse` (37)
  - [x] `internal/processors/rapid_connect/processor.go`: `MakeAuthCall` (56), `MakeCompletionCall` (31), `MakeReversalCall` (71)
  - [x] `internal/processors/rapid_connect/xml_structures.go`: `convertToGMF` (106)
  - [x] `internal/processors/tesouro/extraction_helpers.go`: `extractAmountFromRequest` (33)
  - [x] `internal/processors/tesouro/processor.go`: `NewTesouroProcessor` (33)
  - [x] `internal/services/impl/avs_service.go`: `getMerchantAVSSettings` (34)
  - [x] `internal/services/impl/hash_service.go`: `NormalizePaymentData` (36)
  - [x] `internal/services/impl/token_service.go`: `TokenizePaymentMethod` (221), `MigrateToken` (64), `handleExistingPMT` (46), `buildTokenizationRequest` (63), `createProcessorInstrument` (132)
  - [x] `pkg/tesouro/client.go`: `(*Client).Execute` (43)
- `gocyclo` (0)
  - [x] `cmd/mock-tesouro/main.go`: `failureMessage` (18), `buildMockResponse` (22)
  - [x] `internal/config/validation.go`: `ValidateDeploymentReadiness` (16)
  - [x] `internal/handlers/reversal_handler.go`: `AuthVoidPayment` (19)
  - [x] `internal/handlers/transaction_handler.go`: `createValidationRecord` (17)
  - [x] `internal/handlers/transaction_handler_main.go`: `handleValidateOwnershipTransaction` (20)
  - [x] `internal/processors/finix/authorization.go`: `MakeAuthCall` (16)
  - [x] `internal/processors/finix/reversal.go`: `MakeReversalCall` (22)
  - [x] `internal/processors/finix/sale.go`: `MakeBankAccountCompletionCall` (25)
  - [x] `internal/processors/finix/utils.go`: `loadFinixSecrets` (17)
  - [x] `internal/processors/helpers.go`: `(*TransactionHelper).DetermineCardBrand` (19)
  - [x] `internal/processors/rapid_connect/builders.go`: `buildOrigAuthGrpForAuthVoid` (16)
  - [x] `internal/processors/rapid_connect/builders.go`: `handleCOF` (23)
  - [x] `internal/processors/rapid_connect/builders.go`: `buildRefundRequest` (21)
  - [x] `internal/processors/rapid_connect/builders.go`: `validateAdditionalPurchaseData` (21)
  - [x] `internal/processors/rapid_connect/builders.go`: `applyHealthExpenseSettings` (17)
  - [x] `internal/processors/rapid_connect/card_brand.go`: `DetermineCardBrand` (26)
  - [x] `internal/processors/rapid_connect/client.go`: `(*Client).SendRequest` (17), `mapResponseCodeToText` (17)
  - [x] `internal/processors/rapid_connect/tor.go`: `buildTORReversalRequest` (19), `extractOrigAuthGrpForTOR` (17)
  - [x] `internal/services/impl/token_service.go`: `buildTokenizationRequestFromMessage` (17), `parseExpDateFromInterface` (17)
  - [x] `pkg/cryptography/service.go`: `validateProductionCryptoConfig` (20)
  - [x] `pkg/tesouro/client.go`: `handleGraphQLErrors` (17)

## Helpful commands

- Full lint: `make lint`
- Auto-fix: `make lint-fix`
- Narrow scope: `make lint-path LINT_PATH=internal/services/impl/token_service.go`
- `fieldalignment` (for the `govet` `fieldalignment` findings):
  - preview: `fieldalignment -diff ./...`
  - apply: `fieldalignment -fix ./...`

## After we’re green (recommended)

- Align/pin the golangci-lint version used locally vs CI (Makefile currently echoes v2.3.0, but the actual version may differ).
- Add a “new issues only” gate (`golangci-lint run --new-from-rev=...`) if we want to prevent regressions without reintroducing a large baseline.
