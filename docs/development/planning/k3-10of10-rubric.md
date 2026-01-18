# K3: 10/10 Rubric (Quality, Consistency, Completeness, Security, Compliance Readiness)

This rubric defines what “10/10” means for K3’s highest-risk surface area and how category grades are computed.
The goal is to prevent goalpost drift between audit passes by making scoring **versioned, measurable, and repeatable**.

This rubric also defends against **generative AI drift** (and adjacent “green by exclusion” failure modes) by including
meta-gates under **Completeness** that verify the integrity of the verifiers (toolchain alignment, config validity, and
anti-dilution checks).

## Versioning (no moving goalposts)

- **Rubric version:** `v0.3` (2026-01-08)
- **Comparability rule:** grades are only comparable within the same rubric version.
- **Change rule:** any rubric change must bump the version and include a brief changelog entry (what changed + why).

### Changelog

- `v0.3` (2026-01-08): Strengthen **SEC-4** to run the full non-integration security suite and add deterministic checks for CVV persistence + unsanitized Tesouro payload logging.
- `v0.2` (2026-01-06): Add **Completeness (COM)** to prevent drift and “mystery meat” (toolchain, multi-module health, and gate-config integrity).
- `v0.1` (2026-01-06): Initial rubric (K3 CDE baseline).

## Scoring (deterministic)

- Each category is scored **0–10**.
- Each category has requirements with fixed point weights that sum to **10**.
- Requirements are **pass/fail** (either earn the full points or earn 0).
- A category is **10/10 only if all requirements in that category pass**.

## Verification (commands + deterministic artifacts are the source of truth)

Every rubric item has exactly one verification mechanism:

- a command (`make ...`, `go test ...`, `bash scripts/...`), or
- a deterministic artifact check (required doc exists and matches an agreed format).

---

## Quality (QUA) — reliable, testable, change-friendly

| ID | Points | Requirement | How to verify |
| --- | ---: | --- | --- |
| QUA-1 | 4 | Unit tests stay green | `make test-unit` |
| QUA-2 | 3 | Integration tests stay green (DynamoDB Local required) | `make test-integration` |
| QUA-3 | 3 | Coverage stays at or above the CI threshold | `bash scripts/verify-coverage.sh` |

**10/10 definition:** QUA-1 through QUA-3 pass.

---

## Consistency (CON) — one way to do the important things

| ID | Points | Requirement | How to verify |
| --- | ---: | --- | --- |
| CON-1 | 4 | Go formatting is clean (no diffs) | `bash scripts/fmt-check.sh` |
| CON-2 | 6 | Lint stays green (static analysis + style budgets) | `golangci-lint run --timeout=5m --config .golangci-v2.yml` |

**10/10 definition:** CON-1 and CON-2 pass.

---

## Completeness (COM) — no drift, no mystery meat

| ID | Points | Requirement | How to verify |
| --- | ---: | --- | --- |
| COM-1 | 2 | All Go modules in the repo compile (no broken nested modules) | `bash scripts/verify-go-modules.sh` |
| COM-2 | 2 | CI toolchain aligns to repo expectations (no silent Go/lint drift) | `bash scripts/verify-ci-toolchain.sh` |
| COM-3 | 2 | Lint configuration is schema-valid for golangci-lint v2 | `golangci-lint config verify -c .golangci-v2.yml` |
| COM-4 | 2 | Coverage gate configuration is not diluted (default threshold ≥ 90%) | `bash scripts/verify-coverage-threshold.sh` |
| COM-5 | 1 | Security scan configuration is not diluted (no excluded high-signal gosec rules) | `bash scripts/verify-sec-gosec-config.sh` |
| COM-6 | 1 | Logging standard enforcement stays green (Lift structured logging for app/runtime code) | `bash scripts/verify-logging-standards.sh` |

**10/10 definition:** COM-1 through COM-6 pass.

---

## Security (SEC) — abuse-resilient and reviewable by default

| ID | Points | Requirement | How to verify |
| --- | ---: | --- | --- |
| SEC-1 | 3 | Static security scan stays green (gosec) | `bash scripts/sec-gosec.sh` |
| SEC-2 | 3 | Dependency vulnerability scan stays green (govulncheck) | `bash scripts/sec-govulncheck.sh` |
| SEC-3 | 2 | Supply-chain verification stays green | `go mod verify` |
| SEC-4 | 2 | PCI P0 regression gate stays green (no SAD storage) | `go test ./test/security` |

**10/10 definition:** SEC-1 through SEC-4 pass.

---

## Compliance Readiness (CMP) — auditability and evidence

| ID | Points | Requirement | How to verify |
| --- | ---: | --- | --- |
| CMP-1 | 4 | Controls matrix exists and includes required PCI anchors | `bash scripts/verify-planning-docs.sh` |
| CMP-2 | 3 | Evidence plan exists and is reproducible | `bash scripts/verify-planning-docs.sh` |
| CMP-3 | 3 | Threat model exists and is current | `bash scripts/verify-planning-docs.sh` |

**10/10 definition:** CMP-1 through CMP-3 pass.

---

## Maintaining 10/10 (recommended CI surface)

To keep grades stable over time, CI should run (at minimum):

```bash
bash scripts/verify-planning-docs.sh
bash scripts/fmt-check.sh
golangci-lint run --timeout=5m --config .golangci-v2.yml
golangci-lint config verify -c .golangci-v2.yml

make test-unit
make test-integration
bash scripts/verify-coverage.sh

bash scripts/verify-go-modules.sh
bash scripts/verify-ci-toolchain.sh
bash scripts/verify-coverage-threshold.sh
bash scripts/verify-sec-gosec-config.sh
bash scripts/verify-logging-standards.sh

	bash scripts/sec-gosec.sh
	bash scripts/sec-govulncheck.sh
	go mod verify
	go test ./test/security
	```

If any of the above fail, at least one category cannot be 10/10 under this rubric.
