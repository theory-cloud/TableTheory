# Branch + Release Policy Template (High-Risk Domains)

This template is intended to be copied and filled per project.
Use one release lane with an RC phase followed by a stable phase. Branches can separate integration, RC generation, and
stable publishing duties, but they must not create a second release path for the same product.

Factory-standard three-branch framework lane: `staging -> premain -> main -> staging`.

## Branches

- `[prerelease-branch]` — prerelease integration branch (source of prereleases).
- `[release-branch]` — release branch (source of stable releases).

Define exactly what each branch owns. For a three-branch model, document:

- `[integration-branch]` owns normal work and the latest stable baseline after release. During active RC reconciliation,
  it must not keep obsolete RC package-version churn after the stable release publishes.
- `[prerelease-branch]` owns RC state and may carry `X.Y.Z-rc.N` in the release manifest.
- `[release-branch]` owns stable state only; outside explicit pending stable promotion, the release manifest must not
  contain `-rc`.

## Merge flow (expected)

- Feature/fix work lands via PRs into `[prerelease-branch]`.
- A release is cut by merging `[prerelease-branch]` into `[release-branch]` (via PR).
- Hotfixes may merge directly into `[release-branch]` (then backported to `[prerelease-branch]`).

If the project uses separate integration, prerelease, and stable branches, prefer this stable-promotion flow:

1. Work lands on `[integration-branch]`.
2. `[integration-branch]` -> `[prerelease-branch]` PR starts the RC phase.
3. Prerelease automation opens a generated RC release PR, then publishes `vX.Y.Z-rc.N` only after that generated PR
   merges.
4. Verify the intended RC is published and asset-complete, then open and merge the `[prerelease-branch]` ->
   `[release-branch]` promotion PR.
5. Release automation skips publishing during the temporary pending stable promotion state while release-PR automation
   opens the stable release PR with the stable `release-as` derived from the RC baseline.
6. Merging the stable release PR normalizes the release manifest and changelog to the stable version; stable automation
   then publishes immutable `vX.Y.Z` and stamps SDK/package asset versions from the tag in the release-build workspace.
7. Backmerge the stable baseline from `[release-branch]` to `[integration-branch]` through a normal PR. Do not direct-sync
   `[release-branch]` to `[prerelease-branch]`; the next `[integration-branch]` -> `[prerelease-branch]` promotion carries
   the stable baseline forward.

## Protections (required)

Protect both `[prerelease-branch]` and `[release-branch]`:

- Require PRs (no direct pushes).
- Require review approvals (CODEOWNERS recommended).
- Require lightweight release-hygiene checks that validate source branches, release-cycle state, branch release
  supply-chain scaffolding, human promotion release drivers, generated release-PR shape, and stable-branch no-RC release
  PRs.
- Restrict force-pushes and deletions.

Protect `[integration-branch]` with the full project rubric only on PRs targeting `[integration-branch]` and optional
manual dispatch. Do not require or run the full rubric on PRs targeting `[prerelease-branch]` or `[release-branch]`.

## Automated releases (required)

Define and automate:

- **Prereleases** on merges to `[prerelease-branch]` (tag convention: `[vX.Y.Z-rc.N]` or similar).
- **Releases** on merges to `[release-branch]` (tag convention: `vX.Y.Z`).

PRs to `[prerelease-branch]` and `[release-branch]` are release-intent gates, not optional/no-op syncs. A promotion into
the prerelease branch must carry a release-eligible conventional commit or prerelease-shaped explicit version footer that
the release tool can use to open the generated RC PR. A promotion into the release branch must carry a pending RC
promotion state that can become stable. If release-please or another release-PR tool reports "No user facing commits" on
these gates, treat that as a failed precondition, not a green no-op.

Correct remediation is a release-eligible conventional commit or explicit version footer through normal PR flow. Do not
create tags, reset branches, force-push, direct-push protected branches, hand-edit manifests/package versions, or mutate
GitHub releases to force a release.

Implementation options (pick one and pin versions):

- **release-please** (merge-driven versioning + changelog updates)
- **goreleaser** (tag-driven releases + artifact builds)

Document forbidden stable-branch states:

- release manifest set to `X.Y.Z-rc.N` outside explicit pending stable promotion.
- language/package release assets whose metadata does not match the release tag.
- release automation opening a stable release PR for an RC version.
- prerelease PR automation completing without an open generated RC release PR after a prerelease promotion.
- pending stable promotion persisting after the release-branch promotion without an open stable release PR.
- publish automation seeing `release_created=false` or a missing tag name after a generated RC/stable release PR merge.
- direct branch pushes or ref mutations where policy requires PR sync.
- automated post-stable direct-push sync to `[prerelease-branch]` or `[integration-branch]`.
- manual recovery that recreates deleted tags, hand-publishes replacement releases, or reuses an exhausted immutable
  release version.

Document immutable release version reuse explicitly. Treat published release tag names as one-time-use even if the release
or tag is later deleted. If a publish step fails with `tag_name was used by an immutable release`, recovery must go through
a normal release-eligible PR and the release tool must advance to the next RC/stable version for the single release lane.

Document abandoned or exhausted version recovery explicitly. A skipped immutable version must advance only through a normal
release-eligible commit/PR with the release tool's explicit version override footer, for example
`Release-As: [next-version-or-rc]` when using release-please. The footer must survive the integration -> prerelease merge
path. Do not recover by creating tags, rerunning failed exhausted-version workflows, editing immutable releases, patching
manifests, or hand-editing package-version files.

If release-please or another release-PR tool leaves the release branch at a promoted RC state until the stable release PR
merges, document the state as explicit pending stable promotion. The pending verifier mode must be visible in the
workflow, limited to the release branch, require the generated stable release PR to normalize the release manifest and
changelog to the stable version, and must not publish a stable release. Once the stable release PR merges, strict stable
state is required again. If the pending state persists because no stable release PR opens, pause and investigate; do not
patch the release branch by hand.

If the integration branch must merge the prerelease branch during active recovery to surface later promotion conflicts,
document that as bounded RC reconciliation. The verifier should accept it only when the single release manifest remains
coherent with the intended branch role and no retired prerelease manifest or committed RC package-version churn is
reintroduced.

## Required workflow artifacts

- `.github/workflows/prerelease.yml`
- `.github/workflows/release.yml`
- `.github/workflows/quality-gates.yml` for integration-branch full rubric only
- `.github/workflows/release-hygiene.yml` for prerelease/release branch PR hygiene

## Evidence / verification

- Link this policy from the project’s rubric (as an artifact check) and add a verifier that fails if:
  - the workflows don’t exist,
  - tools are unpinned (`@latest`),
  - the full rubric is not limited to integration-branch PRs and manual dispatch,
  - prerelease/release branch PRs do not have lightweight release hygiene.

Evidence should include deterministic branch/version checks and a read-only release watch command.

## Watchpoints / stop conditions

Pause before merge or release when:

- stable-branch release state contains `-rc` outside explicit pending stable promotion.
- prerelease stable baseline is behind the release branch.
- integration branch lacks the latest stable baseline after a stable release, or keeps RC reconciliation state after the
  active RC phase has been normalized to stable.
- security/governance checks still observe a vulnerable toolchain or dependency state.
- branch/version sync checks fail.
- release-please or the release-PR tool reports "No user facing commits" on a prerelease/release branch gate.
- prerelease PR automation completes without an open generated RC release PR.
- an abandoned/exhausted version recovery lacks the required release-eligible commit footer or tries to skip the release
  tool with manual version/tag/release edits.
- release automation completes without creating the expected release, including `release_created=false` after a generated
  RC/stable release PR merge.
- pending stable promotion persists without an open stable release PR.
- release asset steps have no tag name, or the GitHub release exists without required assets.
- a requested release tag is draft, lacks a published timestamp, has no git tag ref, uses an `untagged-...` draft URL, or
  points the release target and git tag ref at different commits.
- the intended RC is not yet published, non-draft, marked prerelease, tagged, and asset-complete.
- automation attempts direct branch mutation where the documented path expects PR sync, including post-stable baseline
  sync pushes.

Allowed recovery should be non-destructive: new PR branches from known bases and verified PR-based sync. The normal
release path should leave version and changelog edits to release automation. Do not
retag, overwrite release assets, force-push, delete protected branches, hand-publish replacement releases, reuse exhausted
immutable release versions, or merge around quality/security checks.
