# TableTheory: Branch + Release Policy (main release, premain prerelease)

This document defines the intended branch strategy and release automation for TableTheory in high-risk usage contexts.
TableTheory has one release lane: `staging` -> `premain` -> `main` -> `staging` (staging -> premain -> main -> staging).
`premain` owns the RC phase of that lane, and `main` owns the stable phase; neither branch starts a separate release path.

## Branches

- `staging` — integration branch (all work lands here first).
- `premain` — prerelease branch (source of prereleases).
- `main` — release branch (source of stable releases).

## Ownership

- `staging` owns integration-ready code, docs, security/toolchain updates, and release-cycle guardrails before they enter
  the RC or stable phases. After a stable release, `staging` must receive the latest stable baseline from `main` through a
  normal PR backmerge.
- `premain` owns prerelease state. Release-please runs in prerelease mode on `premain` and writes RC versions to the
  single manifest `.release-please-manifest.json`.
- `main` owns stable releases. Outside the short, explicit single-manifest pending stable-promotion state immediately
  after a `premain` -> `main` promotion, `.release-please-manifest.json` must be stable on `main`.
- `ts/package.json`, `ts/package-lock.json`, and `py/src/tabletheory_py/version.json` are package metadata, not release-cycle
  state. Release workflows stamp them from `tag_name` in the release-build workspace before packaging; release automation
  must not commit RC churn to those files solely to publish assets.

## Merge flow (expected)

- Feature/fix work lands via PRs into `staging`.
- An **RC** is cut by merging `staging` into `premain` (via PR), then merging the release-please prerelease PR.
- A `staging` -> `premain` PR is a release-intent gate, not an optional sync. It must include a release-eligible
  conventional commit or RC-shaped `Release-As:` footer so release-please can open the generated RC PR. If release-please
  would report "No user facing commits", the gate is failing and the fix must happen through normal `staging` PR content
  or the promotion PR squash title/body/footer.
- A **stable release** is prepared by verifying the intended RC release, then opening and merging the `premain` -> `main`
  promotion PR. The promoted single manifest may briefly be RC-shaped on `main`; `release.yml` must skip publication in
  that state and `release-pr.yml` must open the generated stable release-please PR with `release-as` computed from the RC
  base.
- A `premain` -> `main` PR is valid only when the single manifest carries a numbered RC that can become stable. The
  follow-up generated release-please PR targeting `main` must be stable-shaped; RC-shaped main release PRs and releases
  are forbidden.
- A **release** is cut by merging the stable release-please PR, which normalizes `.release-please-manifest.json` and
  `CHANGELOG.md` to stable state before the stable release workflow publishes `vX.Y.Z`.
- After the stable release publishes, the next operator step is a normal PR backmerge from `main` to `staging`; `premain`
  receives the new baseline through the next `staging` -> `premain` promotion.
- During an active RC, a premain-derived repair PR back to `staging` is allowed only when `premain` already contains
  `staging` and RC release content must be reconciled to avoid forward merge conflicts. This is an exceptional in-flight
  RC repair state, not the normal release hop; ordinary RC manifest state on `staging` remains forbidden.
- Hotfixes should still follow `staging` -> `premain` -> `main` so the release lane stays linear.

## Post-release backmerge (operator PR)

After a stable release is published on `main`, CI must not push baseline sync commits to `premain` or `staging`. The
post-release lane step is a normal PR backmerge from `main` to `staging` so the next cycle starts from the released
stable state. Do not direct-push from CI to `premain` or `staging`, do not run a workflow that mutates protected branch
refs through the GitHub API, and do not sync `main` directly to `premain` after stable publication.

The normal stable promotion path does not use a local stable-normalization branch. Release-please owns manifest and
changelog updates; the TypeScript/Python release asset versions are generated from the release tag in the release-build
workspace and verified inside the tarball/wheel/sdist before upload.

No post-stable sync helper or local stable-normalization helper remains in the release lane. Recovery remains PR-based:
if a branch is stranded, create a new PR branch from the correct base and replay only the needed file state. Do not retag,
overwrite release assets, force-push, delete branches, mutate GitHub releases, or direct-push protected branches.

## Immutable release version reuse

GitHub release tag names are one-time-use for TableTheory release-cycle purposes. If a published immutable release or its
git tag is deleted, that version name is still exhausted and must not be reused. Do not manually recreate the tag, publish
a replacement release by hand, edit immutable release assets, or patch manifests to make the same version run again.

If a publish step fails with `tag_name was used by an immutable release`, recover through the normal PR-driven release
flow: land a release-eligible change, promote it through the documented branch path, and let release-please advance to the
next RC or stable version for the single release lane.

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
- Require CODEOWNERS/review approvals.
- Require release-hygiene status checks for PRs targeting `premain` and `main`; do not require the full gov-infra rubric
  on those promotion branches.
- Require the GitHub merge queue. The queue must require the `Release Hygiene` workflow's `merge_group` validation before
  a promotion PR or generated release-please PR can merge.
- Restrict force-pushes and deletions.

Protect `staging` with the full gov-infra rubric on PRs targeting `staging`. The full rubric may also run by
`workflow_dispatch`, but it must not run on push or on PRs targeting `premain` or `main`. Require the GitHub merge queue
for `staging` too, with the `Quality Gates (10/10 Rubric)` `merge_group` validation as the queue check.

### Merge queue provenance disposition

Release-lane v2 keeps the provenance checks that identify *what* is allowed to enter a protected release branch:

- PR base/head repositories must be `theory-cloud/TableTheory`; forks and name-spoofed repositories are rejected before
  any PR-head checkout.
- `premain` accepts only `staging` promotions or generated `release-please--branches--premain` RC PRs, with numbered
  `X.Y.Z-rc.N` release titles.
- `main` accepts only `premain` promotions or generated `release-please--branches--main` stable PRs, with no RC-shaped
  release title.
- Human promotion PRs still run the release-driver guard, so release-please "No user facing commits" remains a failed
  release-intent gate.

The old required PR-time "event base/head SHA must equal the current live branch ref" guard is retired from the normal
release-hygiene PR path because the merge queue now validates the exact queued merge ref against the latest protected
branch tip before merge. The strict live-ref check remains in `scripts/verify-release-lane-provenance.sh` for documented
manual-freeze fallback use if merge queue is unavailable.

## Automated releases (required)

This repo should publish:

- **Prereleases** on merges to `premain`.
- **Releases** on merges to `main`.

Recommended approach: **release-please** (merge-driven versioning + changelog updates) with:

- prerelease workflow producing tags like `vX.Y.Z-rc.N`, and
- release workflow producing stable `vX.Y.Z` tags and updating `CHANGELOG.md`.

### Release triggers (required)

`release-please` only cuts a new rc/release when there is at least one **release-eligible** (user-facing) commit since the previous tag. As a result:

- **Dependency/platform updates must use a release-eligible conventional commit type** (recommended: `fix(deps): ...`) so they produce an rc/release.
- Pure `chore(...)` commits may be treated as non-user-facing and can be skipped by `release-please`.
- On `premain` and `main` gates, a release-please "No user facing commits" result is a failed gate precondition, not a
  successful no-op. Correct remediation is a release-eligible conventional commit or the appropriate `Release-As:` footer
  through normal PR flow. Do not create tags, reset branches, force-push, direct-push protected branches, hand-edit
  manifests/package versions, or mutate GitHub releases to force a release.

**Recommendation:** use squash-merge and set the squash title to a conventional commit that matches the intended version bump:

- Patch: `fix(deps): update multi-language dependencies`
- Minor: `feat: ...`
- Major: `feat!: ...` or include `BREAKING CHANGE:` in the body

### Release assets (required)

GitHub Releases must attach build artifacts for the non-Go SDKs:

- **TypeScript:** `npm pack` output from `ts/` (tarball)
- **Python:** wheel + sdist from `py/` (`python -m build`)

Release workflows must derive the package version from the release `tag_name`, stamp `ts/package.json`,
`ts/package-lock.json`, and `py/src/tabletheory_py/version.json` in the workflow workspace, build assets, and verify the
packed TypeScript and Python metadata matches the tag before upload. These workspace edits are not committed.

If a release workflow was expected to publish but `release_created` is false, pause before trying to patch files by hand.
If an asset upload or publish step has no `tag_name`, fail the workflow. If a GitHub release exists but TypeScript/Python
assets are missing, use a documented asset recovery workflow only after confirming the tag and release are immutable and
correct.

Generated release-please PR merges are the publish steps. If a generated RC PR merge on `premain` or generated stable PR
merge on `main` reports `release_created=false`, fail loudly; do not treat the publish workflow as green. Plain
`staging` -> `premain` and `premain` -> `main` promotion merges are PR-generation setup only when `prerelease-pr.yml` or
`release-pr.yml` is responsible for and required to open the generated release-please PR.

Before promoting `premain` to `main`, confirm the intended RC is actually published: it must be non-draft, marked as a
GitHub prerelease, have a `publishedAt` timestamp, resolve through `refs/tags/vX.Y.Z-rc.N`, use a tag-addressed release
URL rather than an `untagged-...` draft URL, and include the required TypeScript/Python assets. Asset presence alone is
not sufficient evidence that a release is healthy.

## Multi-language versioning (required)

- **Single shared repo version:** Go, TypeScript, and Python use the same GitHub tag/release version.
- **Single release-please manifest:** `.release-please-manifest.json` is the only release-please manifest. `premain` may
  carry `X.Y.Z-rc.N`; `main` must normalize to stable through the generated stable release-please PR.
- **No registry publishing:** TypeScript is not published to npm and Python is not published to PyPI; GitHub releases are
  the source of truth.
- **Tag-derived package versions:** release workflows, not release-please manifests, stamp TypeScript/Python package
  metadata from `tag_name` immediately before building assets.

## Required workflow artifacts (Rubric COM-8)

These files are required to exist and be kept current:

- `.github/workflows/prerelease.yml`
- `.github/workflows/release.yml`
- `scripts/verify-release-cycle-state.sh`
- `scripts/watch-release-cycle.sh`
- `scripts/prepare-release-package-versions.py`
- `scripts/verify-release-package-version-assets.py`

Release-lane quality workflow expectations:

- `.github/workflows/quality-gates.yml` runs the full gov-infra rubric only for PRs targeting `staging`, queued
  `staging` merge groups, and manual dispatch.
- `.github/workflows/release-hygiene.yml` runs lightweight source-branch/provenance, release-cycle, supply-chain, and
  main-RC-PR checks for PRs targeting `premain`/`main`, plus queued `premain`/`main` merge groups.
- Other security workflows, such as `.github/workflows/codeql.yml`, may run independently, but they do not replace the
  release-lane gate split above.

## Notes

- This policy is intentionally tool-agnostic; the rubric requires automation and pinning, not a specific release tool.
- Branch protection rules are configured in the hosting platform (GitHub settings) and must be treated as part of the supply chain.

## Release watchpoints

Run `bash scripts/watch-release-cycle.sh` during a release and rerun with `--strict` before merge/release gates. Pause on:

- `origin/main` carrying an RC manifest outside the explicit single-manifest pending stable-promotion window.
- `.release-please-manifest.json` set to an RC version on `staging`, unless it is a verified in-flight premain RC repair
  where `premain` already contains `staging` and the RC release content is being reconciled to avoid forward merge
  conflicts.
- the retired `.release-please-manifest.premain.json` appearing on release branches.
- `origin/premain` release track behind `origin/main`.
- `origin/staging` missing the latest stable baseline after a stable release.
- SEC-2/govulncheck still observing Go `1.26.3`.
- COM-8 branch/version sync failing.
- release-please opening a stable PR for an RC version.
- `origin/main` in pending stable promotion without an open stable release-please PR for the computed stable version.
- `prerelease-pr.yml` completing after a `staging` -> `premain` promotion without an open generated RC release-please PR
  targeting `premain`.
- release-please reporting "No user facing commits" on a `premain` or `main` release-intent gate.
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
