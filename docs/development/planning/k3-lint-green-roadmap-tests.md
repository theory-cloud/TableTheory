# K3 Lint Green Roadmap (tests enabled)

Goal: get to a green `make lint` pass with tests included (golangci-lint config: `.golangci-v2.yml` with `run.tests: true`) via milestone-driven, small, reviewable change sets.

This roadmap is intentionally separate from `docs/planning/k3-lint-green-roadmap.md` (which tracked the pre-tests baseline).

## Status

- [x] Milestone 1: Hygiene in tests (`goimports`, `misspell`, `noctx`, `unconvert`, `bodyclose`)
- [x] Milestone 2: `errcheck` in tests (type assertions + ignored errors)
- [x] Milestone 3: `govet` (tests: `shadow`, `unusedwrite`)
- [x] Milestone 4: `govet` (tests: `fieldalignment`)
- [x] Milestone 5: `gosec` in tests
- [x] Milestone 6: `gocritic` in tests
- [x] Milestone 7: test refactors for complexity (`gocognit`)

## Baseline (tests now included)

Captured from `make lint` after enabling test analysis.

- Total issues: `404`
- By linter:
  - `errcheck`: `322`
  - `govet`: `52`
  - `goimports`: `9`
  - `gosec`: `7`
  - `misspell`: `4`
  - `gocognit`: `3`
  - `gocritic`: `3`
  - `noctx`: `2`
  - `bodyclose`: `1`
  - `unconvert`: `1`

Hotspot files (most findings):

- `cmd/lambda/main_test.go` (22)
- `internal/handlers/transaction_handler_main_additional_test.go` (22)
- `internal/processors/tesouro/tokenization_reversal_ownership_additional_test.go` (22)
- `internal/processors/tesouro/coverage_additional_test.go` (15)
- `internal/services/impl/async_operation_queue_test.go` (13)
- `internal/models/token_test.go` (11)
- `internal/services/impl/authorization_service_test.go` (11)
- `pkg/cryptography/adapters_additional_test.go` (11)
- `internal/services/impl/token_service_finix_instrument_additional_test.go` (10)
- `pkg/tesouro/retry_test.go` (8)

## Progress snapshots

- Baseline: total `404` issues (`errcheck` 322, `govet` 52, `goimports` 9, `gosec` 7, `misspell` 4, `gocognit` 3, `gocritic` 3, `noctx` 2, `bodyclose` 1, `unconvert` 1)
- After Milestone 1: total `387` issues (`errcheck` 322, `govet` 52, `gosec` 7, `gocognit` 3, `gocritic` 3)
- After Milestone 2A: total `210` issues (`errcheck` 145, `govet` 52, `gosec` 7, `gocognit` 3, `gocritic` 3)
- After Milestone 2B: total `143` issues (`errcheck` 77, `govet` 52, `gosec` 7, `gocognit` 4, `gocritic` 3)
- After Milestone 2C: total `88` issues (`errcheck` 22, `govet` 52, `gosec` 7, `gocognit` 4, `gocritic` 3)
- After Milestone 2D: total `68` issues (`errcheck` 0, `govet` 52, `gosec` 7, `gocognit` 4, `gocritic` 3, `staticcheck` 2)
- After Milestone 2D (cleanup): total `66` issues (`errcheck` 0, `govet` 52, `gosec` 7, `gocognit` 4, `gocritic` 3)
- After Milestone 3: total `55` issues (`errcheck` 0, `govet` 41, `gosec` 7, `gocognit` 4, `gocritic` 3)
- After Milestone 4: total `14` issues (`gosec` 7, `gocognit` 4, `gocritic` 3)
- After Milestone 5: total `7` issues (`gocognit` 4, `gocritic` 3)
- After Milestone 6: total `4` issues (`gocognit` 4)
- After Milestone 7: total `0` issues (green `make lint`)

## Guardrails (no shortcuts)

- Don’t “make lint green” by weakening `.golangci-v2.yml` (don’t disable linters, raise thresholds, or add broad excludes).
- Don’t use blanket `//nolint`/`// #nosec`; any suppression must be narrowly scoped, justified, and exceptional.
- Keep diffs reviewable: prefer scoped changes and helper extraction over mass rewrites.
- Preserve test intent and assertions; refactor structure without changing test semantics.

## Milestones (sequence)

### Milestone 1: Hygiene in tests (`goimports`, `misspell`, `noctx`, `unconvert`, `bodyclose`)

Focus: low-risk, mostly mechanical fixes.

Scope highlights:

- `goimports` formatting failures (9 files)
- spelling: `CANCELLED` → `CANCELED`, `cancelled` → `canceled` (4)
- `http.NewRequest` → `http.NewRequestWithContext` (2)
- remove unnecessary `string(...)` conversion (1)
- close response bodies in tests (1)

Acceptance criteria:

- `make lint` reports 0 findings for: `goimports`, `misspell`, `noctx`, `unconvert`, `bodyclose`.

### Milestone 2: `errcheck` in tests

Focus: stop ignoring errors (and type-assertion results) in tests; this is the largest bucket.

Key patterns observed:

- unchecked type assertions (188) — e.g. `v := x.(T)` and `m["k"].([]T)`
- unchecked `(*json.Encoder).Encode` (57)
- unchecked writes in `httptest` handlers: `fmt.Fprintf`/`fmt.Fprint`/`w.Write` (49 total)
- unchecked closes: `r.Body.Close` (5) + response bodies
- unchecked `processors.Register` (7) and other “it should never fail” calls

Suggested phases (to keep commits reviewable):

- **2A: Type assertions**
  - Fix single-value type assertions and chained map assertions by using `v, ok := ...` and asserting `ok` (or failing the test) before using `v`.
- **2B: JSON encode/decode**
  - Check `Encode`, `Unmarshal`, `io.ReadAll`, regex helpers, etc.
- **2C: HTTP handler writes/closes**
  - Check write errors and close errors in test servers.
- **2D: Processor registration and other returns**
  - Check `processors.Register` and any remaining ignored errors.

Phase status:

- [x] **2A:** Type assertions
- [x] **2B:** JSON encode/decode
- [x] **2C:** HTTP handler writes/closes
- [x] **2D:** Processor registration and other returns

Acceptance criteria:

- `make lint` reports 0 `errcheck` findings.

### Milestone 3: `govet` (tests: `shadow`, `unusedwrite`)

Focus: correctness warnings that can hide bugs in tests.

Current known findings:

- `shadow` in `internal/services/impl/encryption_service_test.go`
- `unusedwrite` (mostly in `internal/models/token_test.go`)

Acceptance criteria:

- `make lint` reports 0 `govet:shadow` and 0 `govet:unusedwrite` findings.

### Milestone 4: `govet` (tests: `fieldalignment`)

Focus: mechanical reorders in test-only structs (table-driven test case structs and helper stubs).

Notes:

- `cmd/lambda/main_test.go` existed locally but was git-ignored due to a broad `.gitignore` pattern (`lambda`). This made lint non-reproducible across machines.
- Updated `.gitignore` to ignore `/lambda` (root artifact only) and added `cmd/lambda/main_test.go` to the repo so lint reflects the committed tree.

Acceptance criteria:

- `make lint` reports 0 `govet:fieldalignment` findings.

### Milestone 5: `gosec` in tests

Focus: eliminate weak crypto usage and unsafe file patterns in tests.

Findings addressed:

- `internal/processors/rapid_connect/tor_lambda_additional_test.go`: `crypto/md5` + weak primitive (`G501`, `G401`)
- `internal/processors/rapid_connect/tor_sqs_additional_test.go`: `crypto/md5` + weak primitive (`G501`, `G401`)
- `internal/services/processor_instrument_queue_service_test.go`: `crypto/md5` + weak primitive (`G501`, `G401`)
- `test/security/logging_compliance_test.go`: variable-path file read (`G304`)

Notes:

- Removed `crypto/md5` usage from test SQS stubs by switching to fake SQS clients (unit tests should not rely on AWS SDK checksum behavior or HTTP stubs).
- Reworked `test/security/logging_compliance_test.go` to walk/open files via `io/fs` (`os.DirFS` + `fs.WalkDir` + `fs.FS.Open`) to address `G304` without suppressions.

Acceptance criteria:

- `make lint` reports 0 `gosec` findings (prefer code fixes over `// #nosec`).

### Milestone 6: `gocritic` in tests

Current known findings:

- `pkg/cryptography/service_test.go`: `ifElseChain`
- `pkg/money/money_test.go`: `dupArg`
- `test/security/pci_compliance_test.go`: `ifElseChain`

Notes:

- Replaced `if/else if` prefix chains with `switch` statements in test-only parsers.
- Avoided `dupArg` in `Money.Equals` test by comparing distinct variables (`a.Equals(b)`).

Acceptance criteria:

- `make lint` reports 0 `gocritic` findings.

### Milestone 7: test refactors for complexity (`gocognit`)

Current known findings:

- `internal/processors/finix/digital_wallet_token_error_additional_test.go`: `TestFinixProcessor_CreateTokenForDigitalWallet_ErrorBranches` (38)
- `internal/processors/finix/tokenization_branch_coverage_more_test.go`: `TestFinixProcessor_MakeTokenizationCall_AdditionalBranches` (35)
- `pkg/errors/errors_test.go`: `TestPayTheoryErrorMarshalJSON` (45)
- `test/security/logging_compliance_test.go`: `testCriticalViolationDetection` (32)

Notes:

- Split high-complexity tests into smaller top-level tests and extracted helpers to keep each function’s cognitive complexity under the `gocognit` threshold.
- Kept semantics intact (same branch coverage intent) while reducing nested control flow.

Acceptance criteria:

- `make lint` reports 0 `gocognit` findings.

## Workflow (repeat per milestone)

- Run `make lint` (or `make lint-path LINT_PATH=...`) to confirm scope.
- Fix issues without changing `.golangci-v2.yml` and without blanket suppressions.
- Update this document’s Status + Progress snapshots.
- Commit + push after each milestone (or a clearly bounded phase within a milestone).

## Helpful commands

- Full lint: `make lint`
- Narrow scope: `make lint-path LINT_PATH=./path/to/file_or_dir`
- Auto-fix (use carefully): `make lint-fix`
