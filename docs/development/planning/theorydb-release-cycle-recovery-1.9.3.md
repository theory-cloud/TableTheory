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
5. Promote `premain` to `main` through the documented normalized stable-promotion PR.
6. Let release-please open and CI publish the generated `v1.9.3` stable release.
7. Sync the stable `main` baseline back to `staging` and `premain` through PRs or documented verified automation.

## Stop Conditions

- Do not create, recreate, or delete tags for recovery.
- Do not rerun failed `v1.9.2` workflows as recovery.
- Do not hand-publish releases, edit immutable GitHub releases, or upload release assets by hand.
- Do not hand-edit `.release-please-manifest*.json`, `CHANGELOG.md`, `ts/package*.json`, or
  `py/src/theorydb_py/version.json`; release-please owns those updates.
- Do not hard reset, force-push, or mutate protected branches directly.

Abandoned or exhausted immutable versions are skipped only through a normal release-eligible commit/PR with a
release-please `Release-As` footer. For this recovery, the footer must survive the `staging` merge and later
`staging` -> `premain` promotion so release-please advances the prerelease lane to `v1.9.3-rc.1` instead of `1.9.2`.
