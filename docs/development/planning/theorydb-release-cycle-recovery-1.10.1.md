# TableTheory Release-Cycle Recovery: 1.10.1

This record documents the release-cycle recovery decision for version `1.10.1` (addressing THE-2101 / THE-2104) in TableTheory's one release lane: `staging` -> `premain` -> `main` -> `staging` (staging -> premain -> main -> staging).

## Observed Facts

- **Latest Published Release:** The latest successfully published release remains `v1.10.0`. No `v1.10.1` or `v1.10.1-rc` has been released.
- **PR #281:** Successfully landed the 2026-06-18 security remediation into `staging` with green checks.
- **PR #282 & PR #284:** PR #282 (`staging` -> `premain`) and PR #284 (`premain` -> `main`) were merged despite red Release Hygiene checks. Consequently, they do not serve as proof of release-process health. These red promotions are preserved solely as history, and must not be used as release proof.
- **PR #285:** The release-please generated PR `chore(premain): release 1.10.1-rc` was closed unmerged because the postcondition expected a numbered RC shape (e.g., `1.10.1-rc.N`). It should remain closed unless release-please recreates or updates it through normal automation.
- **PR #287:** The promotion PR `staging` -> `premain` is currently open but invalid as recovery due to an empty diff, lack of a release-eligible conventional commit, and lack of a numbered RC `Release-As` footer. It must not be merged as recovery.
- **Approach Constraints:** Operator has explicitly rejected bootstrap fallback / forced-green approaches (such as `factory/the-2104-tabletheory-premain-ci-bootstrap` or any manual tag/manifest/release path).

## Required Recovery Path

Recovery must progress strictly through the normal protected-branch and release-please flow:

1. **Staging Recovery PR (Current Assignment):** Land a staging PR carrying this recovery document and a release-eligible conventional commit containing the `Release-As: 1.10.1-rc.1` footer:
   ```text
   fix(security): recover release cycle for 1.10.1

   Release-As: 1.10.1-rc.1
   ```
2. **Staging to Premain Promotion:** Merge `staging` to `premain` via a promotion PR, verifying it passes with green Release Hygiene checks.
3. **Release-Please RC PR `1.10.1-rc.1`:** Let release-please open the generated RC release PR targeting `premain`.
4. **RC Evidence:** Verify the generated RC release is successfully published with required assets (TypeScript and Python packages) as prerelease evidence, and tagged.
5. **Premain to Main Promotion:** Merge `premain` to `main` via a promotion PR.
6. **Stable Release-Please PR `1.10.1`:** Let release-please open the stable release PR `1.10.1` targeting `main`.
7. **Backmerge:** Merge the stable release and then backmerge `main` into `staging` via a normal PR.

## Security Invariant

The original THE-2101 security remediation fixes must remain fully effective. Recovery actions must not weaken, bypass, or alter these guards:
- **Token Protection:** The release token must not be exposed to PR code.
- **Trusted Code Execution:** The privileged release workflow must not execute untrusted npm code at runtime.
- **Key Escaping:** Derived key values must be escaped.
- **Input Validation:** Required derived-key inputs must reject null/undefined values.
- **Provenance Verification:** The release-lane source branch check must not be spoofable.
- **Fail-Closed Audits:** npm audit findings must fail closed unless reviewed expiring acceptance is explicitly enabled.
- **Postconditions Verification:** Release guards must verify release-please PR postconditions.

## Stop Conditions

- Do not create, recreate, or delete tags for recovery.
- Do not rerun failed workflows as recovery.
- Do not hand-publish releases, edit immutable GitHub releases, or upload release assets by hand.
- Do not hand-edit manifests (`.release-please-manifest*.json`), changelogs, package versions (`ts/package*.json`), or version descriptors (`py/src/theorydb_py/version.json`); release-please owns those updates.
- Do not hard reset, force-push, or mutate protected branches directly.
