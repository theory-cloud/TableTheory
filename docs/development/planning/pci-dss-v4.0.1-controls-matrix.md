# K3 PCI DSS v4.0.1 Controls Matrix (Draft)

This is a working controls matrix for K3 as a cardholder data environment (CDE).

- This is not legal advice.
- This does not claim PCI compliance.
- Status should be treated as **evidence-driven** (implemented requires a verifier + evidence pointer).

## Source of truth for PCI DSS text

To avoid licensing/distribution issues, the PCI DSS v4.0.1 standard text should live out-of-repo in a local/internal KB.

- Suggested env var: `PCI_KB_PATH`
- Example KB path on one machine: `/home/aron/Downloads/pci/knowledge-base/pci-dss-v4.0.1`
- Index file: `${PCI_KB_PATH}/index.md`

## Scope summary

- **Domain(s):** payments (CDE)
- **Framework(s):** PCI DSS v4.0.1 (+ evaluate Appendix A1 for service provider applicability)
- **In-scope environments:** local dev (SDLC controls), CI, lab/study/prod stages (`paytheorylab`, `paytheorystudy`, `paytheory`)
- **In-scope systems:** API ingress, K3 Lambda handlers, stream processors, DynamoDB (Global Tables), KMS, AWS Payment Cryptography,
  Secrets Manager, CI/CD, operational logging/monitoring, processor integrations (Tesouro/Finix/...)
- **Out of scope:** explicitly define any systems that cannot impact the CDE (to be refined)
- **Data types:** CHD (PAN), SAD (CVV/CVC/CID), tokens, cryptographic keys, secrets, operational telemetry

## Controls matrix

| Framework | Requirement ID | Requirement (short) | Status | Control (what we implement) | Verification (tests/gates) | Evidence (where) | Owner |
| --- | --- | --- | --- | --- | --- | --- | --- |
| PCI DSS v4.0.1 | 3.3.1 | SAD is not stored after authorization | implemented | CVV never stored in models/encrypted blobs; transient use only | `go test ./test/security -run TestPCIDSSRequirement3_2_2_Compliance` | `docs/PCI_COMPLIANCE_IMPLEMENTATION.md` | K3 |
| PCI DSS v4.0.1 | 3.2.1 | Retention/disposal minimizes stored account data | unknown | Define retention/TTL strategy for any stored account data + logs | (define) | (define) | K3/Security |
| PCI DSS v4.0.1 | 3.5 | PAN secured wherever stored | partial | Encrypt stored PAN/payment data; prefer tokens; multi-region encryption strategy | (define) | `docs/Technical References/MULTI_REGION_ENCRYPTION_REQUIREMENTS.md` | K3 |
| PCI DSS v4.0.1 | 3.6 | Cryptographic keys are secured | partial | Least-privilege key access; rotation; separation of duties; per-partner isolation | (define) | `docs/Technical References/MULTI_REGION_ENCRYPTION_REQUIREMENTS.md` | Platform/Security |
| PCI DSS v4.0.1 | 4.2.1 | Strong crypto/protocols protect PAN in transit | unknown | Enforce TLS for public ingress and processor calls; disable weak protocol fallback | (define) | (define) | Platform |
| PCI DSS v4.0.1 | 6.2.1 | Bespoke/custom software developed securely | partial | CI tests + lint + code review; secure coding guidance for CDE | `make test-unit` | `.github/workflows/test.yml` | K3 |
| PCI DSS v4.0.1 | 6.3 | Vulnerabilities identified and addressed | partial | SAST + vuln scanning in CI; remediation workflow | `bash scripts/sec-gosec.sh` | `.github/workflows/test.yml` | K3/Security |
| PCI DSS v4.0.1 | 8 | Users identified/authenticated to system components | partial | JWT/API key/webhook auth middleware; least-privilege IAM | `go test ./...` | `SECURITY_MIDDLEWARE_IMPLEMENTATION.md` | K3 |
| PCI DSS v4.0.1 | 10 | Log/monitor access to system components and CHD | unknown | Audit logging strategy, retention, monitoring alerts, time sync | (define) | `docs/operations/incident-response-runbook.md` | Platform/Security |
| PCI DSS v4.0.1 | 11 | Test security of systems regularly | partial | CI security scans + targeted security tests | `bash scripts/sec-govulncheck.sh` | `.github/workflows/test.yml` | K3/Security |
| PCI DSS v4.0.1 | 12 | Security policies/programs support information security | partial | Runbooks, incident response process, TPSP management approach | (define) | `docs/operations/incident-response-runbook.md` | Platform/Security |

## Notes / next steps

- Expand this matrix from “anchor controls” to complete PCI DSS coverage as scope is finalized.
- For each row, prefer a verifier that can be automated (tests/CI) and an evidence pointer that is reproducible.

