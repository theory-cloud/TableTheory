# Repository Guidelines

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

## Commit & Pull Request Guidelines
- Branch naming commonly uses `feature/...`, `fix/...`, `chore/...`.
- Prefer Conventional Commit-style subjects (`feat:`, `fix:`, `docs:`, `test:`) and keep the first line ≤72 chars.
- PRs: describe intent and scope, link issues, list commands run, add/adjust tests, and update `CHANGELOG.md` + relevant docs when public APIs change (see `CONTRIBUTING.md`).
