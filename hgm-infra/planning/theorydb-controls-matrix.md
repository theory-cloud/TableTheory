# theorydb Controls Matrix (custom — v0.2)

This matrix is the “requirements → controls → verifiers → evidence” backbone for theorydb. It is intentionally
engineering-focused: it does not claim compliance, but it makes security/quality assertions traceable and repeatable.

## Scope
- **System:** Theorydb (multi-language DynamoDB library + tooling) used in security-critical production applications.
- **In-scope data:** PII, authentication/session tokens, secrets, and cardholder data (CHD) *by transitive usage* (data stored by consuming services).
- **Environments:** local dev, CI, staging, production; “prod-like” means CI running the full rubric surface with pinned tooling and DynamoDB Local where required.
- **Third parties:** AWS (DynamoDB, STS, KMS, Lambda), GitHub Actions, npm registry, PyPI.
- **Out of scope:** consuming service IAM/policy design, app-layer authn/authz, environment hardening (owned by consuming services).
- **Assurance target:** audit-ready engineering evidence for a high-risk shared library.

## Threats (reference IDs)
- Enumerate threats as stable IDs (`THR-*`) in `hgm-infra/planning/theorydb-threat-model.md`.
- Each `THR-*` must map to ≥1 row in the controls table below.

## Status (evidence-driven)
If you track implementation status, treat it as evidence-driven:
- `unknown`: no verifier/evidence yet
- `partial`: some controls exist but coverage/evidence is incomplete
- `implemented`: verifier exists and evidence path is repeatable

## Engineering Controls (Threat → Control → Verifier → Evidence)
This table is the canonical mapping used by the rubric/roadmap/evidence plan.

| Area | Threat IDs | Control ID | Requirement | Control (what we implement) | Verification (command/gate) | Evidence (artifact/location) |
| --- | --- | --- | --- | --- | --- | --- |
| Quality | THR-1, THR-2, THR-3 | QUA-1 | Unit tests stay green | Unit tests cover core update/query/marshal semantics across supported packages/languages | `bash scripts/verify-unit-tests.sh` | `hgm-infra/evidence/QUA-1-output.log` |
| Quality | THR-1, THR-2, THR-4 | QUA-2 | Integration/contract tests stay green | DynamoDB Local integration tests + contract tests (where applicable) | `bash scripts/verify-integration-tests.sh` | `hgm-infra/evidence/QUA-2-output.log` |
| Quality | THR-1, THR-2, THR-3 | QUA-3 | Coverage ≥ 90% (no denominator games) | Coverage gates are raise-only and default to ≥90% | `bash scripts/verify-coverage.sh` | `hgm-infra/evidence/QUA-3-output.log` |
| Quality | THR-1, THR-2, THR-3 | QUA-4 | Validator ↔ converter parity | Validator and converter behavior stay aligned (no “validated but crashes” inputs) | `bash scripts/verify-validation-parity.sh` | `hgm-infra/evidence/QUA-4-output.log` |
| Quality | THR-2, THR-3 | QUA-5 | Bounded fuzz smoke pass | Bounded fuzz smoke runs to catch crashers early | `bash scripts/fuzz-smoke.sh` | `hgm-infra/evidence/QUA-5-output.log` |
| Consistency | THR-6 | CON-1 | Formatting is clean (no diffs) | gofmt + language formatters enforced | `bash scripts/verify-formatting.sh` | `hgm-infra/evidence/CON-1-output.log` |
| Consistency | THR-2, THR-3, THR-6 | CON-2 | Lint/static analysis is enforced (pinned toolchain) | golangci-lint + ruff/eslint (when applicable) stay green under pinned versions | `bash scripts/verify-lint.sh` | `hgm-infra/evidence/CON-2-output.log` |
| Consistency | THR-7 | CON-3 | Public boundary contract parity + DMS-first workflow | Exported helpers stay consistent with canonical tags/metadata and follow DMS-first rules | `bash scripts/verify-public-api-contracts.sh`, `bash scripts/verify-dms-first-workflow.sh` | `hgm-infra/evidence/CON-3-output.log` |
| Completeness | THR-6 | COM-1 | Builds compile and versions align | All in-repo Go/TS/Py builds compile; lockfile-based dependency installs are required | `bash scripts/verify-typescript-deps.sh`, `bash scripts/verify-python-deps.sh`, `bash scripts/verify-builds.sh` | `hgm-infra/evidence/COM-1-output.log` |
| Completeness | THR-6 | COM-2 | CI/toolchain pins align to repo expectations | CI uses `go-version-file: go.mod`, Node/Python versions pinned, and tooling is pinned (no `latest`) | `bash scripts/verify-ci-toolchain.sh` | `hgm-infra/evidence/COM-2-output.log` |
| Completeness | THR-6 | COM-3 | Planning docs exist and are versioned | Rubric/roadmap/evidence plan/threat model/controls matrix are present and versioned | `bash scripts/verify-planning-docs.sh` | `hgm-infra/evidence/COM-3-output.log` |
| Completeness | THR-6 | COM-4 | Lint config schema-valid (no silent skip) | golangci-lint config is schema-valid under v2 | `golangci-lint config verify -c .golangci-v2.yml` | `hgm-infra/evidence/COM-4-output.log` |
| Completeness | THR-6 | COM-5 | Coverage threshold not diluted | Default threshold across languages remains ≥90% | `bash scripts/verify-coverage-threshold.sh` | `hgm-infra/evidence/COM-5-output.log` |
| Completeness | THR-6 | COM-6 | CI enforces rubric surface | CI runs `make rubric`, uses pinned tools, and archives key artifacts | `bash scripts/verify-ci-rubric-enforced.sh` | `hgm-infra/evidence/COM-6-output.log` |
| Completeness | THR-6 | COM-7 | DynamoDB Local pinned (no `:latest`) | DynamoDB Local image is pinned to a deterministic version | `bash scripts/verify-dynamodb-local-pin.sh` | `hgm-infra/evidence/COM-7-output.log` |
| Completeness | THR-6 | COM-8 | Branch + release supply-chain enforced | premain prerelease, main release; branch/version sync enforced | `bash scripts/verify-branch-release-supply-chain.sh`, `bash scripts/verify-branch-version-sync.sh` | `hgm-infra/evidence/COM-8-output.log` |
| Security | THR-2, THR-3, THR-6 | SEC-1 | Baseline SAST stays green (config not diluted) | gosec stays green on first-party code; security config cannot be silently weakened | `bash hgm-infra/verifiers/hgm-verify-rubric.sh` | `hgm-infra/evidence/SEC-1-output.log` |
| Security | THR-6 | SEC-2 | Dependency vulnerability scan stays green | govulncheck + npm audit + pip-audit (when present) | `bash scripts/sec-dependency-scans.sh` | `hgm-infra/evidence/SEC-2-output.log` |
| Security | THR-6 | SEC-3 | Module integrity and checksum verification | Go module checksums verified | `go mod verify` | `hgm-infra/evidence/SEC-3-output.log` |
| Security | THR-3 | SEC-4 | No `panic(...)` in production paths | Codebase avoids panic-driven control flow in production paths | `bash scripts/verify-no-panics.sh` | `hgm-infra/evidence/SEC-4-output.log` |
| Security | THR-1, THR-5 | SEC-5 | Safe-by-default marshaling | Unsafe behavior is only available via explicit opt-in | `bash scripts/verify-safe-defaults.sh` | `hgm-infra/evidence/SEC-5-output.log` |
| Security | THR-2 | SEC-6 | Expression boundary hardening | Expression construction validates risky update/query paths to prevent injection-by-construction | `bash scripts/verify-expression-hardening.sh` | `hgm-infra/evidence/SEC-6-output.log` |
| Security | THR-4 | SEC-7 | Network hygiene defaults | Default timeouts/retry posture avoid accidental DoS/cost blowups | `bash scripts/verify-network-hygiene.sh` | `hgm-infra/evidence/SEC-7-output.log` |
| Security | THR-5 | SEC-8 | `theorydb:"encrypted"` has enforced semantics (fail closed) | Encrypted tag is enforced by library behavior; missing key material fails closed | `bash scripts/verify-encrypted-tag-implemented.sh` | `hgm-infra/evidence/SEC-8-output.log` |
| Security | THR-5 | SEC-9 | Logging/operational standards enforced (repo-scoped) | Library code avoids stdout printing and process termination; standards doc is present and checked | `bash hgm-infra/verifiers/hgm-verify-rubric.sh` | `hgm-infra/evidence/SEC-9-output.log` |
| Compliance | THR-6 | CMP-1 | Controls matrix exists and is current | Controls matrix is versioned and maps threats → controls → evidence | File existence check | `hgm-infra/planning/theorydb-controls-matrix.md` |
| Compliance | THR-6 | CMP-2 | Evidence plan exists and is reproducible | Evidence plan is versioned and refresh commands are deterministic | File existence check | `hgm-infra/planning/theorydb-evidence-plan.md` |
| Compliance | THR-6 | CMP-3 | Threat model exists and is current | Threat model is versioned and maps threats → primary controls | File existence check | `hgm-infra/planning/theorydb-threat-model.md` |
| Maintainability | THR-6 | MAI-1 | File-size/complexity budgets enforced | File-size budgets prevent unreviewable “god files” | `bash scripts/verify-file-size.sh` | `hgm-infra/evidence/MAI-1-output.log` |
| Maintainability | THR-6 | MAI-2 | Maintainability roadmap current | Maintainability convergence plan is present and required sections stay current | `bash scripts/verify-maintainability-roadmap.sh` | `hgm-infra/evidence/MAI-2-output.log` |
| Maintainability | THR-1, THR-6 | MAI-3 | Canonical implementations (no duplicate semantics) | One canonical Query implementation to prevent semantic drift | `bash scripts/verify-query-singleton.sh` | `hgm-infra/evidence/MAI-3-output.log` |
| Docs | THR-6 | DOC-4 | Doc integrity (repo docs + HGM docs) | Markdown links resolve and no template tokens leak into committed docs | `bash hgm-infra/verifiers/hgm-verify-rubric.sh` | `hgm-infra/evidence/DOC-4-output.log` |
| Docs | THR-6, THR-7 | DOC-5 | Threat model ↔ controls parity (no unmapped threats) | Threat IDs are stable and mapped to ≥1 control row | `bash hgm-infra/verifiers/hgm-verify-rubric.sh` | `hgm-infra/evidence/DOC-5-output.log` + `hgm-infra/evidence/DOC-5-parity.log` |

> Add rows as needed for repo-specific anti-drift controls or newly discovered threats.

## Framework Mapping (Optional; for PCI/HIPAA/SOC2)
If a compliance framework applies, keep standards text out of the repo; store only IDs + short titles and reference a KB
path/env var.

| Framework | Requirement ID | Requirement (short) | Status | Related Control IDs | Verification (command/gate) | Evidence (artifact/location) | Owner |
| --- | --- | --- | --- | --- | --- | --- | --- |
| (optional) | (id) | (title) | (status) | (control IDs) | (command) | (path) | (owner) |

## Notes
- Prefer deterministic verifiers (tests, static analysis, pinned build checks) over manual checklists.
- Treat this matrix as “source material”: the rubric/roadmap/evidence plan must stay consistent with Control IDs here.
