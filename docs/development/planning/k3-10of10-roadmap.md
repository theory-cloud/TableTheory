# K3: 10/10 Roadmap (Rubric v0.2)

This roadmap is the execution plan for achieving and maintaining **10/10** across **Quality**, **Consistency**,
**Completeness**, **Security**, and **Compliance Readiness**, as defined by:

- `docs/planning/k3-10of10-rubric.md` (source of truth; versioned)

## Current scorecard (Rubric v0.2, 2026-01-07)

| Category | Grade | Blocking rubric items |
| --- | ---: | --- |
| Quality | 10/10 | — |
| Consistency | 10/10 | — |
| Completeness | 10/10 | — |
| Security | 10/10 | — |
| Compliance Readiness | 10/10 | — |

Scoring note: a check is only treated as “passing” if it is both green **and** enforced by a trustworthy verifier
(schema-valid config, non-diluted thresholds, no high-signal exclusions). COM failures invalidate “green by drift”.

Evidence (most recent):

- ✅ `go build ./...`
- ✅ `make test-unit`
- ✅ `make test-integration`
- ✅ `bash scripts/verify-coverage.sh` (Total Coverage 90.7% vs threshold 90%)
- ✅ `bash scripts/fmt-check.sh`
- ✅ `bash scripts/verify-go-modules.sh`
- ✅ `bash scripts/verify-ci-toolchain.sh`
- ✅ `golangci-lint config verify -c .golangci-v2.yml`
- ✅ `golangci-lint run --timeout=5m --config .golangci-v2.yml`
- ✅ `bash scripts/verify-coverage-threshold.sh`
- ✅ `bash scripts/verify-logging-standards.sh`
- ✅ `bash scripts/verify-planning-docs.sh`
- ✅ `go mod verify`
- ✅ `go test ./test/security -run TestPCIDSSRequirement3_2_2_Compliance`
- ✅ `bash scripts/verify-sec-gosec-config.sh`
- ✅ `golangci-lint run --timeout=5m --config .golangci-v2.yml --enable-only=gosec`
- ✅ `bash scripts/sec-gosec.sh`
- ✅ `bash scripts/sec-govulncheck.sh`

## Rubric-to-milestone mapping

| Rubric ID | Status | Milestone |
| --- | --- | --- |
| QUA-1 | ✅ | M1.5 |
| QUA-2 | ✅ | M1.5 |
| QUA-3 | ✅ | M1.5 |
| CON-1 | ✅ | M1 |
| CON-2 | ✅ | M1 |
| COM-1 | ✅ | M1 |
| COM-2 | ✅ | M1 |
| COM-3 | ✅ | M1 |
| COM-4 | ✅ | M1 |
| COM-5 | ✅ | M2 |
| COM-6 | ✅ | M1 |
| SEC-1 | ✅ | M2 |
| SEC-2 | ✅ | M2 |
| SEC-3 | ✅ | M2 |
| SEC-4 | ✅ | M2 |
| CMP-1 | ✅ | M0 |
| CMP-2 | ✅ | M0 |
| CMP-3 | ✅ | M0 |

## Milestones (map directly to rubric IDs)

### M0 — Freeze scope + planning artifacts ✅

**Closes:** CMP-1, CMP-2, CMP-3  
**Goal:** prevent goalpost drift and establish audit-ready planning structure for the K3 CDE.

**Status:** ✅ Complete (Rubric v0.2, 2026-01-06)

**Acceptance criteria**
- Controls matrix exists for PCI DSS v4.0.1 and includes K3 scope summary.
- Rubric is versioned and deterministic.
- Threat model exists (even if some items are `unknown`).
- Evidence plan documents where evidence comes from (CI artifacts, reports, runbooks).

**Suggested verification**
```bash
bash scripts/verify-planning-docs.sh
```

---

### M1 — Make the CI-quality loop reproducible locally

**Closes:** CON-1, CON-2, COM-1, COM-2, COM-3, COM-4, COM-6  
**Goal:** make “what CI checks” runnable and repeatable for developers/agents.

**Status:** ✅ Complete (2026-01-06) — core developer loop is stable and verifiers are trustworthy; the remaining work is a dedicated test/coverage push (M1.5) and security hardening (M2).

**Notes / known inconsistencies**
- COM-5 is part of the **Completeness** scorecard, but is mapped to **M2** because it is coupled to security hardening (SEC-1) and requires explicit risk decisions (no “green by exclusion”).

**Acceptance criteria**
- CI checks are runnable locally with deterministic commands.
- Failing gates produce actionable output (no “silent green”, no placeholder verifiers, no diluted thresholds).
- Work is executed in two stages:
  - **Stage A (application remediation):** restore `go build` + `golangci-lint` reliability with tests excluded.
  - **Stage B (test remediation):** make tests meaningful and meet the 90% coverage bar (tracked as **M1.5** so it does not become an afterthought).

#### Stage A — Application remediation (tests excluded)

**Why:** unblock the core developer loop (build + lint) before spending cycles on broken tests.

**Logging consistency workstream (Lift-only; core to CON-2 readiness)**
- **Single logging API:** Lift structured logging everywhere (Zap is an implementation detail inside Lift).
- **Request path:** use `ctx.Logger` in handlers and middleware.
- **Non-request path:** use `pkg/logger.LiftLogger` only as a fallback for startup/background code.
- **Banned patterns in app code:** `logger.Logger`, `logger.Sugar`, printf-style logging, direct `go.uber.org/zap` usage.
- **Data hygiene:** never log PAN/CVV/API keys; mask tokens and rely on Lift sanitization as a backstop.
- **Enforcement (once lint runs):** add import guards to prevent re-introducing direct Zap usage outside Lift internals.

**Suggested verification**
```bash
go build ./...
bash scripts/fmt-check.sh
# Lint config should set run.tests: false during Stage A (see Lesser example).
golangci-lint run --timeout=5m --config .golangci-v2.yml
```

#### Completeness drift checks (COM-*) — keep standards honest

These checks are intentionally “meta”: they catch diluted gates (invalid lint config, low coverage thresholds, CI drift)
even when the primary commands happen to be green.

**Suggested verification**
```bash
bash scripts/verify-go-modules.sh
bash scripts/verify-ci-toolchain.sh
golangci-lint config verify -c .golangci-v2.yml
bash scripts/verify-coverage-threshold.sh
bash scripts/verify-logging-standards.sh
```

---

### M1.5 — Make tests meaningful and hit 90% coverage (Quality gates)

**Closes:** QUA-1, QUA-2, QUA-3  
**Goal:** turn tests into a safety net and meet the **90%** coverage bar without “hiding” production code.

**Status:** ✅ Complete (2026-01-07)

**Why this was a dedicated milestone**
- The coverage delta was large enough that it could not be solved “along the way” without an explicit plan and sustained focus.
- With this complete, larger refactors (including security hardening) have a meaningful regression net (as long as the gates stay enforced).

**Gate policy (to prevent drift)**
- Treat **M1.5 completion as a prerequisite** for claiming later milestones “done” (except narrow hotfixes / risk-reducing security patches).
- Do not accept “green by exclusion”: no shrinking `-coverpkg`, no excluding packages/directories to hit the threshold.

**Reality check (current state)**
- Total coverage: **90.7%** (covered **9,796** / total **10,807** statements)
- Package floors: ✅ all packages ≥ 90% (`docs/planning/coverage-targets/k3-cov-6.tsv`)
- Gate status: ✅ `bash scripts/verify-coverage.sh`

**Work breakdown (what it took)**

1) **Test harness hardening (make tests fast and deterministic)**
- Stabilize DynamoDB Local setup for integration tests (consistent endpoints, teardown, fixtures).
- Reduce test flakiness and non-determinism (time, randomness, network calls; use dependency injection).
- Build reusable test utilities (builders, fakes, fixtures) to make adding tests cheap.

2) **Unit coverage ramp in high-leverage packages**
- Add unit tests to pure/near-pure packages first (e.g., `pkg/money`, `pkg/validation`, `internal/config`).
- Refactor “hard to test” logic into pure functions where possible, keeping behavior unchanged.

3) **Service + handler coverage ramp (dominant scope)**
- `internal/services/impl`: add tests that cover business rules, error handling, retries, and edge cases.
- `internal/handlers`: add HTTP tests via `net/http/httptest` and table-driven cases; validate status codes, bodies, and error mapping.
- `internal/repositories`: add tests for query/build logic; use DynamoDB Local where integration is required.

4) **Re-enable lint on test files (durability)**
- Flip `.golangci-v2.yml:run.tests` to `true` once `_test.go` code is stable so tests can’t rot unlinted.
  - Current state: `.golangci-v2.yml:run.tests` is `true`.

**Acceptance criteria**
- `make test-unit` is green.
- `make test-integration` is green.
- `bash scripts/verify-coverage.sh` is green at the default threshold (≥ 90%).

**Suggested verification**
```bash
make test-unit
make test-integration
bash scripts/verify-coverage.sh
bash scripts/fmt-check.sh
golangci-lint run --timeout=5m --config .golangci-v2.yml
```

---

### M2 — Install P0 security gates (and keep them green)

**Closes:** SEC-1, SEC-2, SEC-3, SEC-4, COM-5  
**Goal:** prevent regressions in the highest-risk control surface (CDE).

**Status:** ✅ Complete (2026-01-07)

**Current state (baseline)**
- SEC-3 ✅ `go mod verify`
- SEC-4 ✅ `go test ./test/security -run TestPCIDSSRequirement3_2_2_Compliance`
- SEC-2 ✅ `bash scripts/sec-govulncheck.sh`
- COM-5 ✅ `bash scripts/verify-sec-gosec-config.sh`
- SEC-1 ✅ `bash scripts/sec-gosec.sh`

**Acceptance criteria**
- Gosec and govulncheck run in CI (or a security pipeline) and are treated as blocking for P0.
- `SEC-4` PCI P0 regression test is treated as blocking.
- Evidence artifacts are retained in CI (SARIF/reports).
- Security scan configuration is not diluted (no excluded high-signal gosec rules without an explicit exception).

#### M2.1 — Fix govulncheck blockers (SEC-2)

**Closes:** SEC-2  
**Goal:** `bash scripts/sec-govulncheck.sh` stays green.

**Resolved (baseline)**
- `github.com/go-viper/mapstructure/v2@v2.2.1` flagged by govulncheck (e.g., `GO-2025-3900`, `GO-2025-3787`); fixed in `>= v2.4.0`.

**Work**
- Upgrade `github.com/go-viper/mapstructure/v2` to a fixed version (`>= v2.4.0`) and run `go mod tidy`.
- Re-run `make test-unit`, `make test-integration`, `golangci-lint run --timeout=5m --config .golangci-v2.yml` to ensure the upgrade doesn’t regress runtime behavior.
- Re-run `bash scripts/sec-govulncheck.sh` until it is clean.

**Done when**
- `bash scripts/sec-govulncheck.sh` exits 0 with no findings.

#### M2.2 — Make gosec scan green (SEC-1)

**Closes:** SEC-1  
**Goal:** `bash scripts/sec-gosec.sh` stays green without “papering over” real risk.

**Resolved (baseline)**
- Hardcoded secret present in test tooling (`appsync_testing/...`) that must be removed and rotated.
- Unsafe integer conversions/bit shifts flagged in retry logic (`pkg/cryptography/errors.go`) and tooling (`tools/migration-test/...`) if in scope.
- Unchecked I/O flagged in examples/tests (e.g., `test/examples/async_callback_example.go`).
- `G101` false positives on non-secret error-code constants (e.g., `internal/constants/constants.go`, `pkg/errors/mappings.go`) requiring narrow `#nosec` justification.
- Scan/build errors caused by nested Go modules that currently do not compile (`pkg/cdk/...`, `infrastructure/cdk/...`, `appsync_testing/...`, `tools/migration-test/...`).

**Work**
- Eliminate **any real secrets** committed to the repo (e.g., hardcoded API keys in test tooling); replace with env vars + `.env.example` where appropriate.
- Fix gosec HIGH/MED findings (e.g., unsafe integer conversions and unchecked I/O) in in-scope code.
- For gosec **false positives** (commonly `G101` on non-secret constants), add narrowly-scoped `#nosec` with a justification comment (or adjust gosec configuration if justified).
- Resolve gosec scan/build failures caused by nested Go modules:
  - **Preferred (scope-aligned):** make gosec scan only the root module packages (CDE in-scope) and explicitly document exclusions for nested modules that are not part of the deployed runtime.
  - **Alternate (full-repo):** run gosec per-module and bring each nested module back to a compiling state before enabling it as a blocking gate.

**Done when**
- `bash scripts/sec-gosec.sh` exits 0 with no findings.

#### M2.3 — Lock in evidence + CI integration (SEC-*)

**Closes:** supports SEC-1/SEC-2 durability  
**Goal:** “security green” stays green and produces audit-friendly artifacts.

**Work**
- Add CI jobs for:
  - `bash scripts/sec-gosec.sh` (export SARIF artifact)
  - `bash scripts/sec-govulncheck.sh` (export JSON/text artifact)
  - `go mod verify`
  - `go test ./test/security -run TestPCIDSSRequirement3_2_2_Compliance`
- Pin tool versions for determinism (replace `@latest` with explicit versions) once green; treat upgrades as explicit PRs.
  - Current pins: `GOSEC_VERSION=v2.22.11`, `GOVULNCHECK_VERSION=v1.1.4`.

**Suggested verification**
```bash
bash scripts/sec-gosec.sh
bash scripts/verify-sec-gosec-config.sh
bash scripts/sec-govulncheck.sh
go mod verify
go test ./test/security -run TestPCIDSSRequirement3_2_2_Compliance
```
