# Controls Matrix Template (High-Risk Domains)

This template is intended to be copied and filled per project.

## Scope summary

- **Domain(s):** [finance|healthcare|other]
- **Framework(s):** [PCI DSS v4.x / HIPAA / SOC2 / ...]
- **In-scope environments:** [dev|staging|prod]
- **In-scope systems:** [list services/accounts/vendors]
- **Out of scope:** [explicit exclusions]
- **Data types:** [CHD/SAD/PHI/PII/secrets/telemetry]

## Controls matrix

| Framework | Requirement ID | Requirement | Control (what we implement) | Verification (tests/gates) | Evidence (where) | Owner |
| --- | --- | --- | --- | --- | --- | --- |
| [PCI DSS] | [e.g. 3.3.1] | [short name] | [design + code/infra/process] | [command / CI gate / monitor] | [path/link/runbook] | [team] |

## Notes

- Keep requirements granular (don’t merge unrelated controls).
- Prefer verification that can be automated; use deterministic artifacts for the rest (docs, diagrams, runbooks).
- “Evidence” should be reproducible: a command, a config export, or a stable document location.
- Include CI/CD and supply-chain controls explicitly (protected branches, pinned tooling, automated releases, dependency update policy).
