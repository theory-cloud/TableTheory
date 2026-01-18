# TableTheory: Rubric vNext Notes (v0.7+)

This file captures **potential rubric improvements** and **process upgrades** discovered during remediation work.
Items are only part of the rubric once explicitly adopted and versioned in `docs/development/planning/theorydb-10of10-rubric.md`.

## Status

- Adopted in rubric `v0.3` (2026-01-10): COM-6, COM-7, SEC-4, SEC-5, SEC-7, QUA-4, QUA-5, DOC-4, DOC-5 (and supporting verifiers + roadmap milestones).
- Adopted in rubric `v0.4` (2026-01-10): MAI-1, MAI-2, MAI-3, SEC-8 (and supporting verifiers + roadmap milestones).
- Adopted in rubric `v0.5` (2026-01-11): CON-3, SEC-6, COM-8 (and supporting roadmap milestones).
- This document now tracks *new* candidates for `v0.7+` and captures lessons learned while hardening the rubric surface.

## Why vNext exists (the “10/10 but…” problem)

Rubric `v0.2` was intentionally minimal and heavily tool-driven (tests/lint/coverage/scanners). That’s good for repeatability,
but it can miss **design-level safety defaults** and **runtime crashers** that don’t show up as lint or SAST findings.

Rubric `v0.3` adopted several “10/10 but…” safety gates, but vNext still exists because new classes of failure will
continue to appear as the codebase and usage evolves.

Concrete examples of “passes v0.2, still risky” categories:

- **Availability regressions** that only appear at runtime (e.g., panics on invalid inputs).
- **Security posture drift** where a “safe default” exists but is not actually the default code path.
- **Docs drift** (broken internal links, version claims that no longer match reality).
- **Validator / converter parity** gaps (validator accepts values that later fail conversion, causing panics or partial
  failures).
- **Network hygiene** defaults (timeouts/retries) that are safe in apps but dangerous when embedded in a library.

The purpose of this file is to capture those gaps as *candidate* rubric items so we can decide which ones are worth
turning into deterministic gates.

## Adopted in v0.3 (historical record)

These items are preserved here as context for why `v0.3` exists and how/why the gates were introduced.

### 1) Make “CI enforces rubric” a scored rubric item

Today, M2 is a roadmap milestone but not directly scored. Consider adding a new **Completeness** item (e.g., `COM-6`)
that verifies:

- A workflow exists under `.github/workflows/` that runs `make rubric` for PRs to `premain`.
- Security-critical tools are pinned (no `@latest`).
- The workflow uploads at least `coverage_lib.out` and `gosec.sarif` as artifacts.

Rationale: prevents “10/10 locally” from drifting without CI enforcement.

### 2) Pin DynamoDB Local image (integration determinism)

Integration tests depend on DynamoDB Local. Consider a verifier to ensure we do not use `:latest` for:

- `docker-compose.yml` image tags
- Any `docker run amazon/dynamodb-local...` fallbacks

Rationale: reduces CI/non-CI drift when upstream images change.

### 3) Add a dedicated “CI surface parity” verifier

Current `scripts/verify-ci-toolchain.sh` checks for `go-version-file: go.mod` and rejects `@latest`, but it does not
assert that the **recommended rubric surface** is actually executed in CI. Consider a separate script (and rubric item)
that validates the expected workflow/commands exist.

---

## Candidates discovered during security-critical review (catch earlier)

### 4) Ban panics in first-party, non-test code paths (availability + DoS hardening)

**Proposed rubric item:** `SEC-4` (or `QUA-4`) — “No `panic(...)` in non-test production code, unless explicitly
allowlisted with a justification comment.”

**Verifier:** add `bash scripts/verify-no-panics.sh`

- Scans `*.go` excluding `*_test.go`, `examples/`, and docs fixtures.
- Fails on `panic(` unless the file+line is allowlisted.
- Allows panics in explicitly scoped test helper packages (e.g., `pkg/testing/`, `pkg/mocks/`) if we still want them.

**Evidence:** verifier output in CI logs.

**Rationale:** panics are an availability vulnerability in high-risk domains. They also mask validation mismatches
(“validator accepts X, converter panics on X”).

**Adoption notes (to keep it honest):**
- Introduce an allowlist mechanism that’s explicit and reviewable (e.g., `// theorydb: allow-panic <reason>`).
- Prefer returning typed errors over panics in expression/value processing.

### 5) “Safe by default” marshaling must be *actually* default (unsafe only via explicit opt-in)

**Proposed rubric item:** `SEC-5` — “Default `tabletheory.New(...)` path uses memory-safe marshaling; unsafe marshaling is
only available behind explicit opt-in + acknowledgment.”

**Verifier options:**

Option A (preferred): a unit test + structural check
- `go test` asserts the default DB uses the safe marshaler (and that the unsafe marshaler requires explicit config).
- `bash scripts/verify-safe-defaults.sh` asserts we don’t wire `marshal.New(...)` (unsafe) into defaults.

Option B: structural-only
- Grep-based verifier that checks `tabletheory.New` and `NewLambdaOptimized` construct marshalers via a safe factory.

**Evidence:** test output + verifier output.

**Rationale:** “safe default exists but isn’t the default” is one of the easiest ways to accidentally ship risk into a
security-critical fleet.

### 6) Validator ↔ converter parity gate (no “validated but crashes” inputs)

**Proposed rubric item:** `QUA-4` (or `SEC-6`) — “Any value accepted by public validators must not cause panics or
undefined behavior when fed into expression building / marshaling.”

**Verifier:** add a focused unit test suite (and/or fuzz smoke test) that:
- enumerates representative values that validators accept (including maps/slices/structs),
- runs them through the same conversion used by expression building (placeholders + `AttributeValue` conversion),
- asserts: **no panics**, and failures are returned as **typed errors** (not silent NULL substitutions).

**Evidence:** `make test-unit` output (and a named test we can point at in incident writeups).

**Rationale:** this is the exact class of “validation gap” that’s hard to spot with lint/SAST and easy to miss in
regular unit coverage.

### 7) Add fuzz “smoke tests” for crashers in core primitives (expr + marshal + cursor)

**Proposed rubric item:** `QUA-5` — “Run a short fuzz pass that targets panic-prone boundaries.”

**Verifier:** add `make fuzz-smoke` (or `bash scripts/fuzz-smoke.sh`) that runs a bounded fuzz time, e.g.:

- `go test ./internal/expr -run '^$' -fuzz Fuzz -fuzztime=10s`
- `go test ./pkg/marshal -run '^$' -fuzz Fuzz -fuzztime=10s`
- `go test ./pkg/query -run '^$' -fuzz Fuzz -fuzztime=10s`

**Evidence:** CI logs (and optionally generated corpus as an artifact if we want reproducibility).

**Rationale:** fuzzing is the cheapest “unknown unknown” detector for panics, infinite loops, and surprising
type/reflect behavior.

### 8) Docs integrity gates (internal links + “claims match code”)

**Proposed rubric item:** `DOC-4` — “No broken internal doc links; critical version claims match the codebase.”

**Verifier:** add `bash scripts/verify-doc-integrity.sh` that:
- checks all repo-local markdown links resolve to files that exist (at least under `README.md` and `docs/`),
- flags common drift vectors:
  - referenced files that don’t exist,
  - Go version badges/claims that don’t match `go.mod`.

**Evidence:** verifier output.

**Rationale:** broken docs are “silent regressions” that reduce safe usage in high-risk domains; version drift also
creates toolchain confusion that can degrade the quality of downstream builds.

### 9) Network hygiene defaults (timeouts + retry policy posture)

**Proposed rubric item:** `SEC-7` (or `QUA-6`) — “HTTP clients constructed by TableTheory have explicit timeouts, and retry
behavior is intentional, documented, and testable.”

**Verifier (minimal, deterministic):**
- `bash scripts/verify-network-hygiene.sh` flags:
  - `http.Client{}` / `&http.Client{}` without a non-zero `Timeout` in non-test code,
  - `aws.NopRetryer{}` usage outside tests unless behind an explicit feature flag/config that is documented.

**Evidence:** verifier output in CI logs.

**Rationale:** “no timeouts” and “no retries” are both common causes of production incidents; they matter more in a
library because they become transitive defaults for multiple services.

### 10) Threat model ↔ controls matrix parity gate (no “named threat without control”)

**Proposed rubric item:** `DOC-5` (or `COM-7`) — “Every threat in the threat model has at least one mapped control ID in
the controls matrix.”

**Verifier:** a deterministic doc check (no NLP required):
- Require the threat model “Top threats” section to enumerate threats with stable IDs (e.g., `THR-1`).
- Require the controls matrix to reference those `THR-*` IDs in a dedicated column.
- Verify: all `THR-*` are referenced at least once in the controls matrix.

**Evidence:** verifier output.

**Rationale:** the rubric should not be able to reach “10/10” while known top threats are explicitly listed as gaps
with no controls.

---

## Roadmapping process improvements (so gaps become gates, not trivia)

### A) Add a “rubric blind spot review” step to milestone closure

When closing a milestone in `docs/development/planning/theorydb-10of10-roadmap.md`, add a short checklist item:

- “Did we discover any new *classes* of failure not covered by rubric verifiers?”
- If yes: add an entry in this file (with a proposed verifier) and link it from the milestone notes.

This turns reviews into durable gates instead of one-off findings.

### B) Require “verifier first” framing for high-risk changes

For changes touching expression building, marshaling, transactions, retries, or logging:

- PR description includes: “Which rubric IDs does this affect?”
- If the answer is “none”, require a sentence explaining why (to avoid accidental scope creep).
- If a new class of bug is possible: add a *candidate* verifier in this file even if it isn’t adopted yet.

### C) Make adoption explicit: proposal → implementation → adoption PR

Suggested workflow for vNext items:

1. **Propose:** add to this file (what/why/verifier).
2. **Implement:** land the script/test behind `make verify-*` without scoring changes.
3. **Adopt:** bump rubric version, assign points, wire verifier into `scripts/verify-rubric.sh`, and update the roadmap.

This keeps “good ideas” from getting lost, and keeps the rubric versioning discipline intact.

---

## v0.7+ candidates (new)

### 11) Add a “rubric report” mode (non fail-fast)

`make rubric` is intentionally fail-fast, but roadmapping benefits from a full pass/fail report.

**Proposed addition:**
- Add `make rubric-report` (or `bash scripts/verify-rubric-report.sh`) that runs the rubric surface and prints a table of pass/fail per rubric ID (and exits non-zero if any fail).

**Rationale:** reduces “first failure hides the rest” during remediation and makes scorecard refreshes faster.

### 12) Verifier authoring rules (to avoid false red/false green)

When adding a new verifier (shell or Go harness), require these properties up front:

- **Scope discipline:** only scan relevant files (e.g., `--glob '*.go'`) and exclude the verifier itself to avoid self-matches.
- **Correct error handling:** treat `rg` exit codes `>1` as a hard failure; never mask parse errors with `|| true`.
- **Determinism:** pin tools/images; bound time-based checks (fuzz); avoid flaky network calls in verifiers.
- **Actionable output:** print `file:line` plus the minimum remediation hint needed to fix the failure.

**Rationale:** verifier bugs can create churn (false red) or hide risk (false green), both of which are unacceptable in a high-risk domain.

### 13) “Defaults are safe” unit tests for public constructors

Verifier scripts help, but the most robust check is a direct unit test for the exported constructors.

**Proposed addition:**
- Add a unit test that asserts `tabletheory.New(...)` uses safe-by-default marshaling and that any unsafe option is explicit and named accordingly.

**Rationale:** reduces the chance of verifier drift and documents the contract in code.
