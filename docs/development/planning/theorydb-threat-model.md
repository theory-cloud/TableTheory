# TableTheory Threat Model (Draft)

This is a living threat model for TableTheory. It is an engineering artifact to guide controls, tests, and evidence
generation; it is not a formal assessment or certification.

## Scope

- **System:** TableTheory (Go library for DynamoDB access patterns + example apps)
- **In-scope data:** whatever the consuming application persists in DynamoDB (often includes PII, tokens, secrets, or customer metadata)
- **Out of scope:** application authentication/authorization, IAM policy design, and environment hardening (owned by consuming services)
- **Important note:** `theorydb:"encrypted"` has enforced field-level encryption semantics and fails closed unless `session.Config.KMSKeyARN` is configured (see `docs/development/planning/theorydb-encryption-tag-roadmap.md`).

## Assets (what we protect)

- Integrity of persisted data (no accidental overwrites, especially on partial updates)
- Correctness of DynamoDB expressions (no silent query broadening or unexpected filter behavior)
- AWS credentials and permissions (avoid patterns that encourage overbroad IAM usage)
- Reliability and resource safety (avoid unbounded reads/writes and retry storms)
- Supply chain integrity (dependencies and build toolchain)

## Trust boundaries (high level)

- **Calling application code** (trusted to provide correct models and validated inputs)
- **TableTheory library** (responsible for safe defaults, deterministic behavior, and clear docs)
- **AWS SDK / DynamoDB API** (remote dependency and policy enforcement point)
- **CI environment** (build/test/security toolchain)

## Top threats (initial list)

- **THR-1 — Data clobber via surprising update semantics:** empty-but-non-nil values overwriting stored attributes; mismatch between update APIs.
- **THR-2 — Expression misuse / injection-by-construction:** unvalidated attribute names or raw expression strings (including list index update paths) leading to broken queries or unintended access patterns.
- **THR-3 — Unsafe reflection hazards:** unsafe pointer math or reflect edge cases leading to panics, data corruption, or non-deterministic behavior.
- **THR-4 — DoS / cost blowups:** unbounded scans/queries, large batch operations, or aggressive retries causing throttling storms.
- **THR-5 — Sensitive data leakage:** user-provided values accidentally logged in examples/tests or surfaced in error strings.
- **THR-6 — Supply-chain compromise:** vulnerable dependencies or drift in security tooling causing missed findings.
- **THR-7 — Public API contract drift:** exported helpers that silently diverge from canonical TableTheory tag/metadata semantics (e.g., unmarshalling helpers ignoring `pk`/`sk`/`attr:`/`encrypted`), causing incorrect behavior and unsafe assumptions in consuming services.

## Mitigations (where we have controls today)

- Versioned planning rubric + CI gates: `docs/development/planning/theorydb-10of10-rubric.md`
- Integration tests against DynamoDB Local: `make integration`
- Static analysis: `make lint`, `bash scripts/sec-gosec.sh`, `bash scripts/sec-govulncheck.sh`
- Resource limiting primitives: `pkg/protection/` (for consumers that choose to use them)

## Gaps / open questions

- Add an opt-in integration test suite that validates real KMS round-trips behind an env gate (so default CI can stay credential-free).
- Should we add fuzzing for expression building and marshaling edge cases?
- Where should “safe defaults” live for limits/retries (library vs application)?
