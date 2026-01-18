# TableTheory: Contract Test Suite Outline (Runnable, Go↔TS↔Py)

Goal: a single, shareable test suite that **prevents semantic drift** between TableTheory implementations in different
languages (starting with Go ↔ TypeScript, then adding Python).

This is a runnable outline: it specifies folder structure, scenario formats, required drivers, and the exact commands
each language repo should expose so CI can run the same contract surface.

Related:
- Spec: `docs/development/planning/theorydb-spec-dms-v0.1.md`
- Parity tiers: `docs/development/planning/theorydb-go-ts-parity-matrix.md`
- Multi-lang plan: `docs/development/planning/theorydb-multilang-roadmap.md`

## Repository strategy (recommended)

Create a dedicated repo: `theorydb-contract-tests` (or `theorydb-spec` if you want spec + tests together).

Each implementation repo (`theorydb` (Go), `tabletheory-ts`) should:
- vendor/pin the contract tests repo at a specific commit (git submodule or `git subtree`), OR
- pull it as a package (later; not required for v0.1).

Why a dedicated repo: contract tests must be **shared** and **pinned**; otherwise they drift independently.

## “Runnable” contract test architecture

Contract tests run scenarios against an implementation through a **Driver** interface.

- Scenarios are language-agnostic YAML files (`.yml`) + DMS fixtures.
- Each language repo implements a small driver wrapper that calls its own TableTheory API.
- The runner:
  - creates tables using AWS SDK (not the library under test),
  - runs scenario steps through the driver,
  - asserts results (items, errors, cursor strings) using canonical encodings from the spec.

## Proposed folder layout (contract repo)

```text
theorydb-contract-tests/
  README.md
  docker-compose.yml
  dms/
    v0.1/
      models/
        user.yml
        order.yml
  scenarios/
    p0/
      01-crud-basic.yml
      02-omitempty.yml
      03-lifecycle-created-updated.yml
      04-version-optimistic-lock.yml
      05-ttl-epoch-seconds.yml
      06-sets.yml
      07-errors-condition-failed.yml
    p1/
      01-query-eq-begins-with.yml
      02-query-index-selection.yml
      03-pagination-cursor-golden.yml
      04-projection.yml
      05-filter-groups.yml
    p2/
      01-batch-get.yml
      02-batch-write.yml
      03-transact-write.yml
  golden/
    cursor/
      cursor_v0.1_basic.json
      cursor_v0.1_basic.cursor
  runners/
    ts/
      package.json
      src/
        driver.ts
        runner.ts
      test/
        contract.test.ts
    go/
      go.mod
      internal/
        driver.go
        runner.go
      contract_test.go
    py/
      README.md
      test_*.py
```

Notes:
- `golden/` holds byte-for-byte fixtures (especially cursors) that MUST match across languages.
- `runners/ts` and `runners/go` are reference runners; each implementation repo can either:
  - copy these runners (fastest), or
  - implement the same interfaces in its own test folder.

## Seed fixtures (in this repo, for bootstrapping)

This repo includes an initial seed set so `tabletheory-ts` can start implementing immediately:

- `contract-tests/README.md`

It mirrors the proposed contract repo layout (`dms/`, `scenarios/`, `golden/`) and should be moved/copied into the shared
contract repo once it exists.

## DynamoDB Local pin (determinism)

Use the same pinned image everywhere:

- `amazon/dynamodb-local:3.1.0`

Contract repo `docker-compose.yml`:

```yaml
services:
  dynamodb-local:
    image: amazon/dynamodb-local:3.1.0
    ports: ["8000:8000"]
    command: "-jar DynamoDBLocal.jar -sharedDb -inMemory"
```

## Standard environment variables

Every runner must support:

- `DYNAMODB_ENDPOINT` (default `http://localhost:8000`)
- `AWS_REGION` or `AWS_DEFAULT_REGION` (default `us-east-1`)
- `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` (dummy values for local)

## Scenario format (v0.1)

Scenario YAML is intentionally constrained so it can be implemented quickly in TS and Go.

### Example: `p0/01-crud-basic.yml`

```yaml
name: "p0.crud.basic"
dms_version: "0.1"
model: "User"
table:
  name: "users_contract"
steps:
  - op: create
    if_not_exists: true
    item:
      PK: "USER#1"
      SK: "PROFILE"
      emailHash: "hash_abc"
      tags: ["a", "b"]        # SS in DMS
      version: 0             # N
    expect:
      ok: true

  - op: get
    key:
      PK: "USER#1"
      SK: "PROFILE"
    save:
      createdAt0: "createdAt"
      updatedAt0: "updatedAt"
    expect:
      item_contains:
        PK: "USER#1"
        SK: "PROFILE"
        emailHash: "hash_abc"
      item_has_fields: ["createdAt", "updatedAt", "version"]

  - op: update
    item:
      PK: "USER#1"
      SK: "PROFILE"
      tags: ["a", "b", "c"]
      version: 0             # driver uses this as currentVersion
    expect:
      ok: true

  - op: get
    key:
      PK: "USER#1"
      SK: "PROFILE"
    expect:
      item_contains:
        tags: ["a", "b", "c"]
        version: 1

  - op: delete
    key:
      PK: "USER#1"
      SK: "PROFILE"
    expect:
      ok: true

  - op: get
    key:
      PK: "USER#1"
      SK: "PROFILE"
    expect:
      error: "ErrItemNotFound"
```

### Scenario assertion primitives

Minimum assertions required for v0.1:

- `ok: true`
- `error: <ErrorCode>`
- `item_contains: { attr: value }` (subset match)
- `item_equals: { ... }` (exact match; optional in v0.1)
- `item_has_fields: [attr]` (presence)
- `item_missing_fields: [attr]` (absence in the *raw DynamoDB item*; critical for `omit_empty`)
- `raw_attribute_types: { attr: "S"|"N"|"B"|... }` (type assertions against the raw DynamoDB item)
- `cursor_equals: "<cursor>"` (byte-for-byte; for golden cursor tests)
- `item_field_equals_var: { attr: "varName" }` (value equals previously saved var)
- `item_field_not_equals_var: { attr: "varName" }` (value differs from previously saved var)

Value encoding in scenario files is “logical” (strings/numbers/bools/arrays/objects); the runner encodes based on DMS
attribute `type`.

Comparison rules:
- DynamoDB set types (`SS`/`NS`/`BS`) MUST be compared **order-insensitively**.

## Driver interface (what each implementation must provide)

### P0 driver operations

- `create(modelName, item)`
- `get(modelName, key)`
- `update(modelName, item, options?)`
- `delete(modelName, key)`

### P1 driver operations

- `query(modelName, request)` returning `{ items, cursor? }`
- `scan(modelName, request)` returning `{ items, cursor? }`
- cursor encode/decode must match `theorydb-spec-dms-v0.1` requirements

### P2 driver operations

- `batchGet(modelName, keys)`
- `batchWrite(modelName, puts?, deletes?)`
- `transactWrite(ops[])`

### Error code mapping

Drivers MUST map native errors to these stable codes (minimum set):

- `ErrItemNotFound`
- `ErrConditionFailed`
- `ErrInvalidModel`
- `ErrMissingPrimaryKey`
- `ErrInvalidOperator`
- `ErrEncryptionNotConfigured` (reserved for post-P2 suites)
- `ErrEncryptedFieldNotQueryable` (reserved for post-P2 suites)

## Test suites (what to implement first)

### P0 (ship-blocking)

1) **CRUD basics** (`p0/01-crud-basic.yml`)
2) **`omit_empty` behavior** (`p0/02-omitempty.yml`)
   - empty string/array/map omitted
   - empty set encoded as NULL then omitted when `omit_empty`
3) **Lifecycle fields** (`p0/03-lifecycle-created-updated.yml`)
   - `createdAt` present after create
   - `updatedAt` changes on update
   - values parse as RFC3339Nano
4) **Version optimistic locking** (`p0/04-version-optimistic-lock.yml`)
   - update requires `version == currentVersion`
   - increments to `currentVersion+1`
   - stale version update fails with `ErrConditionFailed`
5) **TTL encoding** (`p0/05-ttl-epoch-seconds.yml`)
   - stored as epoch seconds (`N`) regardless of input convenience types
6) **Sets** (`p0/06-sets.yml`)
   - `SS` round-trips
   - empty set handling is deterministic
7) **Condition-failed mapping** (`p0/07-errors-condition-failed.yml`)
   - idempotent create with `IfNotExists` returns `ErrConditionFailed` when item exists

### P1 (query + cursor parity)

1) **Operators** (`p1/01-query-eq-begins-with.yml`)
2) **Index selection** (`p1/02-query-index-selection.yml`)
3) **Golden cursor** (`p1/03-pagination-cursor-golden.yml`)
   - uses `golden/cursor/*.cursor` produced by Go reference
   - TS MUST produce byte-identical cursor strings for the same query response
4) **Projection** (`p1/04-projection.yml`)
5) **Filter groups** (`p1/05-filter-groups.yml`)

### P2 (batch + transactions)

1) **BatchGet** (`p2/01-batch-get.yml`) including partial failures/unprocessed keys handling
2) **BatchWrite** (`p2/02-batch-write.yml`) including retry/unprocessed items semantics
3) **TransactWrite** (`p2/03-transact-write.yml`) including per-operation error mapping

## “Runnability” requirements for each implementation repo

### TypeScript repo (`tabletheory-ts`)

Expose these scripts (names are part of the contract):

- `npm run test:contract:p0`
- `npm run test:contract:p1`
- `npm run test:contract:p2`

Each script must:
- start DynamoDB Local (docker compose) OR assume it’s running (documented in `README.md`)
- run only that tier’s scenarios

### Go repo (`theorydb`)

Expose these make targets (or scripts):

- `make test-contract-p0`
- `make test-contract-p1`
- `make test-contract-p2`

These can be thin wrappers that run `go test` against the contract runner folder with tier filtering.

## Golden fixture generation (cursor)

Golden fixtures should be generated by the Go reference implementation and checked in.

Minimum workflow:

1) Generate `golden/cursor/cursor_v0.1_basic.json` (the Cursor JSON before base64url)
2) Generate `golden/cursor/cursor_v0.1_basic.cursor` (the base64url cursor string)
3) TS runner must:
   - produce the same cursor string for the same `LastEvaluatedKey` + index + sort direction
   - decode the golden cursor and recover the expected key map

## CI wiring (high level)

Each implementation CI should run:

1) `contract:p0` on every PR (blocking)
2) `contract:p1` on every PR once query parity is in progress
3) `contract:p2` on every PR once batch/tx parity is in progress

All CI must pin:
- Node version (TS)
- Go toolchain (Go)
- DynamoDB Local image
