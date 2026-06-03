# TableTheory: Branch + Release Policy (main release, premain prerelease)

This document defines the intended branch strategy and release automation for TableTheory in high-risk usage contexts.

## Branches

- `staging` — integration branch (all work lands here first).
- `premain` — prerelease branch (source of prereleases).
- `main` — release branch (source of stable releases).

## Ownership

- `staging` owns integration-ready code, docs, security/toolchain updates, and release-cycle guardrails before they enter
  prerelease or stable lanes. After a stable release, `staging` must receive the latest stable baseline from `main` so the
  next cycle starts from the released state. During active RC reconciliation, `staging` may temporarily carry the current
  `premain` RC lane in `.release-please-manifest.premain.json` and SDK version files, but only while the stable manifest
  remains aligned with `main` and the RC files are internally consistent and ahead of that stable baseline.
- `premain` owns prerelease state. The prerelease manifest `.release-please-manifest.premain.json` and SDK version files
  (`ts/package.json`, `ts/package-lock.json`, `py/src/theorydb_py/version.json`) may carry `X.Y.Z-rc.N` while an RC is in
  progress. Its stable manifest `.release-please-manifest.json` must stay aligned with the latest stable baseline.
- `main` owns stable state only. Outside explicit pending stable promotion, `.release-please-manifest.json`,
  `.release-please-manifest.premain.json`, `ts/package.json`, `ts/package-lock.json`, and
  `py/src/theorydb_py/version.json` must not contain `-rc` on `main`.

## Merge flow (expected)

- Feature/fix work lands via PRs into `staging`.
- An **RC** is cut by merging `staging` into `premain` (via PR), then merging the release-please prerelease PR.
- A **release** is prepared by verifying the intended RC release, then opening and merging the `premain` -> `main`
  promotion PR.
- The `main` release workflow skips publishing while pending stable promotion is present, and `release-pr.yml` opens the
  stable release-please PR with `release-as` computed from the premain RC baseline. Pull-request quality checks for the
  `premain` -> `main` promotion may verify this state with explicit pending stable-promotion mode.
- A **release** is cut by merging the stable release-please PR, which normalizes the stable manifest, prerelease manifest,
  and SDK version files to stable state before the stable release workflow publishes `vX.Y.Z`.
- Hotfixes should still follow `staging` -> `premain` -> `main` so version lines stay aligned.

## Post-release sync (automated)

After a stable release is published on `main`, the `Release (main)` workflow runs
`scripts/sync-post-stable-release-baselines.sh`. The sync commits the stable release baseline back to
`premain` and `staging`:

- `.release-please-manifest.json`
- `.release-please-manifest.premain.json` reset to the newly published stable version
- `CHANGELOG.md`
- `py/src/theorydb_py/version.json`
- `ts/package.json`
- `ts/package-lock.json`

This replaces manual manifest resets and manual post-release `main` -> `premain` / `main` -> `staging` release-baseline
back-merges. The next `staging` -> `premain` promotion starts from the latest stable version, and the subsequent
prerelease PR advances from that baseline.

The normal stable promotion path does not use a local stable-normalization branch. `release-please-config.json` must keep
`.release-please-manifest.premain.json`, `ts/package.json`, `ts/package-lock.json`, and
`py/src/theorydb_py/version.json` wired as stable release-please extra-files so the generated stable Release PR owns all
version-file normalization.

Acceptable post-stable sync paths:

- Preferred: PR from `main` to `staging`, then PR from `main` or updated `staging` to `premain`.
- Acceptable automation: a documented workflow that creates PR branches, runs COM-8 and SEC-2 checks, and never pushes
  directly to protected branches.
- Recovery: if a branch is already stranded, create a new PR branch from the correct base and replay only the needed file
  state. Do not retag, overwrite release assets, force-push, delete branches, or mutate GitHub releases.

## Immutable release version reuse

GitHub release tag names are one-time-use for TableTheory release-cycle purposes. If a published immutable release or its
git tag is deleted, that version name is still exhausted and must not be reused. Do not manually recreate the tag, publish
a replacement release by hand, edit immutable release assets, or patch manifests to make the same version run again.

If a publish step fails with `tag_name was used by an immutable release`, recover through the normal PR-driven release
flow: land a release-eligible change, promote it through the documented branch path, and let release-please advance to the
next RC or stable version for that lane.

If a version is abandoned or otherwise exhausted before a healthy release is available, skip it only through a normal
release-eligible commit/PR with a release-please `Release-As` footer for the next allowed version. The footer must survive
the `staging` merge and the later `staging` -> `premain` promotion. Do not use tags, resets, manual release edits,
manifest edits, package-version edits, or reruns of failed exhausted-version workflows to recover.

Current recovery decision: `1.9.2` is abandoned; the next RC must be `v1.9.3-rc.1`; the next stable release must be
`v1.9.3`. The release-eligible recovery commit must carry `Release-As: 1.9.3-rc.1`. See
`docs/development/planning/theorydb-release-cycle-recovery-1.9.3.md`.

## Protections (required)

Protect both `premain` and `main`:

- Require PRs for human-authored changes.
- Allow only the stable release workflow token to fast-forward post-release baseline sync commits to `premain`.
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

Before promoting `premain` to `main`, confirm the intended RC is actually published: it must be non-draft, marked as a
GitHub prerelease, have a `publishedAt` timestamp, resolve through `refs/tags/vX.Y.Z-rc.N`, use a tag-addressed release
URL rather than an `untagged-...` draft URL, and include the required TypeScript/Python assets. Asset presence alone is
not sufficient evidence that a release is healthy.

## Multi-language versioning (required)

- **Single shared repo version:** Go, TypeScript, and Python use the same GitHub tag/release version.
- **No registry publishing:** TypeScript is not published to npm and Python is not published to PyPI; GitHub releases are the source of truth.
- **Release automation must update TypeScript versions:** if `ts/package.json` exists, the prerelease/release workflows must
  update `ts/package.json` and `ts/package-lock.json` to match the repo version.
- **Release automation must update Python versions:** if `py/pyproject.toml` exists, the prerelease/release workflows must
  update `py/src/theorydb_py/version.json` to match the repo version.
- **Stable release automation must normalize prerelease state:** `release-please-config.json` must update
  `.release-please-manifest.premain.json` to the stable version in the generated stable Release PR.

## Required workflow artifacts (Rubric COM-8)

These files are required to exist and be kept current:

- `.github/workflows/prerelease.yml`
- `.github/workflows/release.yml`
- `scripts/verify-release-cycle-state.sh`
- `scripts/watch-release-cycle.sh`
- `scripts/prepare-stable-promotion.sh`

`scripts/prepare-stable-promotion.sh` is retained as a diagnostic/fallback helper. It is not the normal stable promotion
path, and it must not replace release-please-owned stable version/changelog updates.

Additionally, quality/security workflows should run on PRs to (and/or pushes on) both protected branches:

- `.github/workflows/quality-gates.yml`
- `.github/workflows/codeql.yml`

## Notes

- This policy is intentionally tool-agnostic; the rubric requires automation and pinning, not a specific release tool.
- Branch protection rules are configured in the hosting platform (GitHub settings) and must be treated as part of the supply chain.

## Release watchpoints

Run `bash scripts/watch-release-cycle.sh` during a release and rerun with `--strict` before merge/release gates. Pause on:

- `origin/main` stable files containing `-rc` outside explicit pending stable promotion.
- `.release-please-manifest.json` set to an RC version.
- `origin/premain` stable manifest behind `origin/main`.
- `origin/staging` missing the latest stable baseline after a stable release, or carrying RC reconciliation files after
  the active RC lane has been normalized to stable.
- SEC-2/govulncheck still observing Go `1.26.3`.
- COM-8 branch/version sync failing.
- release-please opening a stable PR for an RC version.
- `origin/main` in pending stable promotion without an open stable release-please PR for the computed stable version.
- a release workflow expected to create a release but not reporting `release_created`.
- asset/publish steps missing `tag_name`.
- a GitHub release existing without uploaded TypeScript/Python assets.
- a requested release tag that is draft, missing `publishedAt`, missing `refs/tags/<tag>`, using an `untagged-...` release
  URL, or whose git tag ref target differs from the release target commitish.
- any recovery plan that reuses a deleted/exhausted immutable release version instead of advancing through a
  release-eligible PR and release-please.
- any abandoned-version recovery that lacks a release-eligible commit with the required release-please `Release-As`
  footer, or that edits release manifests/package versions by hand.
- automation attempting direct branch mutation where PR sync is required.
