---
title: Lifecycle Timestamps
description: created_at and updated_at roles for lifecycle attributes, with runtime-specific automation notes.
---

# Lifecycle Timestamps

`created_at` and `updated_at` are the canonical lifecycle timestamp roles. They
identify the creation and last-update attributes in the model contract.

Current public-runtime behavior is not identical across all generic CRUD APIs:

- **Go** populates lifecycle fields on create and updates `updated_at` on update
  through the query-builder write paths.
- **TypeScript** populates `created_at` / `updated_at` on create/save and updates
  `updated_at` during `TheorydbClient.update`.
- **Python** currently treats the roles as schema/attribute metadata. `Table.put`
  serializes the dataclass values supplied by the caller, and `Table.update` does
  not automatically set `updated_at`.

The [lifecycle P0 fixture](https://github.com/theory-cloud/tabletheory/blob/main/contract-tests/scenarios/p0/03-lifecycle-created-updated.yml)
tracks the shared contract shape and expected timestamp ordering, but consumers
should follow the runtime-specific notes below until Python lifecycle automation
is implemented in the public `Table` API.

## Go

```go
type Note struct {
    PK        string    `theorydb:"pk"          json:"pk"`
    SK        string    `theorydb:"sk"          json:"sk"`
    Body      string    `json:"body"`
    CreatedAt time.Time `theorydb:"created_at"  json:"created_at"`
    UpdatedAt time.Time `theorydb:"updated_at"  json:"updated_at"`
}

n := &Note{PK: "USER#1", SK: "NOTE#welcome", Body: "Hi."}
db.Model(n).Create()
// n.CreatedAt and n.UpdatedAt are populated by the Go runtime.
```

## TypeScript

```typescript
const Note = defineModel({
  name: 'Note',
  table: { name: 'notes_contract' },
  keys: {
    partition: { attribute: 'PK', type: 'S' },
    sort:      { attribute: 'SK', type: 'S' },
  },
  attributes: [
    { attribute: 'PK',        type: 'S', roles: ['pk'] },
    { attribute: 'SK',        type: 'S', roles: ['sk'] },
    { attribute: 'body',      type: 'S' },
    { attribute: 'createdAt', type: 'S', roles: ['created_at'] },
    { attribute: 'updatedAt', type: 'S', roles: ['updated_at'] },
  ],
});

await db.create('Note', { PK: 'USER#1', SK: 'NOTE#welcome', body: 'Hi.' });
// createdAt and updatedAt are written by the TypeScript runtime.
```

## Python

Python's `theorydb_field(roles=[...])` maps the lifecycle attributes into the
model definition, but callers currently provide the values they want stored.

```python
from datetime import datetime, timezone


def now_iso() -> str:
    return datetime.now(timezone.utc).isoformat()


@dataclass(frozen=True)
class Note:
    pk:         str = theorydb_field(roles=["pk"])
    sk:         str = theorydb_field(roles=["sk"])
    body:       str = theorydb_field()
    created_at: str = theorydb_field(roles=["created_at"])
    updated_at: str = theorydb_field(roles=["updated_at"])

model = ModelDefinition.from_dataclass(Note, table_name="notes_contract")
table = Table(model, client=client)

created = now_iso()
table.put(Note(
    pk="USER#1",
    sk="NOTE#welcome",
    body="Hi.",
    created_at=created,
    updated_at=created,
))

table.update("USER#1", "NOTE#welcome", {"body": "Edited.", "updated_at": now_iso()})
```

## Composes with optimistic locking

In Go and TypeScript update paths, `updated_at` is populated alongside the
`version` role on successful versioned writes. A version conflict aborts the
write — neither version nor `updated_at` advances. In Python, use
`update_builder(...).add("version", 1).condition_version(...)` and set
`updated_at` explicitly if you need both fields to advance together.

## Composes with TTL

`updated_at` does **not** influence the TTL attribute. TTL is computed from the field with the `ttl` role, independent of when the item was last touched. If you want "extend TTL on every write," update the TTL field explicitly in your write path.

## Anti-patterns

- **Don't set `created_at` or `updated_at` manually in Go or TypeScript create/update paths.** Those runtimes own lifecycle fields on those paths.
- **Don't assume Python `Table.put` or `Table.update` generates lifecycle values today.** Supply explicit values in Python until lifecycle automation is added there.
- **Don't rely on `created_at` for tie-breaking equal-version writes.** Use the version role; that's what it's for.

## Related

- [Optimistic Locking](optimistic-locking.md) — the `version` field that composes with these timestamps
- [TTL](ttl.md) — separate timestamp axis governed by its own role
- [Contract Scenarios](../reference/contract-scenarios.md) — the full lifecycle-timestamps specification
