---
title: Lifecycle Timestamps
description: Automatic created_at and updated_at on every write — clock-ordered across all three runtimes.
---

# Lifecycle Timestamps

`created_at` and `updated_at` are the canonical lifecycle timestamp roles. The runtime owns them — your code never sets them by hand.

The [lifecycle-timestamps P0 scenario](https://github.com/theory-cloud/tabletheory/tree/main/contract-tests/scenarios) verifies that all three runtimes:

- Set `created_at` on the **first write only**; subsequent writes leave it intact.
- Update `updated_at` on **every write** (including the first).
- Use a consistent clock source and unit across runtimes.
- Preserve clock ordering: a write that follows another write produces an `updated_at` value strictly greater than the predecessor's.

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
// n.CreatedAt and n.UpdatedAt are now populated by the runtime.
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
```

## Python

```python
@dataclass(frozen=True)
class Note:
    pk:         str = theorydb_field(roles=["pk"])
    sk:         str = theorydb_field(roles=["sk"])
    body:       str = theorydb_field()
    created_at: str = theorydb_field(roles=["created_at"])
    updated_at: str = theorydb_field(roles=["updated_at"])
```

## Composes with optimistic locking

`updated_at` is populated alongside the `version` role on every successful write. A version conflict aborts the write — neither version nor `updated_at` advances. This guarantees `updated_at` is monotonic for any item that ever loaded successfully.

## Composes with TTL

`updated_at` does **not** influence the TTL attribute. TTL is computed from the field with the `ttl` role, independent of when the item was last touched. If you want "extend TTL on every write," update the TTL field explicitly in your write path.

## Anti-patterns

- **Don't set `created_at` or `updated_at` in your application code.** The runtime owns them; manual values are overwritten or rejected per the contract.
- **Don't rely on `created_at` for tie-breaking equal-version writes.** Use the version role; that's what it's for.

## Related

- [Optimistic Locking](optimistic-locking.md) — the `version` field that composes with these timestamps
- [TTL](ttl.md) — separate timestamp axis governed by its own role
- [Contract Scenarios](../reference/contract-scenarios.md) — the full lifecycle-timestamps specification
