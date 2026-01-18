# TableTheory Rubric v0.5 Debrief (SEC-6 / CON-3 / COM-8)

Date: 2026-01-11  
Repo: `theory-cloud/tabletheory`  
Context: security-critical (healthcare + cardholder data)

This file captures the context and decisions behind the **rubric v0.5** rollout so a new Codex session (or a human reviewer) can quickly orient.

## What changed (high level)

- The rubric and roadmap were updated to add and enforce new gates:
  - **SEC-6** (expression boundary hardening)
  - **CON-3** (public API contract parity; explicitly includes unmarshalling consistency)
  - **COM-8** (branch + release supply chain for `premain`/`main`)
- All new gates were remediated to green locally (`make rubric`) and in CI.
- Automated prereleases from `premain` and stable releases from `main` were implemented using **Release Please**.

## Where to start (key artifacts)

- Rubric: `docs/development/planning/theorydb-10of10-rubric.md`
- Roadmap: `docs/development/planning/theorydb-10of10-roadmap.md`
- Rubric runner (single entrypoint for CI + local): `scripts/verify-rubric.sh` (via `make rubric`)
- Branch/release policy: `docs/development/planning/theorydb-branch-release-policy.md`

## Gate-by-gate summary

### SEC-6 — Expression boundary hardening

**Problem:** update expression construction accepted untrusted/unchecked field strings (including list-index syntax), enabling injection-by-construction in some update paths.

**Remediation (merged):**
- Hardened list index update expression construction by parsing list-index operations and validating field names via `pkg/validation`.
- `internal/expr/builder.go`: `AddUpdateRemove` now returns `error` and validates fields (call sites updated); `AddUpdateAdd` / `AddUpdateDelete` validate fields.
- Added a deterministic verifier and a small harness program to ensure the hardening stays enforced.
- Added unit tests targeting list-index parsing and field validation behavior.

**Primary commits:** `95d22b7`  
**Primary artifacts:**
- Verifier: `scripts/verify-expression-hardening.sh`, `scripts/internal/expression_hardening/main.go`
- Tests: `internal/expr/list_index_operation_test.go`, `internal/expr/update_field_validation_test.go`

### CON-3 — Public API contract parity (UnmarshalItem)

**Problem (consistency failure):** `pkg/query.UnmarshalItem` behavior diverged from canonical TableTheory tag + naming semantics, creating inconsistent marshaling/unmarshaling expectations across the public API surface. This is especially risky for security-critical use where attribute naming, key fields, and “encrypted” semantics must be deterministic.

**Remediation (merged):**
- `pkg/query.UnmarshalItem` now:
  - Requires `dest` be a non-nil pointer to a struct (avoids reflect panics / partial decodes).
  - Resolves attribute names using TableTheory conventions (including `theorydb:"naming:snake_case"` and untagged exported fields via `pkg/naming`).
  - Preserves `dynamodb:"..."` tag precedence when present.
  - Fails closed for encrypted envelope shapes when encryption isn’t configured, returning an error (instead of silently decoding garbage).
- Added a deterministic verifier harness plus focused tests to lock contract behavior.

**Primary commits:** `b2d580b`  
**Primary artifacts:**
- Verifier: `scripts/verify-public-api-contracts.sh`, `scripts/internal/public_api_contracts/main.go`
- Tests: `pkg/query/unmarshal_contract_test.go`

### COM-8 — Branch + release supply chain (`premain` prerelease, `main` release)

**Goal:** treat `premain` as the prerelease source and `main` as the stable release source, with protections + automation (supply chain).

**Implementation (merged):**
- Added Release Please workflows:
  - `.github/workflows/prerelease.yml` (runs on `premain`)
  - `.github/workflows/release.yml` (runs on `main`)
- Added Release Please config + manifest files:
  - `release-please-config.premain.json` + `.release-please-manifest.premain.json`
  - `release-please-config.json` + `.release-please-manifest.json`
- Updated CI workflows to run on both branches (`premain`, `main`):
  - `.github/workflows/quality-gates.yml`
  - `.github/workflows/codeql.yml`
  - `.github/workflows/unit-cover.yml`
- Added a deterministic verifier and enforced policy doc:
  - `scripts/verify-branch-release-supply-chain.sh`
  - `docs/development/planning/theorydb-branch-release-policy.md`

**Primary commits:** `cd5d25b`

## CI/tooling issues encountered (and fixed)

### GitHub runner missing `rg` (ripgrep)

Some rubric scripts use `rg`. GitHub Actions runners don’t guarantee `ripgrep` is installed.

**Fix (merged):** install `ripgrep` in `.github/workflows/quality-gates.yml`.  
**Commit:** `807df03`

### golangci-lint v2 install path mismatch

This repo uses `.golangci-v2.yml`, which requires **golangci-lint v2**. Installing via the old module path can inadvertently install v1 (or fail), causing config schema verification to fail in CI.

- ✅ Correct: `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0`
- ❌ Incorrect: `go install github.com/golangci/golangci-lint/cmd/golangci-lint@v2.5.0`

**Fix (merged):**
- `.github/workflows/quality-gates.yml` installs the `/v2` module path.
- `make install-tools` installs the same pinned `/v2` version.

**Commit:** `d9b270f`

### CodeQL: unpinned third-party GitHub Action tags

CodeQL flagged `actions/unpinned-tag` for `googleapis/release-please-action@v4` because major-version tags are mutable and therefore not supply-chain safe.

**Fix (PR):** pin `googleapis/release-please-action` to an immutable commit SHA in both `.github/workflows/release.yml` and `.github/workflows/prerelease.yml`, and update `scripts/verify-branch-release-supply-chain.sh` to require SHA pinning.  
**PR:** `#25`

## Release evidence (what happened after merge)

- PR `#22` merged into `premain` and all checks passed.
- Release Please opened and merged PR `#23` (`chore(premain): release 1.1.0-rc`).
- A prerelease was created: `v1.1.0-rc` (tag + GitHub prerelease).

## How to validate locally (the contract we enforced)

- Run rubric: `make rubric`
  - Includes formatting checks, lint, unit tests, integration tests, coverage enforcement, bounded fuzz smoke, and security scanning (gosec + govulncheck).
  - Runs the deterministic verifiers for SEC-6 / CON-3 / COM-8.

## Recommended next follow-ups (still mostly manual)

- Configure GitHub branch protections for `premain` and `main` to match `docs/development/planning/theorydb-branch-release-policy.md` (require PRs, reviews/CODEOWNERS, required checks, restrict force-push/deletes).
- Ensure GitHub Actions repo settings allow Release Please to create/merge release PRs (workflow permissions, and optional auto-merge policy for release PRs).
