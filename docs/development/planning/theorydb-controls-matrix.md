# TableTheory Controls Matrix (Quality + Security + Maintainability)

This controls matrix is the “requirements → controls → verifiers → evidence” backbone for TableTheory.
It is intentionally **engineering-focused** (not a compliance certification claim).

## Scope

- **System:** TableTheory (multi-language monorepo: Go + TypeScript implementations + example apps)
- **In-scope risks:** data loss/corruption via update semantics, expression misuse, unsafe reflection usage, resource/DoS risks, supply-chain drift
- **Out of scope:** IAM policy design, network perimeter controls, and application-layer auth (owned by consuming services)
- **Sensitive data note:** TableTheory may handle sensitive values (PII/tokens) as generic structs/attributes; it does not provide end-to-end encryption by default.

## Controls matrix

Threat ID note:
- Threats are enumerated as stable IDs in `docs/development/planning/theorydb-threat-model.md` (e.g., `THR-1`).
- Every `THR-*` must map to at least one control below (enforced by `bash scripts/verify-threat-controls-parity.sh`).

| Area | Threat IDs | Control ID | Requirement | Control (what we implement) | Verification (tests/gates) | Evidence (where) |
| --- | --- | --- | --- | --- | --- | --- |
| Quality | THR-1, THR-2, THR-3 | QUA-1 | Prevent regressions in core behavior | Unit tests for core behavior across implementations | `bash scripts/verify-unit-tests.sh` | CI logs + `coverage_lib.out` |
| Quality | THR-1, THR-2 | QUA-2 | Prevent integration regressions | DynamoDB Local integration suite across implementations | `bash scripts/verify-integration-tests.sh` | CI logs |
| Quality | THR-1, THR-2, THR-3 | QUA-3 | Maintain baseline coverage | Coverage threshold + repeatable runner | `bash scripts/verify-coverage.sh` | `coverage_lib.out` artifact |
| Quality | THR-2, THR-3 | QUA-4 | Validator ↔ converter parity | Harness ensures validator-accepted inputs don’t panic | `bash scripts/verify-validation-parity.sh` | CI logs |
| Quality | THR-2, THR-3 | QUA-5 | Crash resistance smoke test | Bounded fuzz pass against core primitives | `bash scripts/fuzz-smoke.sh` | CI logs |
| Consistency | — | CON-1 | Reduce review noise | Enforce formatting (gofmt + prettier) | `bash scripts/verify-formatting.sh` | CI logs |
| Consistency | — | CON-2 | Enforce static analysis | Run linters across implementations | `bash scripts/verify-lint.sh` | CI logs |
| Consistency | THR-7 | CON-3 | Public API contract parity | Contract tests/verifiers ensure exported helpers match canonical TableTheory tag/metadata semantics | `bash scripts/verify-public-api-contracts.sh` | CI logs |
| Completeness | THR-6 | COM-1 | No “mystery meat” modules | All language builds compile (Go modules + TypeScript build) | `bash scripts/verify-builds.sh` | CI logs |
| Completeness | THR-6 | COM-2 | No toolchain drift | CI Go version aligned to `go.mod` toolchain; Node version pinned for TS | `bash scripts/verify-ci-toolchain.sh` | CI logs |
| Completeness | THR-6 | COM-3 | Planning artifacts exist | Controls matrix + rubric + roadmap + evidence plan + threat model exist | `bash scripts/verify-planning-docs.sh` | Planning docs |
| Completeness | THR-6 | COM-4 | Lint config is not “green by drift” | golangci-lint config schema is valid | `golangci-lint config verify -c .golangci-v2.yml` | CI logs |
| Completeness | THR-6 | COM-5 | Coverage gate is not diluted | Coverage threshold is enforced at default | `bash scripts/verify-coverage-threshold.sh` | CI logs |
| Completeness | THR-6 | COM-6 | CI enforces rubric | Workflow runs `make rubric` with pinned tooling and uploads artifacts | `bash scripts/verify-ci-rubric-enforced.sh` | `.github/workflows/quality-gates.yml` |
| Completeness | THR-6 | COM-7 | Integration determinism | DynamoDB Local image is pinned (no `:latest`) | `bash scripts/verify-dynamodb-local-pin.sh` | `docker-compose.yml`, `Makefile` |
| Completeness | THR-6 | COM-8 | Branch/release supply chain | Release automation exists and is aligned to `premain` (prerelease) + `main` (release); protections are documented | `bash scripts/verify-branch-release-supply-chain.sh` | Repo files |
| Docs | THR-2 | DOC-1 | Security posture is reviewable | Threat model exists and is maintained | `bash scripts/verify-planning-docs.sh` | `docs/development/planning/theorydb-threat-model.md` |
| Docs | THR-6 | DOC-2 | Evidence is reproducible | Evidence plan exists and is maintained | `bash scripts/verify-planning-docs.sh` | `docs/development/planning/theorydb-evidence-plan.md` |
| Docs | THR-2 | DOC-4 | Docs integrity | No broken internal links; version claims match code | `bash scripts/verify-doc-integrity.sh` | CI logs |
| Docs | THR-6 | DOC-5 | Threat model ↔ controls parity | Every `THR-*` maps to at least one control | `bash scripts/verify-threat-controls-parity.sh` | CI logs |
| Security | THR-2, THR-3, THR-5 | SEC-1 | Baseline SAST | gosec scan is green on first-party code | `bash scripts/sec-gosec.sh` | SARIF/log output |
| Security | THR-6 | SEC-2 | Baseline dependency vuln scan | govulncheck + npm audit are green | `bash scripts/sec-dependency-scans.sh` | CLI output |
| Security | THR-6 | SEC-3 | Module integrity | Dependencies verified | `go mod verify` | CLI output |
| Security | THR-3, THR-4 | SEC-4 | Availability hardening | No `panic(...)` in production paths | `bash scripts/verify-no-panics.sh` | CI logs |
| Security | THR-3, THR-5 | SEC-5 | Safe-by-default marshaling | Unsafe marshaling is opt-in only; defaults use safe marshaling | `bash scripts/verify-safe-defaults.sh` | CI logs |
| Security | THR-2 | SEC-6 | Expression boundary hardening | Reject invalid attribute paths in expression building (including list index update syntax); no raw splices | `bash scripts/verify-expression-hardening.sh` | CI logs |
| Security | THR-4, THR-5 | SEC-7 | Network hygiene defaults | HTTP clients have timeouts; no unreviewed retry disables | `bash scripts/verify-network-hygiene.sh` | CI logs |
| Security | THR-5 | SEC-8 | Encrypted-tag semantics are real | `theorydb:"encrypted"` fails closed unless implemented + configured | `bash scripts/verify-encrypted-tag-implemented.sh` | CI logs |
| Maintainability | THR-6 | MAI-1 | Bounded review surface | Production files stay under a line-count budget (Go + TS) | `bash scripts/verify-file-size.sh` | CI logs |
| Maintainability | THR-6 | MAI-2 | Convergence plan exists | Maintainability roadmap stays current (hotspots + plan) | `bash scripts/verify-maintainability-roadmap.sh` | Planning docs |
| Maintainability | THR-6 | MAI-3 | Avoid semantics drift | One canonical Query implementation | `bash scripts/verify-query-singleton.sh` | CI logs |
