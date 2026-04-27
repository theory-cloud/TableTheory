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
- `make rubric` is the core CI parity/quality check and must pass before opening or updating a pull request.

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

Branch roles:

- **`staging`**: integration branch (all work lands here first)
- **`premain`**: prerelease branch (RCs like `vX.Y.Z-rc.N`)
- **`main`**: stable release branch (releases like `vX.Y.Z`)

Multi-language versioning:

- **Stable manifest**: `.release-please-manifest.json`
- **Prerelease manifest**: `.release-please-manifest.premain.json`
- **TypeScript**: `ts/package.json`, `ts/package-lock.json`
- **Python**: `py/src/theorydb_py/version.json`

Every PR to `staging` must check both release and release-candidate version alignment before it is opened or updated.
The stable release baseline and prerelease/RC baseline must agree with the current `main` line so release-please never
generates an older RC (for example `v1.6.0-rc.N`) after a newer stable release (for example `v1.7.0`) has shipped.

Release automation must keep these files coherent so the stable line never lags the prerelease line on promotion.
The rubric enforces this via:

- `bash scripts/verify-branch-release-supply-chain.sh`
- `bash scripts/verify-branch-version-sync.sh`
