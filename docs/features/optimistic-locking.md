---
title: Optimistic Locking
description: Version-conditional writes and update-builder patterns for optimistic concurrency.
---

# Optimistic Locking

The version role is TableTheory's optimistic concurrency field. The [optimistic-locking P0 scenario](https://github.com/theory-cloud/tabletheory/blob/main/contract-tests/scenarios/p0/04-version-optimistic-lock.yml)
pins the portable behavior: a write with the current version succeeds and moves
the persisted item to the next version; a stale-version write fails with a typed
condition error.

Public runtime ergonomics differ slightly, but the shared contract is the same:
the high-level versioned update path adds the version condition and increments
the version on success. On a version mismatch (another writer raced ahead), the
write fails — never a silent overwrite.

## Why this matters

Every Theory Cloud consumer that ships versioned writes — AppTheory's
idempotency state, Autheory's session refreshes, theory-mcp-server's agent
memory cursor — depends on guarded updates being expressed consistently. A Go
writer racing a Python writer must use the same expected-version value and see
one of the writes fail deterministically.

## Go

```go
type Counter struct {
    PK      string `theorydb:"pk"      json:"pk"`
    SK      string `theorydb:"sk"      json:"sk"`
    Value   int64  `json:"value"`
    Version int64  `theorydb:"version" json:"version"`
}

var c Counter
if err := db.Model(&Counter{PK: "TENANT#42", SK: "COUNTER#impressions"}).First(&c); err != nil {
    return err
}

c.Value++
if err := db.Model(&c).Update("value"); err != nil {
    // Version-mismatch is surfaced as a typed error; check via the
    // pkg/errors helpers (see api-reference.md).
    return err
}
```

## TypeScript

```typescript
const Counter = defineModel({
  name: 'Counter',
  table: { name: 'counters_contract' },
  keys: {
    partition: { attribute: 'PK', type: 'S' },
    sort:      { attribute: 'SK', type: 'S' },
  },
  attributes: [
    { attribute: 'PK',      type: 'S', roles: ['pk'] },
    { attribute: 'SK',      type: 'S', roles: ['sk'] },
    { attribute: 'value',   type: 'N' },
    { attribute: 'version', type: 'N', roles: ['version'] },
  ],
});

const key = { PK: 'TENANT#42', SK: 'COUNTER#impressions' };
const c   = await db.get('Counter', key);

// Pass the just-read version through; TheorydbClient.update asserts it
// matches the persisted item before mutating, and bumps it on success.
await db.update(
  'Counter',
  { ...key, value: (c.value as number) + 1, version: c.version },
  ['value'],
);
```

## Python

```python
@dataclass(frozen=True)
class Counter:
    pk:      str = theorydb_field(roles=["pk"])
    sk:      str = theorydb_field(roles=["sk"])
    value:   int = theorydb_field()
    version: int = theorydb_field(roles=["version"])

model = ModelDefinition.from_dataclass(Counter, table_name="counters_contract")
table = Table(model, client=client)

c = table.get("TENANT#42", "COUNTER#impressions")
table.update(
    "TENANT#42",
    "COUNTER#impressions",
    {"value": c.value + 1},
    expected_version=c.version,
)
```

## Versioning a newly-created item

The first write of an item that does not yet exist starts at `version = 0`.
Subsequent versioned writes increment the value, so the first successful update
moves it to `version = 1`.

## Update is not an upsert

A versioned update always asserts `version == currentVersion`. That condition
also applies when `currentVersion` is `0`, because zero is the valid persisted
version immediately after creation. Skipping the condition for zero would
disable optimistic locking for every first post-create update.

Consequently, an update against an absent item fails with the typed
condition/version-conflict error; it never creates the item. Use the runtime's
create operation for a first write, or its explicitly named save/upsert
operation when overwrite semantics are intended. Do not implement a repository
`Put` method by routing both first writes and mutations through `Update`.

## Versioning under transactions

Optimistic locking composes with DynamoDB transactions. A transactional write group that includes a versioned item asserts the version on the item's condition; if any condition fails, the whole transaction fails atomically. See [Transactions](transactions.md).

## Anti-patterns

- **Don't read with one runtime, write with another, and mutate the version field manually in between.** TableTheory is the only thing that should mutate the version role.
- **Don't catch the conflict error and retry indefinitely.** Bound the retry loop. The contract guarantees deterministic loss, not deterministic eventual success.
- **Don't omit the version role from your model and expect "best effort" concurrency.** No version role means no locking — you opt in by declaring the role on a field.

## Related

- [Lifecycle Timestamps](lifecycle-timestamps.md) — lifecycle roles and runtime-specific timestamp automation notes
- [Transactions](transactions.md) — composing versioned writes into atomic groups
- [Contract Scenarios](../reference/contract-scenarios.md) — the full optimistic-locking specification
