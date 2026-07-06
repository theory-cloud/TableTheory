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
- Assertion collection keys must be omitted when unused. Present-but-empty maps/lists such as `item_equals: {}`,
  `raw_item_contains: {}`, or `items_contains: []` are invalid fixtures; use scalar assertions such as `ok: true`,
  `error: <ErrorCode>`, `item_count: 0`, or omit the assertion key instead.
- Item/raw-item/read assertions cannot be combined with `error`/`errors` expectations. Assertion keys that inspect
  returned data imply a successful operation/read even when `ok: true` is omitted.
- P0 scenarios that describe newly specified behavior before every runtime has shipped support MUST declare
  `requires_capabilities`. Runners skip capability-gated scenarios until their runtime advertises that capability; the
  follow-up runtime milestone removes the skip by adding the advertised capability and making the scenario pass.

## Run (local)

Start DynamoDB Local:

```bash
docker compose -f contract-tests/docker-compose.yml up -d
```

Run the Go runner:

```bash
cd contract-tests/runners/go
go test ./... -count=1 -v
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
