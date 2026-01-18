# theorydb: 10/10 Rubric (Quality, Consistency, Completeness, Security, Compliance Readiness, Maintainability, Docs)

This rubric defines what “10/10” means and how category grades are computed. It is designed to prevent goalpost drift and
“green by dilution” by making scoring **versioned, measurable, and repeatable**.

## Versioning (no moving goalposts)
- **Rubric version:** `v0.2` (2026-01-18)
- **Comparability rule:** grades are comparable only within the same version.
- **Change rule:** bump the version + changelog entry for any rubric change (what changed + why).

### Changelog
- `v0.2`: Bring the HGM verifier to parity with the legacy `make rubric` surface (no lost checks) while keeping deterministic evidence under `hgm-infra/evidence/`.
- `v0.1`: Initial Hypergenium governance scaffold under `hgm-infra/`.

## Scoring (deterministic)
- Each category is scored **0–10**.
- Point weights sum to **10** per category.
- Requirements are **pass/fail** (either earn full points or 0).
- A category is **10/10 only if all requirements in that category pass**.

## Verification (commands + deterministic artifacts are the source of truth)
Every rubric item has exactly one verification mechanism:
- a command (`make ...`, `go test ...`, `bash scripts/...`), or
- a deterministic artifact check (required doc exists and matches an agreed format).

Enforcement rule (anti-drift):
- If an item’s verifier is a command/script, it only counts as passing once it runs and produces evidence under `hgm-infra/evidence/`.

---

## Quality (QUA) — reliable, testable, change-friendly
| ID | Points | Requirement | How to verify |
| --- | ---: | --- | --- |
| QUA-1 | 3 | Unit tests stay green (Go + TypeScript + Python, if present) | `bash scripts/verify-unit-tests.sh` |
| QUA-2 | 2 | Integration/contract tests stay green (Go + TypeScript + Python, if present) | `bash scripts/verify-integration-tests.sh` |
| QUA-3 | 2 | Coverage ≥ 90% (no denominator games; multi-language where applicable) | `bash scripts/verify-coverage.sh` |
| QUA-4 | 2 | Validator ↔ converter parity (no “validated but crashes” inputs) | `bash scripts/verify-validation-parity.sh` |
| QUA-5 | 1 | Bounded fuzz smoke pass for crashers | `bash scripts/fuzz-smoke.sh` |

**10/10 definition:** QUA-1 through QUA-5 pass.

## Consistency (CON) — one way to do the important things
| ID | Points | Requirement | How to verify |
| --- | ---: | --- | --- |
| CON-1 | 3 | gofmt/formatter clean (no diffs) | `bash scripts/verify-formatting.sh` |
| CON-2 | 5 | Lint/static analysis green (pinned version) | `bash scripts/verify-lint.sh` |
| CON-3 | 2 | Public boundary contract parity + DMS-first workflow | `bash scripts/verify-public-api-contracts.sh`, `bash scripts/verify-dms-first-workflow.sh` |

**10/10 definition:** CON-1 through CON-3 pass.

## Completeness (COM) — verify the verifiers (anti-drift)
| ID | Points | Requirement | How to verify |
| --- | ---: | --- | --- |
| COM-1 | 1 | Builds compile and versions align (Go + TypeScript + Python; includes lockfile installs) | `bash scripts/verify-builds.sh` |
| COM-2 | 1 | Toolchain pins align to repo (Go/Node/Python + pinned tool versions) | `bash scripts/verify-ci-toolchain.sh` |
| COM-3 | 1 | Planning docs exist and are versioned | `bash scripts/verify-planning-docs.sh` |
| COM-4 | 1 | Lint config schema-valid (no silent skip) | `golangci-lint config verify -c .golangci-v2.yml` |
| COM-5 | 1 | Coverage threshold not diluted (≥ 90%) | `bash scripts/verify-coverage-threshold.sh` |
| COM-6 | 2 | CI enforces rubric surface (runs `make rubric`, pinned tools, uploads artifacts) | `bash scripts/verify-ci-rubric-enforced.sh` |
| COM-7 | 1 | DynamoDB Local pinned (no `:latest`) | `bash scripts/verify-dynamodb-local-pin.sh` |
| COM-8 | 2 | Branch + release supply-chain enforced (premain prerelease, main release; version sync) | `bash scripts/verify-branch-release-supply-chain.sh`, `bash scripts/verify-branch-version-sync.sh` |

**10/10 definition:** COM-1 through COM-8 pass.

## Security (SEC) — abuse-resilient and reviewable
| ID | Points | Requirement | How to verify |
| --- | ---: | --- | --- |
| SEC-1 | 1 | Static security scan stays green (first-party only; config not diluted) | `bash scripts/sec-gosec.sh` |
| SEC-2 | 1 | Dependency vulnerability scan stays green (Go + TypeScript + Python) | `bash scripts/sec-dependency-scans.sh` |
| SEC-3 | 1 | Supply-chain verification stays green | `go mod verify` |
| SEC-4 | 2 | No `panic(...)` in production paths | `bash scripts/verify-no-panics.sh` |
| SEC-5 | 1 | Safe-by-default marshaling (unsafe only via explicit opt-in) | `bash scripts/verify-safe-defaults.sh` |
| SEC-6 | 1 | Expression boundary hardening (no injection-by-construction; list index update paths validated) | `bash scripts/verify-expression-hardening.sh` |
| SEC-7 | 1 | Network hygiene defaults (timeouts + retry posture) | `bash scripts/verify-network-hygiene.sh` |
| SEC-8 | 1 | `theorydb:"encrypted"` has enforced semantics (fail closed) | `bash scripts/verify-encrypted-tag-implemented.sh` |
| SEC-9 | 1 | Logging/operational standards enforced (repo-scoped) | `check_logging_ops_standards` (implemented in `hgm-infra/verifiers/hgm-verify-rubric.sh`) |

**10/10 definition:** SEC-1 through SEC-9 pass.

## Compliance Readiness (CMP) — auditability and evidence
| ID | Points | Requirement | How to verify |
| --- | ---: | --- | --- |
| CMP-1 | 4 | Controls matrix exists and is current | File exists: `hgm-infra/planning/theorydb-controls-matrix.md` |
| CMP-2 | 3 | Evidence plan exists and is reproducible | File exists: `hgm-infra/planning/theorydb-evidence-plan.md` |
| CMP-3 | 3 | Threat model exists and is current | File exists: `hgm-infra/planning/theorydb-threat-model.md` |

**10/10 definition:** CMP-1 through CMP-3 pass.

## Maintainability (MAI) — convergent codebase (recommended for AI-heavy repos)
| ID | Points | Requirement | How to verify |
| --- | ---: | --- | --- |
| MAI-1 | 4 | File-size/complexity budgets enforced | `bash scripts/verify-file-size.sh` |
| MAI-2 | 3 | Maintainability roadmap exists and is current | `bash scripts/verify-maintainability-roadmap.sh` |
| MAI-3 | 3 | Canonical implementations (no duplicate semantics) | `bash scripts/verify-query-singleton.sh` |

**10/10 definition:** MAI-1 through MAI-3 pass.

## Docs (DOC) — integrity and parity
| ID | Points | Requirement | How to verify |
| --- | ---: | --- | --- |
| DOC-1 | 2 | Threat model present | File exists: `hgm-infra/planning/theorydb-threat-model.md` |
| DOC-2 | 2 | Evidence plan present | File exists: `hgm-infra/planning/theorydb-evidence-plan.md` |
| DOC-3 | 2 | Rubric + roadmap present | File exists: `hgm-infra/planning/theorydb-10of10-rubric.md` |
| DOC-4 | 2 | Doc integrity (repo docs + HGM docs) | `check_doc_integrity` (implemented in `hgm-infra/verifiers/hgm-verify-rubric.sh`) |
| DOC-5 | 2 | Threat ↔ controls parity (repo docs + HGM docs) | `check_threat_controls_parity_full` (implemented in `hgm-infra/verifiers/hgm-verify-rubric.sh`) |

**10/10 definition:** DOC-1 through DOC-5 pass.

## Maintaining 10/10 (recommended CI surface)
```bash
bash scripts/verify-planning-docs.sh
bash scripts/verify-threat-controls-parity.sh
bash scripts/verify-doc-integrity.sh

bash scripts/verify-typescript-deps.sh
bash scripts/verify-python-deps.sh
bash scripts/verify-dms-first-workflow.sh

bash scripts/verify-unit-tests.sh
bash scripts/verify-integration-tests.sh
bash scripts/verify-validation-parity.sh
bash scripts/fuzz-smoke.sh
bash scripts/verify-coverage-threshold.sh
bash scripts/verify-coverage.sh

bash scripts/verify-formatting.sh
bash scripts/verify-lint.sh
bash scripts/verify-public-api-contracts.sh

bash scripts/verify-ci-toolchain.sh
bash scripts/verify-builds.sh
bash scripts/verify-ci-rubric-enforced.sh
bash scripts/verify-branch-release-supply-chain.sh
bash scripts/verify-branch-version-sync.sh
bash scripts/verify-dynamodb-local-pin.sh
golangci-lint config verify -c .golangci-v2.yml

bash scripts/verify-no-panics.sh
bash scripts/verify-safe-defaults.sh
bash scripts/verify-network-hygiene.sh
bash scripts/verify-expression-hardening.sh
bash scripts/verify-encrypted-tag-implemented.sh

bash scripts/sec-gosec.sh
bash scripts/sec-dependency-scans.sh
go mod verify

bash scripts/verify-file-size.sh
bash scripts/verify-maintainability-roadmap.sh
bash scripts/verify-query-singleton.sh

bash hgm-infra/verifiers/hgm-verify-rubric.sh
```
