# TableTheory: 10/10 Roadmap (Rubric v0.7)

This roadmap is the execution plan for achieving and maintaining **10/10** across **Quality**, **Consistency**,
**Completeness**, **Security**, **Maintainability**, and **Docs** as defined by:

- `docs/development/planning/theorydb-10of10-rubric.md` (source of truth; versioned)

## Current scorecard (Rubric v0.7)

Scoring note: a check is only treated as “passing” if it is both green **and** enforced by a trustworthy verifier
(pinned toolchain, stable commands, and no “green by exclusion” shortcuts).

| Category | Grade | Blocking rubric items |
| --- | ---: | --- |
| Quality | 10/10 | — |
| Consistency | 10/10 | — |
| Completeness | 10/10 | — |
| Security | 10/10 | — |
| Maintainability | 10/10 | — |
| Docs | 10/10 | — |

Evidence (refresh whenever behavior changes):

- `bash scripts/verify-unit-tests.sh`
- `bash scripts/verify-integration-tests.sh`
- `bash scripts/verify-coverage.sh` (current: **90.1%** vs threshold **90%**)
- `bash scripts/verify-coverage-threshold.sh` (default threshold **90%**)
- `bash scripts/verify-formatting.sh`
- `golangci-lint config verify -c .golangci-v2.yml`
- `bash scripts/verify-lint.sh`
- `bash scripts/verify-public-api-contracts.sh`
- `bash scripts/verify-builds.sh`
- `bash scripts/verify-ci-toolchain.sh`
- `bash scripts/verify-ci-rubric-enforced.sh`
- `bash scripts/verify-dynamodb-local-pin.sh`
- `bash scripts/verify-threat-controls-parity.sh`
- `bash scripts/verify-doc-integrity.sh`
- `bash scripts/verify-no-panics.sh`
- `bash scripts/verify-safe-defaults.sh`
- `bash scripts/verify-network-hygiene.sh`
- `bash scripts/verify-expression-hardening.sh`
- `bash scripts/verify-encrypted-tag-implemented.sh`
- `bash scripts/verify-file-size.sh`
- `bash scripts/verify-maintainability-roadmap.sh`
- `bash scripts/verify-query-singleton.sh`
- `bash scripts/verify-validation-parity.sh`
- `bash scripts/fuzz-smoke.sh`
- `bash scripts/sec-gosec.sh`
- `bash scripts/sec-dependency-scans.sh`
- `go mod verify`

## Rubric-to-milestone mapping

| Rubric ID | Status | Milestone |
| --- | --- | --- |
| QUA-1 | ✅ | M1.5 |
| QUA-2 | ✅ | M1.5 |
| QUA-3 | ✅ | M1.5 |
| QUA-4 | ✅ | M3.5 |
| QUA-5 | ✅ | M3.5 |
| CON-1 | ✅ | M1 |
| CON-2 | ✅ | M1 |
| CON-3 | ✅ | M3.7 |
| COM-1 | ✅ | M2 |
| COM-2 | ✅ | M2 |
| COM-3 | ✅ | M0 |
| COM-4 | ✅ | M1 |
| COM-5 | ✅ | M1.5 |
| COM-6 | ✅ | M2 |
| COM-7 | ✅ | M2.5 |
| COM-8 | ✅ | M6 |
| SEC-1 | ✅ | M2 |
| SEC-2 | ✅ | M2 |
| SEC-3 | ✅ | M2 |
| SEC-4 | ✅ | M3 |
| SEC-5 | ✅ | M3 |
| SEC-6 | ✅ | M3.6 |
| SEC-7 | ✅ | M3 |
| SEC-8 | ✅ | M3.75 |
| MAI-1 | ✅ | M5 |
| MAI-2 | ✅ | M5 |
| MAI-3 | ✅ | M5 |
| DOC-1 | ✅ | M0 |
| DOC-2 | ✅ | M0 |
| DOC-3 | ✅ | M0 |
| DOC-4 | ✅ | M4 |
| DOC-5 | ✅ | M4 |

## Milestones (map directly to rubric IDs)

### M0 — Freeze rubric + planning artifacts

**Closes:** COM-3, DOC-1, DOC-2, DOC-3  
**Goal:** prevent goalpost drift by making the definition of “good” explicit and versioned.

**Acceptance criteria**
- Rubric exists and is versioned.
- Threat model exists and is owned.
- Evidence plan maps rubric IDs → verifiers → artifacts.

---

### M1 — Lint remediation (get `make lint` green)

**Closes:** CON-1, CON-2, COM-4  
**Goal:** remove surprises by making strict lint enforcement sustainable (no “works on my machine” exceptions).

Tracking document: `docs/development/planning/theorydb-lint-green-roadmap.md`

**Acceptance criteria**
- `golangci-lint config verify -c .golangci-v2.yml` is green.
- `bash scripts/fmt-check.sh` is green (no diffs).
- `make lint` is green (0 issues) with `.golangci-v2.yml` (no threshold loosening and no new blanket excludes).
- Any `//nolint` usage is line-scoped and justified; remove stale linter names (e.g., `unusedparams`, `unusedwrite`).

---

### M1.5 — Coverage remediation (hit 90% and keep it honest)

**Closes:** QUA-1, QUA-2, QUA-3, COM-5  
**Goal:** raise library coverage to **≥ 90%** without reducing the measurement surface.

Tracking document: `docs/development/planning/theorydb-coverage-roadmap.md`

**Prerequisite**
- M1 is complete (lint is green). During the coverage push, treat `make lint` as a regression gate and keep it green after every coverage pass.

**Acceptance criteria**
- `make test-unit` is green.
- `make integration` is green (DynamoDB Local).
- `bash scripts/verify-coverage-threshold.sh` is green (default threshold ≥ 90%).
- `bash scripts/verify-coverage.sh` is green at the default threshold (≥ 90%).

Guardrails (no denominator games):
- Do not exclude production packages from `scripts/coverage.sh` beyond the existing `examples/` + `tests/` filtering.
- If we need package-level floors, add a targets-based verifier (modeled after K3) rather than weakening the global gate.

---

### M2 — Enforce the loop in CI (after remediation)

**Closes:** COM-1, COM-2, COM-6, SEC-1, SEC-2, SEC-3  
**Goal:** run the recommended rubric surface on every PR with pinned tooling.

**Acceptance criteria**
- CI runs the recommended surface from `docs/development/planning/theorydb-10of10-rubric.md`.
- Tooling is pinned (no `@latest` for security-critical verifiers).

**Implementation (in repo)**
- Workflow: `.github/workflows/quality-gates.yml` runs `make rubric` on PRs to `premain` (and on pushes to `premain`).
- Tooling pins: `golangci-lint@v2.5.0`, `govulncheck@v1.1.4`, `gosec@v2.22.11` (plus `go.mod` toolchain `go1.25.6` via `go-version-file`).
- Integration infra pin: DynamoDB Local uses `amazon/dynamodb-local:3.1.0` (via `docker-compose.yml` and `DYNAMODB_LOCAL_IMAGE`).

---

### M2.5 — Determinism gates (integration stability)

**Closes:** COM-7  
**Goal:** reduce CI/non-CI drift by pinning integration infrastructure dependencies.

**Acceptance criteria**
- `bash scripts/verify-dynamodb-local-pin.sh` is green.
- No `amazon/dynamodb-local:latest` usage anywhere in the repo (including example `docker-compose.yml` files).

---

### M3 — Safety defaults (availability + security posture)

**Closes:** SEC-4, SEC-5, SEC-7  
**Goal:** make “safe by default” true in code paths that handle PHI/PII/CHD-like data, and prevent runtime crashers.

**Acceptance criteria**
- `bash scripts/verify-no-panics.sh` is green (no panics in production paths).
- `bash scripts/verify-safe-defaults.sh` is green (unsafe marshaling not wired into defaults).
- `bash scripts/verify-network-hygiene.sh` is green (HTTP timeouts + reviewed retry posture).

---

### M3.5 — Boundary hardening (validator parity + fuzz smoke)

**Closes:** QUA-4, QUA-5  
**Goal:** ensure inputs accepted by validators don’t crash downstream conversion/expression building, and add a cheap
“unknown unknown” detector for crashers.

**Acceptance criteria**
- `bash scripts/verify-validation-parity.sh` is green (no panics; errors are surfaced safely).
- `bash scripts/fuzz-smoke.sh` is green (bounded fuzz pass with at least one Fuzz target per package group).

---

### M3.6 — Expression boundary hardening (list index updates)

**Closes:** SEC-6  
**Goal:** remove “injection-by-construction” risks in expression building paths (especially list index updates).

**Acceptance criteria**
- `bash scripts/verify-expression-hardening.sh` exists, is green, and is wired into `make rubric`.
- Update-expression list index operations validate indexes (numeric-only) and fail closed on invalid syntax.
- Invalid field paths never get spliced into UpdateExpressions as raw strings.

---

### M3.7 — Public API contract parity (unmarshal/tag semantics)

**Closes:** CON-3  
**Goal:** ensure exported helpers (especially unmarshalling helpers) respect canonical TableTheory model tags and metadata semantics.

**Acceptance criteria**
- `bash scripts/verify-public-api-contracts.sh` exists, is green, and is wired into `make rubric`.
- `theorydb.UnmarshalItem` and stream-image unmarshalling behavior is consistent with canonical tag semantics (`pk`/`sk`/`attr:`/`encrypted`) or the API is explicitly changed/deprecated with a documented migration path.
- Contract tests cover the public boundary and fail on semantic drift (no “green by omission”).

---

### M3.75 — Implement `theorydb:"encrypted"` semantics (KMS Key ARN only)

**Closes:** SEC-8  
**Goal:** remove “metadata-only encryption” risk by implementing real field-level encryption semantics with a provided KMS key ARN.

Tracking document: `docs/development/planning/theorydb-encryption-tag-roadmap.md`

**Acceptance criteria**
- `bash scripts/verify-encrypted-tag-implemented.sh` is green.
- Encrypted fields fail closed if key ARN is not configured.
- Encrypted fields are rejected for PK/SK/index keys and are not queryable.

---

### M4 — Docs integrity + risk→control traceability

**Closes:** DOC-4, DOC-5  
**Goal:** prevent silent documentation drift and ensure every named top threat has at least one mapped control.

**Acceptance criteria**
- `bash scripts/verify-doc-integrity.sh` is green (internal links resolve; version claims match go.mod).
- `bash scripts/verify-threat-controls-parity.sh` is green (every `THR-*` maps to at least one control).

---

### M5 — Maintainability convergence (decompose + unify query)

**Closes:** MAI-1, MAI-2, MAI-3  
**Goal:** keep the codebase structurally convergent so future changes remain reviewable and safe.

Tracking document: `docs/development/planning/theorydb-maintainability-roadmap.md`

**Acceptance criteria**
- `bash scripts/verify-go-file-size.sh` is green.
- `bash scripts/verify-maintainability-roadmap.sh` is green.
- `bash scripts/verify-query-singleton.sh` is green.

---

### M6 — Branch + release supply chain (main release, premain prerelease)

**Closes:** COM-8  
**Goal:** make releases reproducible, reviewable, and automated (prerelease from `premain`, release from `main`).

**Acceptance criteria**
- `docs/development/planning/theorydb-branch-release-policy.md` exists and is current (branch strategy + protections + release process).
- `.github/workflows/prerelease.yml` exists and produces prereleases from merges to `premain`.
- `.github/workflows/release.yml` exists and produces releases from merges to `main`.
- Required protections for `premain` and `main` are documented and enforced (status checks required; direct pushes restricted).
