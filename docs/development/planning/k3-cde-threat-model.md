# K3 CDE Threat Model (Draft)

This is a living threat model for K3 as a cardholder data environment (CDE). It is an engineering artifact to guide
controls, tests, and evidence generation; it is not a formal assessment or certification.

## Scope

- **System:** K3 payment processing (Lambda + multi-region data replication + processor integrations)
- **In-scope data:** PAN (CHD), SAD (CVV/CVC/CID), tokens, keys, authentication material, logs/telemetry that could expose CHD/SAD
- **Primary frameworks:** PCI DSS v4.0.1 (see `docs/planning/pci-dss-v4.0.1-controls-matrix.md`)

## Assets (what we protect)

- CHD/SAD in transit and in memory during authorization flows
- Stored tokens and encrypted payment data
- Cryptographic keys (AWS Payment Cryptography, KMS)
- Processor credentials and webhook secrets
- Audit logs and operational telemetry
- CI/CD supply chain (code, dependencies, deployments)

## Trust boundaries (high level)

- Public ingress (API Gateway / public endpoints)
- Internal AWS components (Lambda, DynamoDB, KMS, Payment Cryptography)
- External processors (Tesouro, Finix, etc.)
- CI/CD systems and build agents

## Top threats (initial list)

- **Data leakage to logs/telemetry**: PAN/SAD accidentally logged, persisted in traces, or included in error payloads.
- **SAD persistence**: CVV stored in DB, encrypted blobs, caches, or crash dumps after authorization.
- **Unauthorized access**: weak auth, missing MFA, overly broad IAM permissions, missing segmentation.
- **Key compromise / misuse**: KMS or Payment Cryptography key permissions too broad; rotation mishandled.
- **Webhook spoofing**: processors’ webhooks not authenticated; replay attacks.
- **Supply chain compromise**: dependency vulnerabilities, poisoned builds, leaked secrets in CI.
- **Multi-region drift**: cross-region encryption population bugs leading to plaintext exposure or inconsistent controls.

## Mitigations (where we have controls today)

- PCI P0 non-storage guardrails for CVV (tests + docs): `docs/PCI_COMPLIANCE_IMPLEMENTATION.md`
- Multi-region encryption design and key strategy: `docs/Technical References/MULTI_REGION_ENCRYPTION_REQUIREMENTS.md`
- Authentication + rate limiting middleware: `internal/middleware/` and `SECURITY_MIDDLEWARE_IMPLEMENTATION.md`

## Gaps / open questions

- What is the authoritative CDE scope boundary per partner/stage, including CI/CD systems that can impact the CDE?
- What automated guardrails prevent PAN/SAD from being emitted in structured logs (beyond “don’t log it”)?
- What evidence bundle do we retain for key management (rotation, access reviews, HSM/PAC operations)?
- What is the TPSP responsibility matrix for processors and cloud services (PCI DSS Req 12.8 / 12.9)?

