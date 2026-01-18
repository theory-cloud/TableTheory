# TableTheory: Multi-language Verification Parity Matrix (Go / TypeScript / Python)

Goal: make “quality, consistency, completeness, security, and maintainability” **equally enforceable** across Go,
TypeScript, and Python — with **no green-by-exclusion**.

This document answers two questions:

1) **What do we measure?** (per language: library scope, unit vs integration, coverage)
2) **What is enforced today?** (rubric gate → language mapping)

Related:

- Rubric: `docs/development/planning/theorydb-10of10-rubric.md`
- Verification roadmap: `docs/development/planning/theorydb-multilang-verification-parity-roadmap.md`
- Feature parity: `docs/development/planning/theorydb-multilang-feature-parity-matrix.md`

## Measurement definitions (per language)

These definitions are intentionally explicit to prevent denominator games and “same words, different meaning”.

### Go (repo root)

- **Library code scope (coverage):** packages from `go list ./...` excluding `/examples/**`, `/tests/**`, `/scripts/**`
  (see `scripts/coverage.sh`).
- **Unit tests:** `make test-unit` (no DynamoDB Local; excludes `./tests/integration`).
- **Integration tests:** `make integration` (DynamoDB Local; `./tests/integration/...`).
- **Coverage metric:** Go tool “statements” coverage.
- **Coverage minimum:** **90%** (library coverage) enforced by `bash scripts/verify-coverage.sh`.

### TypeScript (`ts/`)

- **Library code scope (coverage):** `ts/src/**` (exclude `ts/test/**`, `ts/examples/**`, `ts/dist/**`).
- **Unit tests:** `npm --prefix ts run test:unit` (no DynamoDB Local).
- **Integration tests:** `npm --prefix ts run test:integration` (DynamoDB Local; `DYNAMODB_ENDPOINT`).
- **Coverage metrics:** line/branch/function (Node.js test coverage).
- **Coverage minimum:** **90% line coverage** of `ts/src/**` enforced by `bash scripts/verify-typescript-coverage.sh` (via `bash scripts/verify-coverage.sh`).

### Python (`py/`)

- **Library code scope (coverage):** `py/src/theorydb_py/**` (exclude `py/tests/**`, `py/examples/**`, `py/dist/**`).
- **Unit tests:** `uv --directory py run pytest -q tests/unit` (no DynamoDB Local).
- **Integration tests:** `uv --directory py run pytest -q tests/integration` (DynamoDB Local; `DYNAMODB_ENDPOINT`).
- **Coverage metric:** line coverage via `coverage.py` / `pytest-cov`.
- **Coverage minimum:** **90% line coverage** of `py/src/theorydb_py/**` enforced by `bash scripts/verify-python-coverage.sh` (via `bash scripts/verify-coverage.sh`).

## Rubric gates → language matrix (current)

Legend:

- **Enforced:** required by `make rubric` / CI
- **Measured:** collected in CI, but not a rubric gate
- **Planned:** explicitly targeted by the roadmap (not yet implemented)
- **N/A:** language-specific (e.g., Go `panic` bans)

| Rubric ID | Verification | Go | TypeScript | Python | Notes |
| --- | --- | --- | --- | --- | --- |
| QUA-1 | `bash scripts/verify-unit-tests.sh` | Enforced | Enforced | Enforced | Unit tests are “no Docker” across languages |
| QUA-2 | `bash scripts/verify-integration-tests.sh` | Enforced | Enforced | Enforced | Must be strict-by-default (no silent skips) |
| QUA-3 | `bash scripts/verify-coverage.sh` | Enforced | Enforced | Enforced | Go: statements; TS/Py: line coverage (>= 90%) |
| QUA-4 | `bash scripts/verify-validation-parity.sh` | Enforced | Planned | Planned | Currently Go-only validator/converter parity |
| QUA-5 | `bash scripts/fuzz-smoke.sh` | Enforced | Planned | Planned | Current fuzz harness is Go-only |
| CON-1 | `bash scripts/verify-formatting.sh` | Enforced | Enforced | Enforced | Go `gofmt`, TS `prettier`, Py `ruff format` |
| CON-2 | `bash scripts/verify-lint.sh` | Enforced | Enforced | Enforced | Go `golangci-lint`, TS `eslint`, Py `ruff` |
| CON-3 | `bash scripts/verify-public-api-contracts.sh` | Enforced | Planned | Planned | Contract runner exists; API contract verifier is Go-only today |
| COM-1 | `bash scripts/verify-builds.sh` | Enforced | Enforced | Enforced | Includes Go modules + TS build + Py mypy/build + version alignment |
| COM-2 | `bash scripts/verify-ci-toolchain.sh` | Enforced | Enforced | Enforced | Enforces Go toolchain pin + Node 24 + Python 3.14 pins in workflows |
| COM-3 | `bash scripts/verify-planning-docs.sh` | Enforced | Enforced | Enforced | Repo-wide |
| COM-4 | `golangci-lint config verify -c .golangci-v2.yml` | Enforced | N/A | N/A | Go-only config validation |
| COM-5 | `bash scripts/verify-coverage-threshold.sh` | Enforced | Enforced | Enforced | Ensures default thresholds stay >= 90% (raise-only) |
| COM-6 | `bash scripts/verify-ci-rubric-enforced.sh` | Enforced | Enforced | Enforced | Ensures CI runs `make rubric` and pins toolchains |
| COM-7 | `bash scripts/verify-dynamodb-local-pin.sh` | Enforced | Enforced | Enforced | Repo-wide |
| COM-8 | `bash scripts/verify-branch-release-supply-chain.sh` | Enforced | Enforced | Enforced | Repo-wide |
| SEC-1 | `bash scripts/sec-gosec.sh` | Enforced | N/A | N/A | Go-only static scan |
| SEC-2 | `bash scripts/sec-dependency-scans.sh` | Enforced | Enforced | Enforced | Go `govulncheck`, TS `npm audit`, Py `pip-audit` |
| SEC-3 | `go mod verify` | Enforced | N/A | N/A | Go-only |
| SEC-4 | `bash scripts/verify-no-panics.sh` | Enforced | N/A | N/A | Go-only |
| SEC-5 | `bash scripts/verify-safe-defaults.sh` | Enforced | Planned | Planned | TS/Py safe-defaults verifiers not yet defined |
| SEC-6 | `bash scripts/verify-expression-hardening.sh` | Enforced | Planned | Planned | Current hardening verifier is Go-only |
| SEC-7 | `bash scripts/verify-network-hygiene.sh` | Enforced | Planned | Planned | Current network hygiene verifier is Go-only |
| SEC-8 | `bash scripts/verify-encrypted-tag-implemented.sh` | Enforced | Planned | Planned | TS/Py have encryption, but KMS/AAD contract parity not yet enforced |
| MAI-1 | `bash scripts/verify-file-size.sh` | Enforced | Enforced | Enforced | File-size budgets apply equally to `go/ts/py` |
| MAI-2 | `bash scripts/verify-maintainability-roadmap.sh` | Enforced | Enforced | Enforced | Repo-wide roadmap; should call out multi-language hotspots |
| MAI-3 | `bash scripts/verify-query-singleton.sh` | Enforced | Planned | Planned | Today it enforces “one Go query implementation” |
| DOC-1 | `bash scripts/verify-planning-docs.sh` | Enforced | Enforced | Enforced | Repo-wide |
| DOC-2 | `bash scripts/verify-planning-docs.sh` | Enforced | Enforced | Enforced | Repo-wide |
| DOC-3 | `bash scripts/verify-planning-docs.sh` | Enforced | Enforced | Enforced | Repo-wide |
| DOC-4 | `bash scripts/verify-doc-integrity.sh` | Enforced | Enforced | Enforced | Repo-wide |
| DOC-5 | `bash scripts/verify-threat-controls-parity.sh` | Enforced | Enforced | Enforced | Repo-wide |

## Immediate parity gaps (what VP-1 and VP-2 fix)

- **Coverage parity:** enforced >= 90% for Go/TypeScript/Python (VP-2 complete).
- **Go-only security verifiers:** several SEC/QUA gates are Go-only (panic bans, fuzz, expression hardening). We should
  either (a) define equivalents for TS/Py or (b) explicitly scope them as Go-only and add compensating controls for other
  runtimes.
