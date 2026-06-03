# Branch + Release Policy Template (High-Risk Domains)

This template is intended to be copied and filled per project.

## Branches

- `[prerelease-branch]` — prerelease integration branch (source of prereleases).
- `[release-branch]` — release branch (source of stable releases).

Define exactly what each branch owns. For a three-branch model, document:

- `[integration-branch]` owns normal work and the latest stable baseline after release.
- `[prerelease-branch]` owns RC state and may carry `X.Y.Z-rc.N` in prerelease manifests and SDK/package files.
- `[release-branch]` owns stable state only; stable manifests and SDK/package files must not contain `-rc`.

## Merge flow (expected)

- Feature/fix work lands via PRs into `[prerelease-branch]`.
- A release is cut by merging `[prerelease-branch]` into `[release-branch]` (via PR).
- Hotfixes may merge directly into `[release-branch]` (then backported to `[prerelease-branch]`).

If the project uses separate integration, prerelease, and stable branches, prefer this stable-promotion flow:

1. Work lands on `[integration-branch]`.
2. `[integration-branch]` -> `[prerelease-branch]` PR starts the prerelease lane.
3. Prerelease automation produces `vX.Y.Z-rc.N` on `[prerelease-branch]`.
4. Create a promotion branch from `[release-branch]`, merge `[prerelease-branch]` into it, and normalize RC-owned files
   to stable state before opening the PR to `[release-branch]`.
5. After the normalized promotion PR merges, release-PR automation opens the stable release PR while release automation
   skips publishing during the temporary pending stable promotion state.
6. Merging the stable release PR restores strict equality and stable automation publishes immutable `vX.Y.Z`.
7. Sync the stable baseline back to `[integration-branch]` and `[prerelease-branch]` through PRs or documented verified
   automation.

## Protections (required)

Protect both `[prerelease-branch]` and `[release-branch]`:

- Require PRs (no direct pushes).
- Require review approvals (CODEOWNERS recommended).
- Require CI status checks to pass (define the minimum required checks explicitly).
- Restrict force-pushes and deletions.

## Automated releases (required)

Define and automate:

- **Prereleases** on merges to `[prerelease-branch]` (tag convention: `[vX.Y.Z-rc.N]` or similar).
- **Releases** on merges to `[release-branch]` (tag convention: `vX.Y.Z`).

Implementation options (pick one and pin versions):

- **release-please** (merge-driven versioning + changelog updates)
- **goreleaser** (tag-driven releases + artifact builds)

Document forbidden stable-branch states:

- stable manifest set to `X.Y.Z-rc.N`.
- language/package version files left at `X.Y.Z-rc.N`.
- release automation opening a stable release PR for an RC version.
- pending stable promotion persisting after the normalized promotion commit without an open stable release PR.
- direct branch pushes or ref mutations where policy requires PR sync.
- manual recovery that recreates deleted tags, hand-publishes replacement releases, or reuses an exhausted immutable
  release version.

Document immutable release version reuse explicitly. Treat published release tag names as one-time-use even if the release
or tag is later deleted. If a publish step fails with `tag_name was used by an immutable release`, recovery must go through
a normal release-eligible PR and the release tool must advance to the next RC/stable version for that lane.

If release-please or another release-PR tool leaves the stable manifest at the prior stable version until the stable
release PR merges, document the state as explicit pending stable promotion. The pending verifier mode must be visible in
the workflow, limited to the release branch, require normalized stable package files to be internally consistent and ahead
of the current stable baseline, and must not publish a stable release. Once the stable release PR merges,
strict stable equality is required again. If the pending state persists because no stable release PR opens, pause and
investigate; do not patch the release branch by hand.

## Required workflow artifacts

- `.github/workflows/prerelease.yml`
- `.github/workflows/release.yml`

## Evidence / verification

- Link this policy from the project’s rubric (as an artifact check) and add a verifier that fails if:
  - the workflows don’t exist,
  - tools are unpinned (`@latest`),
  - releases are not gated on the project’s quality/security surface.

Evidence should include deterministic branch/version checks and a read-only release watch command.

## Watchpoints / stop conditions

Pause before merge or release when:

- stable-branch files contain `-rc`.
- prerelease stable baseline is behind the release branch.
- integration branch lacks the latest stable baseline after a stable release.
- security/governance checks still observe a vulnerable toolchain or dependency state.
- branch/version sync checks fail.
- release automation completes without creating the expected release.
- pending stable promotion persists without an open stable release PR.
- release asset steps have no tag name, or the GitHub release exists without required assets.
- a requested release tag is draft, lacks a published timestamp, has no git tag ref, uses an `untagged-...` draft URL, or
  points the release target and git tag ref at different commits.
- the intended RC is not yet published, non-draft, marked prerelease, tagged, and asset-complete.
- automation attempts direct branch mutation where the documented path expects PR sync.

Allowed recovery should be non-destructive: new PR branches from known bases, deterministic normalization scripts, and
verified PR-based sync. Do not retag, overwrite release assets, force-push, delete protected branches, hand-publish
replacement releases, reuse exhausted immutable release versions, or merge around quality/security checks.
