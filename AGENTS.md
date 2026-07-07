# TableTheory Agent Bootstrap

- This repository's stewardship agent is the published `tabletheory` Theory Cloud namespace agent.
- The namespace agent is materialized into git-ignored host installs under `.codex/`, `.claude/`, and `.agents/`.
- The published namespace is the source of truth; do not hand-edit those materialized files. Re-materialize the namespace agent to change them.
- See `.codex/steward.md` for the steward's full identity and skills.

# Repository Guidelines

## Stewardship Loop
- TableTheory maintenance runs through a two-party stewardship loop: the sole human maintainer and the dedicated TableTheory steward.
- Assume there is no additional human maintainer or reviewer who will catch process, contract, release, or CI mistakes.
- All repository changes, pull requests, release-flow decisions, and stewardship corrections go through this maintainer/steward loop.

## Project Structure & Module Organization
- `theorydb.go` and other root `*.go`: main `theorydb` package.
- `pkg/`: public packages (`core/`, `model/`, `query/`, `session/`, `types/`, `marshal/`, `transaction/`, `errors/`, `mocks/`).
- `internal/expr/`: internal-only expression helpers.
- `tests/`: shared test utilities + suites (`tests/integration/`, `tests/benchmarks/`, `tests/stress/`, `tests/models/`).
- `examples/`: runnable examples (including `examples/lambda/`).
- `docs/` and `scripts/`: documentation and helper scripts.

## Build, Test, and Development Commands
Go/tooling:
- Install the Go toolchain declared in `go.mod` (includes a `toolchain` pin).
- If you have Ubuntu snap `go` installed, ensure it doesn't override the pinned toolchain (otherwise you may see `compile: version "goX.Y.Z" does not match go tool version "goX.Y.W"` during coverage/covdata); fix with `export GOTOOLCHAIN="$(awk '/^toolchain /{print $2}' go.mod | head -n1)"` (the `Makefile` already exports this).
- `make install-tools` — install `golangci-lint` and `mockgen`

Common workflows:
- `make fmt` — format (`go fmt` + `gofmt -s -w .`)
- `make lint` — lint (`golangci-lint run ./...`)
- `make test-unit` — fast unit tests (race + coverage; no DynamoDB Local)
- `make unit-cover` — offline coverage baseline (`go test -short ...`)
- `make integration` / `make test` — integration or full suite (starts DynamoDB Local)
- `make benchmark` / `make stress` — performance and stress suites
- `make lambda-build` — build `examples/lambda` → `build/lambda/function.zip`

Single test example: `go test -v -run TestName ./pkg/query`

## Coding Style & Naming Conventions
- Run `make fmt` before pushing; keep changes gofmt-clean.
- Use standard Go naming (exported `PascalCase`, packages `lowercase`).
- Model structs must use canonical tags: `theorydb:"pk"`/`theorydb:"sk"` + matching `json:"..."` (see `docs/development-guidelines.md`).

## Testing Guidelines
- Tests use `testing` + `stretchr/testify`; prefer table-driven tests.
- Unit tests should avoid Docker; use interfaces in `pkg/core/` and mocks in `pkg/mocks/`.
- Integration tests rely on DynamoDB Local and `DYNAMODB_ENDPOINT` (see `tests/README.md` and `./tests/setup_test_env.sh`).
- `make rubric` is the core CI parity/quality check and must pass before opening or updating a pull request targeting
  `staging`.

## Commit & Pull Request Guidelines
- Branch naming commonly uses `feature/...`, `fix/...`, `chore/...`.
- Prefer Conventional Commit-style subjects (`feat:`, `fix:`, `docs:`, `test:`) and keep the first line ≤72 chars.
- PRs: describe intent and scope, link issues, list commands run, add/adjust tests, and update `CHANGELOG.md` + relevant docs when public APIs change (see `CONTRIBUTING.md`).

## Release / Versioning (immutable GitHub releases)

TableTheory publishes **immutable** GitHub releases (no retagging / no overwriting release assets). Any change that must be
published requires:

- **staging → premain**: merge a PR from `staging` into `premain` to start the prerelease pipeline (RCs)
- **premain → main**: merge a PR from `premain` into `main` to start the stable release pipeline
- **post-release sync**: back-merge `main` into `staging` so the next cycle starts from the latest stable baseline
- **in-flight RC repair (exceptional)**: when `premain` already contains `staging` and RC release content must be
  reconciled back into `staging` to avoid forward merge conflicts, open a premain-derived repair PR back to `staging`.
  Follow-up repair PRs to `staging` may preserve that already-accepted RC manifest until stable publication, but must be
  based on current `staging` and must not change the RC baseline. This is not the normal release hop and must remain
  provenance-verified as a repair state.

The cross-framework release lane is exactly `staging -> premain -> main -> staging`. `staging` is the only branch that
requires the full gov-infra rubric, and only on PRs targeting `staging` plus manual workflow dispatch. PRs targeting
`premain` or `main` require release-hygiene checks only, not the full rubric.

Protected `staging`, `premain`, and `main` are merge-queue branches. Repo-owned workflows support this with
`merge_group` triggers: `quality-gates.yml` validates queued `staging` merges with the full rubric, and
`release-hygiene.yml` validates queued `premain`/`main` promotion or release-please merges with release hygiene. The live
GitHub setting is still operator-owned: require the merge queue and the corresponding checks in branch protection/rulesets
before relying on the v2 lane.

Branch roles:

- **`staging`**: integration branch (all work lands here first)
- **`premain`**: prerelease branch (RCs like `vX.Y.Z-rc.N`)
- **`main`**: stable release branch (releases like `vX.Y.Z`)

Promotion PRs to `premain` or `main` are release-intent gates, not optional/no-op syncs. A `staging` -> `premain` PR
must contain a release-eligible conventional commit or RC-shaped `Release-As:` footer that release-please can use to open
the generated RC PR. A `premain` -> `main` PR must carry a pending RC promotion state that can become stable; `main` must
never receive an RC-shaped release PR or release.

Multi-language versioning:

- **Single release-please manifest**: `.release-please-manifest.json` is the only release-please manifest.
- **TypeScript/Python package metadata**: `ts/package.json`, `ts/package-lock.json`, and
  `py/src/tabletheory_py/version.json` remain package metadata, but release workflows stamp them from `tag_name` in the
  release-build workspace before packaging instead of committing RC churn through release-please extra-files.
- **Asset proof**: release workflows must verify the TypeScript tarball and Python wheel/sdist metadata match `tag_name`
  before upload.

Release ownership:

- `staging` owns integration-ready code, docs, security/toolchain updates, and the latest stable baseline returned from
  `main`. It must not pretend to cut a release.
- `premain` owns RC generation. It may carry `X.Y.Z-rc.N` in `.release-please-manifest.json` while the RC phase is
  active.
- `main` owns stable state. Outside explicit single-manifest pending stable promotion, `.release-please-manifest.json` on
  `main` must not contain `-rc`.

Immutable version reuse:

- Treat published GitHub release tag names as one-time-use, even if the published release or git tag was later deleted.
  A deleted immutable release/tag has exhausted that version name for release-cycle purposes.
- Do not manually recreate tags, hand-publish releases, edit immutable release assets, or hand-edit manifests as recovery.
- If publishing fails with `tag_name was used by an immutable release`, recover through a normal release-eligible PR and
  let release-please advance to the next RC/stable version for the single release lane.
- If a version is abandoned or exhausted, skip it only through a normal release-eligible commit/PR with a release-please
  `Release-As` footer for the next allowed version. Do not recover through tags, resets, manual release edits,
  manifest/package-version edits, or reruns of failed exhausted-version workflows.
- Current THE-1869 release-cycle decision: `1.9.2` is abandoned; the next RC must be `v1.9.3-rc.1`; the next stable
  release must be `v1.9.3`; the release-eligible recovery commit must carry `Release-As: 1.9.3-rc.1` and that footer
  must survive the `staging` -> `premain` merge path.

Stable promotion path:

- Do not start `premain` -> `main` promotion until the intended RC exists as a GitHub release that is published,
  non-draft, marked prerelease, backed by `refs/tags/vX.Y.Z-rc.N`, and complete with the required TypeScript/Python
  assets.
- Open the promotion PR from `premain` to `main` and merge it through the merge queue; do not create a local
  stable-normalization branch as the normal path.
- After the `premain` -> `main` promotion merges, `main` may briefly contain a single-manifest RC. This state is allowed
  only until `.github/workflows/release-pr.yml` opens the stable release-please PR and that PR merges.
- Pending stable promotion is explicit automation state only:
  `RELEASE_CYCLE_ALLOW_PENDING_STABLE_PROMOTION=true` may be used by the release workflows to verify the single-manifest
  RC on `main`, and `.github/workflows/release.yml` must skip stable release creation while that state is present.
- `.github/workflows/release-pr.yml` computes the stable `release-as` value from the single-manifest RC baseline (for
  THE-1869, `1.9.3`) and opens the stable release-please PR.
- The stable release-please PR must normalize `.release-please-manifest.json` and `CHANGELOG.md` to stable state.
  TypeScript/Python asset versions are generated from `tag_name` at release-build time and must not be committed by the
  release workflow.
- After the stable release-please PR merges, strict stable state is required again: run
  `bash scripts/verify-release-cycle-state.sh` without the pending env var, and confirm the single manifest is stable.
- If pending state persists because release-please did not open the stable PR, pause and investigate the workflow/check
  failure. Do not patch `main`, retag, edit releases, or hand-edit manifests to advance the cycle.
- After the stable release is published, do not let CI push sync commits to `premain` or `staging`. The next operator step
  is a normal PR backmerge from `main` to `staging`; `premain` receives that baseline through the next
  `staging` -> `premain` promotion.
- No local stable-normalization helper is part of the normal path. Stable manifest and changelog normalization belong to
  the generated stable release-please PR; post-stable baseline sync belongs to a normal `main` -> `staging` PR.

Release watchpoints and stop conditions:

- Stop if live GitHub branch protection/rulesets do not require the merge queue and the queue-compatible
  `Quality Gates (10/10 Rubric)` / `Release Hygiene` checks for their protected target branches.
- Stop if `main` carries an RC manifest outside explicit single-manifest pending stable promotion.
- Stop if `.release-please-manifest.json` is an RC version on `staging`, unless the PR is a verified in-flight premain RC
  repair where `premain` already contains `staging` and the release content is being reconciled to avoid forward merge
  conflicts, or a verified follow-up repair that preserves the already-accepted `staging` RC manifest before stable
  publication. Stop if the retired `.release-please-manifest.premain.json` appears on release branches.
- Stop if `premain` release track is behind `origin/main`, or if `staging` lacks the latest stable baseline after a stable
  release.
- Stop if SEC-2/govulncheck still observes Go `1.26.3`, COM-8 branch/version sync fails, or release-please opens a
  stable PR for an RC version.
- Stop if `main` remains in pending stable promotion and no stable release-please PR opens for the computed stable
  version.
- Stop if release-please reports "No user facing commits" on a `premain` or `main` gate. Fix the staging content, PR
  squash title, or `Release-As:` footer through normal PR flow; do not recover through tags, resets, direct pushes,
  manual release edits, or manifest/package-version edits.
- Stop if `.github/workflows/prerelease-pr.yml` completes after a `staging` -> `premain` promotion without an open
  generated RC release-please PR targeting `premain`.
- Stop if a release workflow was expected to create a release but did not report `release_created`, if asset/publish steps
  have no `tag_name`, or if a GitHub release exists without the TypeScript/Python assets.
- Stop if a requested release tag is draft, has no `publishedAt`, uses an `untagged-...` release URL, is missing
  `refs/tags/<tag>`, or points the release target and git tag ref at different commits.
- Stop if release recovery would reuse a deleted or exhausted immutable release version; use a release-eligible PR to
  advance to the next release-please version instead.
- Stop if abandoned-version recovery lacks the required release-please `Release-As` footer or tries to hand-edit manifests,
  package versions, tags, releases, or release assets.
- Stop if automation tries to push directly to `staging`, `premain`, or `main` where this policy requires PR sync.

Useful checks:

- `bash scripts/verify-branch-release-supply-chain.sh`
- `bash scripts/verify-branch-version-sync.sh`
- `bash scripts/verify-release-cycle-state.sh`
- `bash scripts/watch-release-cycle.sh` for read-only PASS/WARN/FAIL branch and release watchpoints; add `--strict`
  before merge/release gates.

Every PR to `staging` must keep the single-manifest release lane coherent so release-please never generates an older RC
(for example `v1.6.0-rc.N`) after a newer stable release (for example `v1.7.0`) has shipped. Release asset package
versions are generated from tags and verified at build time. The rubric enforces this via:

- `bash scripts/verify-branch-release-supply-chain.sh`
- `bash scripts/verify-branch-version-sync.sh`
