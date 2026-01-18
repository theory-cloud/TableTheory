# Rubric Template (High-Risk Domains)

This template is intended to be copied and filled per project.

## Versioning (no moving goalposts)

- **Rubric version:** `v0.1` (YYYY-MM-DD)
- **Comparability rule:** grades compare only within the same rubric version.
- **Change rule:** any change bumps the version and adds a brief changelog note.

### Changelog

- `v0.1` (YYYY-MM-DD): initial rubric.

## Scoring (deterministic)

- Each category is 0–10.
- Point weights sum to 10 per category.
- Requirements are pass/fail and earn full points or zero.
- A category is 10/10 only if all its requirements pass.

## Verification (commands + artifacts are the source of truth)

Each rubric item must name exactly one verification mechanism:

- a command (`./tool verify ...`, `go test ...`, `terraform validate`, etc.), or
- a deterministic artifact check (required doc exists and matches an agreed format).

---

## Security (SEC) — abuse-resilient by default

| ID | Points | Requirement | How to verify |
| --- | ---: | --- | --- |
| SEC-1 | 2 | Static security scan stays green | [command] |
| SEC-2 | 2 | Dependency vulnerability scan stays green | [command] |
| SEC-3 | 2 | Supply-chain / provenance verification stays green | [command] |
| SEC-4 | 2 | Public boundary contract + regression gates stay green (no “green by omission”) | [command/test] |
| SEC-5 | 2 | Branch/release supply chain is enforced (protected branches + automated release/prerelease) | [artifact check/command] |

**10/10 definition:** SEC-1 through SEC-5 pass.

Notes:
- If the system exposes “security-affordance” tags/flags (e.g., `encrypted`, `redacted`, `masked`), add a scored item that ensures they have **enforced semantics** and fail closed when misconfigured.
- If the system exposes multiple entry points (e.g., SDK wrappers + internal packages), add contract tests/verifiers that ensure exported helpers have the same semantics (no silent tag/validation differences).

---

## Maintainability (MAI) — keep the system reviewable over time

This category is especially important for AI-assisted code generation, where “it works” can still accumulate
structural debt that increases security and reliability risk (duplicate implementations, god files, unclear canonical paths).

| ID | Points | Requirement | How to verify |
| --- | ---: | --- | --- |
| MAI-1 | 4 | Production code stays under a file-size budget (no “god files”) | [command] |
| MAI-2 | 3 | Maintainability/convergence roadmap exists and is current | [artifact check] |
| MAI-3 | 3 | One canonical implementation for each critical path | [command] |

**10/10 definition:** MAI-1 through MAI-3 pass.

---

## Privacy (PRV) — minimize and protect sensitive data

| ID | Points | Requirement | How to verify |
| --- | ---: | --- | --- |
| PRV-1 | 4 | Sensitive fields are scrubbed from logs/telemetry | [command/test] |
| PRV-2 | 3 | Access controls are least-privilege for sensitive data | [IaC assertions / policy checks] |
| PRV-3 | 3 | Encryption at rest/in transit is enforced for sensitive data flows | [config check / integration test] |

**10/10 definition:** PRV-1 through PRV-3 pass.

---

## Compliance Readiness (CMP) — auditability and evidence

| ID | Points | Requirement | How to verify |
| --- | ---: | --- | --- |
| CMP-1 | 4 | Controls matrix is complete for in-scope frameworks | [artifact check] |
| CMP-2 | 3 | Evidence plan exists and is reproducible | [artifact check] |
| CMP-3 | 3 | Risk assessment / threat model exists and is current | [artifact check] |

**10/10 definition:** CMP-1 through CMP-3 pass.
