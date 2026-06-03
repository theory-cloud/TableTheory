# TableTheory: Branch + Release Policy (main release, premain prerelease)

This document defines the intended branch strategy and release automation for TableTheory in high-risk usage contexts.

## Branches

- `staging` — integration branch (all work lands here first).
- `premain` — prerelease branch (source of prereleases).
- `main` — release branch (source of stable releases).

## Ownership

- `staging` owns integration-ready code, docs, security/toolchain updates, and release-cycle guardrails before they enter
  prerelease or stable lanes. After a stable release, `staging` must receive the latest stable baseline from `main` so the
  next cycle starts from the released state.
- `premain` owns prerelease state. The prerelease manifest `.release-please-manifest.premain.json` and SDK version files
  (`ts/package.json`, `ts/package-lock.json`, `py/src/theorydb_py/version.json`) may carry `X.Y.Z-rc.N` while an RC is in
  progress. Its stable manifest `.release-please-manifest.json` must stay aligned with the latest stable baseline.
- `main` owns stable state only. `.release-please-manifest.json`, `ts/package.json`, `ts/package-lock.json`, and
  `py/src/theorydb_py/version.json` must never contain `-rc` on `main`.

## Merge flow (expected)

- Feature/fix work lands via PRs into `staging`.
- An **RC** is cut by merging `staging` into `premain` (via PR), then merging the release-please prerelease PR.
- A **release** is prepared by creating a promotion branch from `origin/main`, merging `origin/premain` into that branch,
  normalizing RC-owned files to stable state, and opening a PR to `main`.
- A **release** is cut by merging the normalized promotion PR to `main`, then merging the release-please stable release PR.
- Hotfixes should still follow `staging` -> `premain` -> `main` so version lines stay aligned.

## Stable promotion path

Do not raw-merge `premain` into `main` and then fix `main`. Use this path instead:

1. Fetch without pruning and record `origin/staging`, `origin/premain`, and `origin/main` SHAs.
2. Create a promotion branch from `origin/main`.
3. Merge `origin/premain` into the promotion branch. Resolve conflicts only on the promotion branch.
4. Run `bash scripts/prepare-stable-promotion.sh --check`. Review the target stable version and planned file changes.
5. Run `bash scripts/prepare-stable-promotion.sh --write` when the plan is correct.
6. Run `bash scripts/verify-release-cycle-state.sh` and `bash scripts/verify-branch-release-supply-chain.sh`.
7. Open a PR from the normalized promotion branch to `main`.
8. After the promotion PR merges, allow `.github/workflows/release-pr.yml` to open the stable release-please PR.
9. Merge the stable release-please PR only after quality/security gates pass and the version is stable `X.Y.Z`, not
   `X.Y.Z-rc.N`.

`scripts/prepare-stable-promotion.sh` normalizes `.release-please-manifest.premain.json`, `ts/package.json`,
`ts/package-lock.json`, and `py/src/theorydb_py/version.json`. It validates but does not advance
`.release-please-manifest.json`; release-please must advance the stable manifest in the stable release PR.

### Pending stable promotion

The normalized promotion commit on `main` is a deliberate temporary state:

- `.release-please-manifest.json` remains at the previous stable version.
- `.release-please-manifest.premain.json`, `ts/package.json`, `ts/package-lock.json`, and
  `py/src/theorydb_py/version.json` are stable, non-prerelease, internally consistent, and may be one stable base ahead of
  `.release-please-manifest.json`.
- The state is valid only between the normalized promotion PR merge to `main` and the stable release-please PR merge.

Automation must make this state explicit. `.github/workflows/release-pr.yml` may verify with
`RELEASE_CYCLE_ALLOW_PENDING_STABLE_PROMOTION=true` so it can open the stable release-please PR. `.github/workflows/release.yml`
must classify the same state, verify it with the pending env var, and skip stable release creation until strict equality
is restored.

After the stable release-please PR merges, pending mode is no longer allowed operationally. Run
`bash scripts/verify-release-cycle-state.sh` without `RELEASE_CYCLE_ALLOW_PENDING_STABLE_PROMOTION`; the stable manifest
and SDK files must match again before the stable release workflow creates `vX.Y.Z`.

If the pending state persists because release-please did not open the stable PR, pause and investigate workflow logs,
permissions, release-please configuration, and COM-8 results. Do not patch `main`, hand-advance manifests, retag, or edit
GitHub releases to force the cycle forward.

Forbidden on `main`:

- `.release-please-manifest.json` set to an RC version.
- `ts/package.json`, `ts/package-lock.json`, or `py/src/theorydb_py/version.json` left at `X.Y.Z-rc.N`.
- A stable release PR titled or shaped as an RC release.
- A pending stable promotion that persists without an open stable release-please PR for the normalized stable version.
- Direct pushes or branch-ref API mutations to `main`, `premain`, or `staging` where this policy requires PR sync.

## Post-release sync (required)

After a stable release is cut on `main`, immediately back-merge `main` into `staging` (via PR) so:

- `staging` carries the latest `.release-please-manifest.json` stable version (and changelog/version files), and
- the next `staging` -> `premain` promotion will carry forward the correct stable baseline.

If `premain` is used directly after a stable release (without a `staging` promotion), back-merge `main` into `premain`
as well so prereleases do not remain on an older major/minor track.

If `.release-please-manifest.premain.json` is behind the latest stable version, reset it to the latest stable version
to start the next prerelease cycle from the correct baseline.

Acceptable post-stable sync paths:

- Preferred: PR from `main` to `staging`, then PR from `main` or updated `staging` to `premain`.
- Acceptable automation: a documented workflow that creates PR branches, runs COM-8 and SEC-2 checks, and never pushes
  directly to protected branches.
- Recovery: if a branch is already stranded, create a new PR branch from the correct base and replay only the needed file
  state. Do not retag, overwrite release assets, force-push, delete branches, or mutate GitHub releases.

## Protections (required)

Protect both `premain` and `main`:

- Require PRs (no direct pushes).
- Require CODEOWNERS/review approvals.
- Require CI status checks to pass (at minimum: `Quality Gates (10/10 Rubric)`).
- Restrict force-pushes and deletions.

## Automated releases (required)

This repo should publish:

- **Prereleases** on merges to `premain`.
- **Releases** on merges to `main`.

Recommended approach: **release-please** (merge-driven versioning + changelog updates) with:

- prerelease workflow producing tags like `vX.Y.Z-rc.N` (or an agreed convention), and
- release workflow producing stable `vX.Y.Z` tags and updating `CHANGELOG.md`.

### Release triggers (required)

`release-please` only cuts a new rc/release when there is at least one **release-eligible** (user-facing) commit since the previous tag. As a result:

- **Dependency/platform updates must use a release-eligible conventional commit type** (recommended: `fix(deps): ...`) so they produce an rc/release.
- Pure `chore(...)` commits may be treated as non-user-facing and can be skipped by `release-please`.

**Recommendation:** use squash-merge and set the squash title to a conventional commit that matches the intended version bump:

- Patch: `fix(deps): update multi-language dependencies`
- Minor: `feat: ...`
- Major: `feat!: ...` or include `BREAKING CHANGE:` in the body

### Release assets (required)

GitHub Releases must attach build artifacts for the non-Go SDKs:

- **TypeScript:** `npm pack` output from `ts/` (tarball)
- **Python:** wheel + sdist from `py/` (`python -m build`)

If a release workflow was expected to publish but `release_created` is false, pause before trying to patch files by hand.
If an asset upload or publish step has no `tag_name`, fail the workflow. If a GitHub release exists but TypeScript/Python
assets are missing, use a documented asset recovery workflow only after confirming the tag and release are immutable and
correct.

## Multi-language versioning (required)

- **Single shared repo version:** Go, TypeScript, and Python use the same GitHub tag/release version.
- **No registry publishing:** TypeScript is not published to npm and Python is not published to PyPI; GitHub releases are the source of truth.
- **Release automation must update TypeScript versions:** if `ts/package.json` exists, the prerelease/release workflows must
  update `ts/package.json` and `ts/package-lock.json` to match the repo version.
- **Release automation must update Python versions:** if `py/pyproject.toml` exists, the prerelease/release workflows must
  update `py/src/theorydb_py/version.json` to match the repo version.

## Required workflow artifacts (Rubric COM-8)

These files are required to exist and be kept current:

- `.github/workflows/prerelease.yml`
- `.github/workflows/release.yml`
- `scripts/verify-release-cycle-state.sh`
- `scripts/watch-release-cycle.sh`
- `scripts/prepare-stable-promotion.sh`

Additionally, quality/security workflows should run on PRs to (and/or pushes on) both protected branches:

- `.github/workflows/quality-gates.yml`
- `.github/workflows/codeql.yml`

## Notes

- This policy is intentionally tool-agnostic; the rubric requires automation and pinning, not a specific release tool.
- Branch protection rules are configured in the hosting platform (GitHub settings) and must be treated as part of the supply chain.

## Release watchpoints

Run `bash scripts/watch-release-cycle.sh` during a release and rerun with `--strict` before merge/release gates. Pause on:

- `origin/main` stable files containing `-rc`.
- `.release-please-manifest.json` set to an RC version.
- `origin/premain` stable manifest behind `origin/main`.
- `origin/staging` missing the latest stable baseline after a stable release.
- SEC-2/govulncheck still observing Go `1.26.3`.
- COM-8 branch/version sync failing.
- release-please opening a stable PR for an RC version.
- `origin/main` in pending stable promotion without an open stable release-please PR for the normalized stable version.
- a release workflow expected to create a release but not reporting `release_created`.
- asset/publish steps missing `tag_name`.
- a GitHub release existing without uploaded TypeScript/Python assets.
- automation attempting direct branch mutation where PR sync is required.
