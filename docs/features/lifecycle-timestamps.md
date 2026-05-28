---
title: Lifecycle Timestamps
description: created_at and updated_at roles for lifecycle attributes across Go, TypeScript, and Python.
---

# Lifecycle Timestamps

`created_at` and `updated_at` are the canonical lifecycle timestamp roles. They
identify the creation and last-update attributes in the model contract. Go,
TypeScript, and Python populate these fields in their shared P0 write paths:

- create/save writes populate `created_at` and `updated_at` with the same write
  timestamp;
- update writes preserve `created_at` and advance `updated_at`;
- a failed conditional update does not advance `updated_at`.

The [lifecycle P0 fixture](https://github.com/theory-cloud/tabletheory/blob/main/contract-tests/scenarios/p0/03-lifecycle-created-updated.yml)
tracks the shared contract shape and expected timestamp ordering across all
three runtimes.

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

```python
@dataclass(frozen=True)
class Note:
    pk:         str = theorydb_field(name="PK", roles=["pk"])
    sk:         str = theorydb_field(name="SK", roles=["sk"])
    body:       str = theorydb_field()
    created_at: str = theorydb_field(name="createdAt", roles=["created_at"], default="")
    updated_at: str = theorydb_field(name="updatedAt", roles=["updated_at"], default="")
    version:    int = theorydb_field(roles=["version"], default=0)

model = ModelDefinition.from_dataclass(Note, table_name="notes_contract")
table = Table(model, client=client)

table.put(Note(
    pk="USER#1",
    sk="NOTE#welcome",
    body="Hi.",
))

note = table.get("USER#1", "NOTE#welcome")
table.update(
    "USER#1",
    "NOTE#welcome",
    {"body": "Edited."},
    expected_version=note.version,
)
```

## Composes with optimistic locking

On successful versioned writes, `updated_at` advances alongside the `version`
role. A version conflict aborts the write — neither version nor `updated_at`
advances. In Python, pass `expected_version=` to `Table.update` for this
high-level guarded update shape.

## Composes with TTL

`updated_at` does **not** influence the TTL attribute. TTL is computed from the field with the `ttl` role, independent of when the item was last touched. If you want "extend TTL on every write," update the TTL field explicitly in your write path.

## Anti-patterns

- **Don't set `created_at` or `updated_at` manually in high-level create/update paths.** The runtime owns lifecycle fields on those paths.
- **Don't rely on `created_at` for tie-breaking equal-version writes.** Use the version role; that's what it's for.

## Related

- [Optimistic Locking](optimistic-locking.md) — the `version` field that composes with these timestamps
- [TTL](ttl.md) — separate timestamp axis governed by its own role
- [Contract Scenarios](../reference/contract-scenarios.md) — the full lifecycle-timestamps specification
