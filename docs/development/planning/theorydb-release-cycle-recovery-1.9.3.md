# TableTheory Release-Cycle Recovery: 1.9.3 Lane

This record documents the THE-1869 release-cycle recovery decision for the abandoned `1.9.2` lane.

## Decision

- `1.9.2` is abandoned and must not be released as an RC or as a stable release.
- The next prerelease lane is `v1.9.3-rc.1`.
- The eventual stable release for this lane is `v1.9.3`.

## Required Recovery Path

Recovery must stay inside the normal protected-branch and release-please flow:

1. Land one release-eligible PR into `staging`.
2. Preserve a release-please footer on the release-eligible commit:

   ```text
   Release-As: 1.9.3-rc.1
   ```

3. Merge `staging` into `premain` through a PR.
4. Let release-please open and CI publish the generated `v1.9.3-rc.1` prerelease.
5. After verifying that `v1.9.3-rc.1` is published, non-draft, marked prerelease, tagged, and asset-complete, promote
   `premain` to `main` through a PR.
6. Let `release.yml` skip stable publishing during the temporary pending stable-promotion state.
7. Let `release-pr.yml` open the stable release-please PR with `release-as: 1.9.3`.
8. Merge the stable release-please PR; release-please must normalize `.release-please-manifest.json`,
   `.release-please-manifest.premain.json`, `ts/package.json`, `ts/package-lock.json`, and
   `py/src/theorydb_py/version.json` to `1.9.3`.
9. Let CI publish the generated immutable `v1.9.3` stable release.
10. Sync the stable `main` baseline back to `staging` and `premain` through PRs or documented verified automation.

During this recovery, a reconciliation PR to `staging` may merge current `premain` first to surface promotion conflicts
before they reach `main`. That PR may carry `.release-please-manifest.premain.json`, `ts/package*.json`, and
`py/src/theorydb_py/version.json` at the active `1.9.3-rc.1` lane as merge-carried state only. The stable manifest must
remain on the current `main` baseline, the RC files must be internally consistent and ahead of that baseline, and the
state must be removed by the stable release-please PR plus post-stable sync after `v1.9.3` publishes.

## Stop Conditions

- Do not create, recreate, or delete tags for recovery.
- Do not rerun failed `v1.9.2` workflows as recovery.
- Do not hand-publish releases, edit immutable GitHub releases, or upload release assets by hand.
- Do not hand-edit `.release-please-manifest*.json`, `CHANGELOG.md`, `ts/package*.json`, or
  `py/src/theorydb_py/version.json`; release-please owns those updates.
- Do not use a local stable-normalization branch as the normal path; `scripts/prepare-stable-promotion.sh` is only a
  diagnostic/fallback helper if release automation is blocked and recovery is explicitly documented.
- Do not hard reset, force-push, or mutate protected branches directly.

Abandoned or exhausted immutable versions are skipped only through a normal release-eligible commit/PR with a
release-please `Release-As` footer. For this recovery, the footer must survive the `staging` merge and later
`staging` -> `premain` promotion so release-please advances the prerelease lane to `v1.9.3-rc.1` instead of `1.9.2`.
