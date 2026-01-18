# TableTheory: Branch + Release Policy (main release, premain prerelease)

This document defines the intended branch strategy and release automation for TableTheory in high-risk usage contexts.

## Branches

- `premain` — prerelease integration branch (source of prereleases).
- `main` — release branch (source of stable releases).

## Merge flow (expected)

- Feature/fix work lands via PRs into `premain`.
- A release is cut by merging `premain` into `main` (via PR).
- Hotfixes may merge directly into `main` (then backported to `premain`).

## Post-release sync (required)

After a stable release is cut on `main`, immediately back-merge `main` into `premain` (via PR) so:

- `premain` carries the latest `.release-please-manifest.json` stable version, and
- prereleases do not remain on an older major/minor track.

If `.release-please-manifest.premain.json` is behind the latest stable version, reset it to the latest stable version
to start the next prerelease cycle from the correct baseline.

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

Additionally, quality/security workflows should run on PRs to (and/or pushes on) both protected branches:

- `.github/workflows/quality-gates.yml`
- `.github/workflows/codeql.yml`

## Notes

- This policy is intentionally tool-agnostic; the rubric requires automation and pinning, not a specific release tool.
- Branch protection rules are configured in the hosting platform (GitHub settings) and must be treated as part of the supply chain.
