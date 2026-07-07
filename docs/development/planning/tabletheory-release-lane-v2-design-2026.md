# TableTheory release-lane v2 design and implementation record

Status: **design record with implementation follow-ups**. This document was authored for TTIP-110 / THE-2448. TTIP-111 /
THE-2449 implemented the single-manifest lane in repo-owned workflows and guards; TTIP-112 / THE-2450 implemented the
merge-queue workflow triggers and provenance-guard disposition below. TTIP-113 retires the helper scripts that the v2
lane no longer uses; live RC-to-stable proof remains a release-readiness receipt, not a prerequisite for removing those
orphaned helpers from source.

## Purpose

At the time of THE-2448, the release lane was safe but too stateful: release-please tracked stable and RC state through
two manifests, committed TypeScript/Python version files moved during RC reconciliation, and several guards existed only
to prevent those state files from drifting. Release-lane v2 keeps the single branch lane
(`staging -> premain -> main -> staging`) and the immutable GitHub Releases distribution model, but reduces the mutable
release state that can diverge across branches.

The v2 target is:

1. **One release-please manifest**: `.release-please-manifest.json` is the only release-please manifest. The prerelease
   manifest `.release-please-manifest.premain.json` is retired after a between-cycles migration.
2. **Native release-please prerelease handling**: `premain` continues to produce `vX.Y.Z-rc.N` through release-please's
   prerelease mode (`versioning: prerelease`, `prerelease-type: rc`, `prerelease: true`) without maintaining a second
   manifest file.
3. **Tag-driven runtime version source**: the Git tag (`vX.Y.Z` or `vX.Y.Z-rc.N`) is the canonical released version for
   Go, TypeScript, and Python.
4. **Generated TS/Py package versions at release-build time**: TypeScript and Python release assets receive their version
   from the release tag while packaging. The source tree no longer commits RC churn into `ts/package.json`,
   `ts/package-lock.json`, or `py/src/tabletheory_py/version.json` solely to publish an asset.
5. **Merge queue on protected release branches**: promotion PRs and release-please PRs merge only after queue validation
   against the exact branch tip that will receive them.
6. **Smaller guard surface**: guards that exist only for the dual-manifest/committed-SDK-version model are merged or
   retired; guards that enforce immutable releases, provenance, release intent, and asset completeness stay.

## Non-goals

- Do not change the distribution model: GitHub Releases remain the only package distribution surface; no npm or PyPI
  publishing is introduced.
- Do not bypass release-please for normal version/changelog ownership.
- Do not alter the stable public API or runtime contract behavior.
- Do not change branch protection or GitHub repository settings as part of this preparatory milestone.
- Historical THE-2448 constraint: later implementation and cleanup items required maintainer sign-off before execution.

## Proposed v2 lane

### Branch ownership

- `staging`: integration branch; never owns release state beyond code/docs/scripts ready for the next release.
- `premain`: prerelease branch; release-please runs in native prerelease mode and writes RC tags/releases from the single
  manifest state on that branch.
- `main`: stable branch; release-please runs stable mode and writes stable tags/releases from the same manifest path on
  that branch.
- `main -> staging`: still a normal PR backmerge after a stable release so the next cycle starts from the released
  baseline.

### Single manifest behavior

`release-please-config.premain.json` may remain as a prerelease config file during the v2 migration, but both premain and
main use `.release-please-manifest.json` as their manifest path. The manifest can legitimately contain an RC version on
`premain` and a stable version on `main`; branch role, not filename, defines whether the version is prerelease or stable.

Stable promotion no longer needs a mixed pending state where one manifest is stable while a second prerelease manifest
and SDK files are still RC-shaped. The remaining pending state is explicit and short-lived: after `premain` promotes to
`main`, `.release-please-manifest.json` may briefly carry the RC until `release-pr.yml` opens the generated stable
release-please PR. That stable PR normalizes the single manifest and changelog only.

### Tag-driven runtime versions

Go already uses tags as the version source. In v2, TS/Py packaging follows the same principle:

- Release workflows derive `RELEASE_VERSION` from the release tag emitted by release-please (`tag_name`, stripped of the
  leading `v` for package metadata).
- Before `npm pack`, the workflow writes that version into a temporary release-build copy of `ts/package.json` and the
  root entries of `ts/package-lock.json`, then packs from that prepared copy or restores the workspace before upload.
- Before `python -m build`, the workflow writes that version into a generated `py/src/tabletheory_py/version.json` in the
  release-build workspace, then builds the wheel/sdist.
- The generated files are release-build artifacts, not source-of-truth release-cycle state. They must not be committed by
  release workflows.

The implementation item must prove that the generated package metadata inside the uploaded TypeScript tarball and Python
wheel/sdist matches the GitHub release tag.

### Merge queue adoption

Release-lane v2 assumes GitHub merge queue is enabled for protected `staging`, `premain`, and `main` branches before the
implementation PRs merge:

- `staging` queue requires the full gov-infra rubric / quality gates.
- `premain` and `main` queues require release-hygiene checks, provenance checks, branch/release supply-chain checks, and
  postcondition checks appropriate to the target branch.
- Promotion PRs (`staging -> premain`, `premain -> main`) must enter the queue like any other protected-branch PR; no
  direct merge or admin bypass is part of v2.
- Generated release-please PRs must also pass through the queue so their release tag/release workflow observes the same
  branch tip that was checked.

If merge queue is unavailable, the documented manual-freeze mode is: one release-lane PR open at a time, no
parallel promotion/release-please merges, and a fresh strict release-lane verification immediately before merge.

THE-2450 repo-owned implementation:

- `.github/workflows/quality-gates.yml` has a `merge_group` trigger for queued `staging` merges.
- `.github/workflows/release-hygiene.yml` has a `merge_group` trigger for queued `premain` and `main` merges and validates
  the queued ref with release-cycle and supply-chain checks. On queued `main` merge groups, it accepts the same explicit
  pending-stable-promotion state that a `premain` -> `main` promotion PR creates.
- Pull-request provenance still rejects forks/name-spoofing and illegal release-lane source branches before checking out
  PR-head code. The prior live-ref equality check is not required in the normal PR path because the merge queue validates
  freshness against the protected branch tip; it remains available as the strict manual-freeze check.
- Live GitHub branch-protection/ruleset changes are operator-owned and cannot be completed through a normal signed PR.

## Guard disposition map

| Current guard / artifact | Current purpose | v2 disposition | Successor / rationale |
| --- | --- | --- | --- |
| `.github/workflows/quality-gates.yml` | Full rubric on PRs targeting `staging` | **Keep** | Still the integration branch quality gate. Add merge-queue requirement outside repo code. |
| `.github/workflows/release-hygiene.yml` | Lightweight trusted release checks for `premain`/`main` PRs | **Keep, update** | Continue provenance and release-hygiene checks; remove checks that only mention the retired prerelease manifest. |
| `.github/workflows/prerelease-pr.yml` | Opens generated RC release-please PRs on `premain` | **Keep, update** | Use single manifest path and native prerelease config; paths-ignore no longer includes SDK version files as release state. |
| `.github/workflows/prerelease.yml` | Publishes prerelease GitHub Releases and assets | **Keep, update** | Use single manifest path; generate TS/Py versions from `tag_name` before packaging. |
| `.github/workflows/release-pr.yml` | Opens deterministic stable Release PRs on `main` after pending promotion | **Keep, update** | Compute the stable version from the single-manifest RC baseline; keep stable PR postcondition checks. |
| `.github/workflows/release.yml` | Publishes stable GitHub Releases and assets | **Keep, update** | Skip publish only during explicit single-manifest pending stable promotion; generate TS/Py versions from `tag_name` before packaging. |
| `release-please-config.json` | Stable release-please config plus extra-files normalization | **Keep, simplify** | Remove `.release-please-manifest.premain.json` and SDK version-file extra-files once generated package versions are proven. |
| `release-please-config.premain.json` | Prerelease release-please config | **Keep initially, possibly merge later** | It may remain branch-specific config while using the single manifest; evaluate config merge only after native prerelease dry-run. |
| `.release-please-manifest.json` | Stable manifest today | **Keep as sole manifest** | Branch-local value becomes stable on `main`, RC on `premain`. |
| `.release-please-manifest.premain.json` | Prerelease manifest today | **Retire** | Its role is replaced by native prerelease handling against the single manifest. |
| `ts/package.json` version | Committed TS package version state | **Retire as release-cycle state** | Keep package metadata file, but release workflow patches version from tag in the build workspace. |
| `ts/package-lock.json` root versions | Committed TS package-lock version state | **Retire as release-cycle state** | Release workflow patches/generated lock metadata from tag for the packed artifact. |
| `py/src/tabletheory_py/version.json` | Committed Python package version state | **Retire as release-cycle state** | Generate from tag during release build; keep runtime import behavior stable in artifacts. |
| `scripts/verify-branch-release-supply-chain.sh` | Source-validates release-lane scaffolding | **Keep, update** | Becomes the source validator for single-manifest invariants, generated version scripts, and merge-queue workflow expectations. |
| `scripts/verify-branch-version-sync.sh` | Branch/version sync and SDK alignment checks | **Merge/simplify** | Drop dual-manifest and committed SDK version alignment checks; keep branch role sanity and tag-derived asset proof hooks. |
| `scripts/verify-release-cycle-state.sh` | Local checked-out release-cycle state check | **Keep, simplify** | Enforce one manifest, no RC on `main`, no stale generated source state, no protected-branch mutation. |
| `scripts/watch-release-cycle.sh` | Read-only release branch/release/tag watchpoints | **Keep, update** | Watch single manifest per branch and release assets; remove dual-manifest drift warnings. |
| `scripts/lib/release-cycle-core.sh` | Shared helper core added in TTIP-109 | **Keep, update** | Central place for branch/version classification used by local and watch verifiers. |
| `scripts/verify-release-lane-provenance.sh` | Same-repository/source SHA guard for release-lane PRs | **Keep** | Still protects release-lane PR provenance independent of manifest model. |
| `scripts/verify-promotion-release-driver.sh` | Ensures promotion PRs carry release intent / Release-As | **Keep, update** | Still required; update stable promotion logic to the one-manifest model. |
| `scripts/verify-prerelease-pr-postcondition.sh` | Ensures generated RC PR exists and is RC-shaped | **Keep, update** | Must validate native prerelease PR shape and single manifest diff. |
| `scripts/verify-main-release-pr-postcondition.sh` | Ensures stable PR shape and forbids RC main release PRs | **Keep, update** | Must validate single manifest stable diff and no SDK version-source commits. |
| `scripts/verify-release-created-postcondition.sh` | Fails publish workflows that should have created a release | **Keep** | Immutable release/publish postcondition remains load-bearing. |
| `scripts/prepare-stable-promotion.sh` | Retired stable-promotion helper | **Retired by TTIP-113** | Stable normalization is owned by `release-pr.yml` and the generated deterministic stable Release PR. |
| `scripts/sync-post-stable-release-baselines.sh` | Deprecated dry-run helper for old baseline sync | **Retired by TTIP-113** | V2 keeps normal `main -> staging` PR backmerge; no direct sync helper remains. |
| `scripts/test-release-hygiene-policy.sh` | Policy tests for provenance/promotion/publish guards | **Keep, update** | Add single-manifest promotion and generated-version assertions. |
| `scripts/test-release-pr-tool-policy.sh` | Ensures deterministic stable Release PR generation stays on the blessed path | **Keep, update** | Protects the privileged release-pr token path from reintroducing ad-hoc release-please CLI execution. |
| `scripts/test-release-verifier-policy.sh` | Fixture matrix for release verifier PASS/WARN/FAIL judgments | **Keep, update** | Extend fixtures to one-manifest good/bad states before changing guards. |
| `scripts/verify-cli-release-assets.sh` | Dry-run proof for CLI asset matrix | **Keep** | Independent of manifest model; still proves Go CLI assets build. |
| `gov-infra/verifiers/gov-verify-rubric.sh` COM-8 | Rubric release supply-chain gate | **Keep, update call graph only** | COM-8 remains the release-lane gate identity. |
| `AGENTS.md` release policy | Operator/steward instructions | **Keep, update** | Must describe v2 after implementation; this design is not enough to change the live policy yet. |
| `docs/development/planning/theorydb-branch-release-policy.md` | Current operator release policy | **Keep, update after sign-off** | Update only with the implementation PR so docs and automation change together. |

THE-2450 guard disposition details:

| Guard / threat | THE-2450 disposition | Rationale / successor |
| --- | --- | --- |
| Fork or name-spoofed release-lane PR checks out untrusted code | **Retained** | `release-hygiene.yml` still runs `verify-release-lane-provenance.sh` from the trusted base checkout before PR-head checkout. |
| Illegal `premain`/`main` source branch or RC/stable release-please title shape | **Retained** | `verify-release-lane-provenance.sh` still allows only `staging`/generated RC heads for `premain` and `premain`/generated stable heads for `main`. |
| Human promotion lacks release intent / lets release-please report "No user facing commits" | **Retained** | `verify-promotion-release-driver.sh` still runs on `staging` -> `premain` and `premain` -> `main` PRs. |
| PR-event base/head SHA must equal current live branch refs | **Covered by queue semantics in normal path; retained for explicit manual-freeze checks** | The protected branch merge queue validates the queued merge ref against the target branch tip before merge. The strict live-ref check remains the default in `verify-release-lane-provenance.sh` for manual-freeze checks; the workflow uses `--queue-freshness`. |
| Release-cycle state, supply-chain scaffolding, main RC release-PR postcondition | **Retained and extended to queue refs** | `release-hygiene.yml` runs these checks on both PR-head checkouts and `merge_group` queue refs. |
| Stable-promotion single-manifest RC on queued `main` merge group | **Replaced by kept pending-promotion check** | Queue validation first tries strict stable state, then accepts only explicit pending stable promotion mode for queued `main` refs. |

## Dry-run protocol for TTIP-111

Before changing live release workflows, TTIP-111 must run this dry-run sequence on the task branch:

1. Start from a stable between-cycles baseline (see execution window below).
2. Add one-manifest policy fixtures **before** changing guard logic.
3. Run `bash scripts/test-release-verifier-policy.sh` and confirm known-good/known-bad PASS/WARN/FAIL judgments under the
   new model.
4. Run release-please in PR-only/dry-run mode for `premain` using the proposed single manifest and prerelease config;
   capture the expected RC PR diff and confirm it changes only the single manifest/changelog/release metadata intended by
   v2.
5. Run release-please in PR-only/dry-run mode for `main`; capture the expected stable PR diff and confirm no second
   manifest or committed TS/Py source version file is required.
6. Build release assets in a local CI-equivalent workspace with a synthetic `tag_name` for one RC and one stable version;
   inspect the TS tarball and Py wheel/sdist metadata to confirm generated package versions match the tag.
7. Run HYG (`bash scripts/test-release-hygiene-policy.sh`, `bash scripts/test-release-verifier-policy.sh`,
   `bash scripts/verify-release-cycle-state.sh`, `bash scripts/verify-branch-release-supply-chain.sh`) and `make rubric`.
8. Record the dry-run evidence in the TTIP-111 PR body. Do not publish tags, mutate GitHub Releases, or change branch
   protection during the dry run.

## Between-cycles execution window

TTIP-111 must execute only when all of the following are true:

- The current stable release has published successfully and is non-draft with complete TypeScript/Python assets.
- `main` has been backmerged to `staging` through a normal PR.
- No generated release-please PR is open for `premain` or `main`.
- No `premain` RC is active, no `main` pending stable promotion state exists, and no abandoned/exhausted version recovery
  is in progress.
- `bash scripts/verify-release-cycle-state.sh` passes on the implementation branch.
- `bash scripts/watch-release-cycle.sh --strict --tag <latest-stable-tag>` passes, or any non-code remote branch warning is
  explicitly resolved by the maintainer before the implementation PR merges.
- The maintainer has explicitly signed off on this document and the dry-run protocol in the TTIP-111 planning/PR thread.

## Rollback plan

Rollback stays PR-based and never reuses an immutable release version:

1. If TTIP-111 fails before merge, close or update the PR; do not patch manifests/tags by hand.
2. If the one-manifest PR merges to `staging` but fails before promotion, revert through a normal PR to `staging` that
   restores the two-manifest workflow files, configs, and guards.
3. If failure is discovered during an RC cycle on `premain`, abandon that RC version through normal release-please flow
   and advance to the next allowed RC with a release-eligible PR / `Release-As` footer. Do not delete/recreate tags or
   release assets.
4. If failure is discovered after stable publication, keep the published release immutable, open a patch release through
   `staging -> premain -> main`, and document any asset metadata mismatch as a release note / migration note if consumers
   could have pinned it.
5. Retired helper scripts are not recovery mechanisms. If rollback requires restoring a retired helper or two-manifest
   guard, do it through a normal signed PR with the failing release evidence attached; do not patch release branches,
   tags, manifests, or assets by hand.

## Historical maintainer sign-off gate before TTIP-111

This sign-off text was required before starting THE-2449 / TTIP-111:

> I approve TableTheory release-lane v2 implementation against
> `docs/development/planning/tabletheory-release-lane-v2-design-2026.md`, including the single-manifest plan, generated
> TS/Py release-build versions, merge-queue requirement, dry-run protocol, rollback plan, and between-cycles execution
> window.

The Factory/operator THE-2450 assignment supersedes the remaining "do not start" language for this implementation path:
release-cycle proof is a release execution/readiness gate, not an implementation prerequisite. If live branch-protection
or merge-queue settings cannot be changed through PR files, the implementation reports the exact operator-owned setting
that remains.
