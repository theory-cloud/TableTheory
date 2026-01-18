# TableTheory: Multi-language Verification Parity Roadmap

Goal: make “quality, consistency, completeness, security, and maintainability” **equally enforceable** across Go,
TypeScript, and Python — with **no green-by-exclusion**.

This roadmap is about verification parity (rubric + CI gates). Feature parity is tracked separately:
`docs/development/planning/theorydb-multilang-feature-parity-matrix.md`.

## Problem statement

Multi-language support fails in practice when one language becomes “best effort”:

- weaker lint/typecheck
- skipped integration tests
- no coverage targets
- no dependency scanning
- missing mocks (testing becomes expensive → drift)

The rubric must treat every language as a first-class implementation.

## Definitions

Verification parity means each language has the same classes of checks:

- **Format** (autoformat + check)
- **Lint** (strict; zero warnings)
- **Typecheck / build** (strict; reproducible)
- **Unit tests** (fast, deterministic)
- **Integration tests** (DynamoDB Local; strict-by-default)
- **Coverage** (>= 90% for “library code”, not examples/tests)
- **Dependency scanning** (no known vulns; no bypass)
- **File-size / maintainability budgets**
- **Release/version alignment checks** (single repo version; no registry publishing)

See also:
- `docs/development/planning/theorydb-multilang-verification-parity-matrix.md` (definitions + rubric gate mapping)

## Milestones

### VP-0 — Codify what we measure (spec the gates)

**Goal:** define the measurable verification surface per language so “coverage” and “tests” are comparable.

**Acceptance criteria**
- A documented definition exists for each language:
  - what counts as “library code”
  - what counts as “unit” vs “integration”
  - what coverage metric(s) matter (line, branch, function) and the minimum thresholds
- A single “verification parity matrix” exists mapping each rubric gate to each language.

**Evidence**
- `docs/development/planning/theorydb-multilang-verification-parity-matrix.md`
- This roadmap + updated rubric references.

---

### VP-1 — Coverage measurement for TS + Python (no gating yet)

**Goal:** generate coverage artifacts in CI without changing pass/fail behavior yet.

**Acceptance criteria**
- TypeScript coverage can be generated for `ts/src/**` (example tool: `c8`) and produces a deterministic summary.
- Python coverage can be generated for `py/src/**` (tool: `pytest-cov`) and produces `coverage.xml`.
- CI uploads coverage artifacts for all three languages (Go already produces coverage profiles).

**Evidence**
- New scripts:
  - `bash scripts/coverage-ts.sh`
  - `bash scripts/coverage-py.sh`
- CI workflow logs show generated summaries and artifacts.

---

### VP-2 — Coverage gates (>= 90%) for TS + Python (rubric-enforced)

**Goal:** the rubric must fail if any language’s library coverage drops below 90%.

**Acceptance criteria**
- New verifiers exist and are included in `make rubric`:
  - `bash scripts/verify-typescript-coverage.sh` (threshold default 90%, raise-only override)
  - `bash scripts/verify-python-coverage.sh` (threshold default 90%, raise-only override)
- Coverage “denominator games” are prevented:
  - cannot exclude additional production directories to inflate %
  - cannot move logic into `examples/` or `tests/` to avoid measurement

**Notes**
- This may require a staged rollout: add the gate, then immediately raise test coverage until it passes.
- Do not weaken existing Go coverage gates.

---

### VP-3 — Shared contract tests become the parity arbiter

**Goal:** enforce behavior equivalence with one suite runnable against all languages.

**Acceptance criteria**
- Contract tests run against Go/TS/Py (at minimum for drift-prone features):
  - cursor encoding/decoding canonical format
  - encryption envelope shape + AAD binding rules
  - reserved word escaping, attribute naming conventions, empty-set handling
- A single pinned DynamoDB Local version is used.

**Evidence**
- `contract-tests/` has executable runners for each language and a minimal scenario set that runs in CI.

---

### VP-4 — Public mocks/testkit parity (make testing cheap)

**Goal:** consumers can test without reinventing mocks for AWS resources.

**Acceptance criteria**
- Each language exposes a **public**, supported mocking surface for:
  - DynamoDB client calls used by TableTheory
  - KMS (for encryption) or an official deterministic provider
  - time/clock source (for lifecycle timestamps)
  - randomness source (for encryption nonce generation) where applicable
- Mocks are versioned, tested, and documented as supported API (not “internal test helpers”).

**Implementation guidance**
- Go: keep expanding `pkg/mocks/` + interfaces in `pkg/core/` / `pkg/interfaces/` (mockgen).
- TypeScript: ship a `testkit` export (e.g., `@theory-cloud/tabletheory-ts/testkit`) with a strict `send()` mock for the AWS
  SDK v3 command surface TableTheory uses.
- Python: ship `theorydb_py.mocks` with stub clients for DynamoDB + KMS and/or documented `botocore.stub.Stubber`
  helpers.

---

### VP-5 — CDK cross-language demo stack (verification + proof)

**Goal:** demonstrate “one table, three runtimes” in a deployable example driven by CDK (this is how we validate language support).

**Acceptance criteria**
- A CDK app exists (TypeScript CDK recommended) that deploys:
  - one DynamoDB table with canonical attribute names (DMS-friendly)
  - Go Lambda + Node.js 24 Lambda + Python 3.14 Lambda
  - a small API surface (API Gateway or Function URLs) to exercise cross-language reads/writes
- CI can at least run `cdk synth` (no AWS credentials needed) to prevent bitrot.
- Optionally: a deployment smoke test exists behind an explicit opt-in gate (requires AWS credentials).

## Rollup: “verification parity done” definition

Verification parity is “done” when:

- The rubric treats all languages symmetrically for format/lint/typecheck/tests/integration/coverage/deps.
- Shared contract tests exist for drift-prone behavior and run in CI.
- Each language ships public mocks/testkit utilities so downstream services can test cheaply.
- CDK example proves real-world cross-language operation on a single table.
