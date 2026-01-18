# Branch + Release Policy Template (High-Risk Domains)

This template is intended to be copied and filled per project.

## Branches

- `[prerelease-branch]` — prerelease integration branch (source of prereleases).
- `[release-branch]` — release branch (source of stable releases).

## Merge flow (expected)

- Feature/fix work lands via PRs into `[prerelease-branch]`.
- A release is cut by merging `[prerelease-branch]` into `[release-branch]` (via PR).
- Hotfixes may merge directly into `[release-branch]` (then backported to `[prerelease-branch]`).

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

## Required workflow artifacts

- `.github/workflows/prerelease.yml`
- `.github/workflows/release.yml`

## Evidence / verification

- Link this policy from the project’s rubric (as an artifact check) and add a verifier that fails if:
  - the workflows don’t exist,
  - tools are unpinned (`@latest`),
  - releases are not gated on the project’s quality/security surface.
