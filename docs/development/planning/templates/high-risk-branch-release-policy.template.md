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
5. Stable automation publishes immutable `vX.Y.Z`.
6. Sync the stable baseline back to `[integration-branch]` and `[prerelease-branch]` through PRs or documented verified
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
- direct branch pushes or ref mutations where policy requires PR sync.

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
- release asset steps have no tag name, or the GitHub release exists without required assets.
- automation attempts direct branch mutation where the documented path expects PR sync.

Allowed recovery should be non-destructive: new PR branches from known bases, deterministic normalization scripts, and
verified PR-based sync. Do not retag, overwrite release assets, force-push, delete protected branches, or merge around
quality/security checks.
