# TableTheory Contract Tests (Seed Fixtures)

This folder contains **seed** DMS + scenario fixtures meant to bootstrap the shared contract test suite described in:

- `docs/development/planning/theorydb-contract-tests-suite-outline.md`

This folder is structured to be extracted into its own repo (suggested name: `theorydb-contract-tests`).

## Layout

```text
contract-tests/
  dms/v0.1/models/*.yml
  scenarios/p0/*.yml
  golden/cursor/*
  runners/go
  runners/ts
```

## Fixture conventions

- Scenario values are “logical” (strings/numbers/bools/arrays/objects). The contract runner encodes them based on DMS
  attribute `type`.
- Set attributes (`SS`/`NS`/`BS`) must be compared **order-insensitively**.
- Cursor fixtures must be compared **byte-for-byte** (`.cursor` string).

## Run (local)

Start DynamoDB Local:

```bash
docker compose -f contract-tests/docker-compose.yml up -d
```

Run the Go runner:

```bash
cd contract-tests/runners/go
go test ./... -v
```

Run the TypeScript runner:

```bash
npm --prefix contract-tests/runners/ts ci
npm --prefix contract-tests/runners/ts test
```

Run the Python runner:

```bash
uv --directory py sync --frozen --all-extras
uv --directory py run pytest -q ../contract-tests/runners/py
```
