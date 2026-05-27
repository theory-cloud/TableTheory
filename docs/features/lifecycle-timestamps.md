---
title: Lifecycle Timestamps
description: Automatic created_at and updated_at on every write — clock-ordered across all three runtimes.
---

# Lifecycle Timestamps

`theorydb:"created_at"` and `theorydb:"updated_at"` are the canonical lifecycle timestamp tags. The runtime owns them — your code never sets them by hand.

The [lifecycle-timestamps P0 scenario](https://github.com/theory-cloud/tabletheory/tree/main/contract-tests/scenarios) verifies that all three runtimes:

- Set `created_at` on the **first write only**; subsequent writes leave it intact.
- Update `updated_at` on **every write** (including the first).
- Use the same clock source and same unit (epoch milliseconds, UTC).
- Preserve clock ordering: a write that follows another write produces an `updated_at` value strictly greater than the predecessor's.

## Go

```go
type Note struct {
    PK        string `theorydb:"pk"          json:"pk"`
    SK        string `theorydb:"sk"          json:"sk"`
    Body      string `json:"body"`

    CreatedAt int64  `theorydb:"created_at"  json:"created_at"`
    UpdatedAt int64  `theorydb:"updated_at"  json:"updated_at"`
}

n := &Note{PK: "user#1", SK: "note#welcome", Body: "Hi."}
_ = db.Put(ctx, n)
// n.CreatedAt and n.UpdatedAt are now populated by the runtime.
```

## TypeScript

```typescript
@model({ naming: "snake_case", table: "notes" })
class Note {
  @field({ role: "pk" }) pk!: string;
  @field({ role: "sk" }) sk!: string;
  @field() body!: string;

  @field({ role: "created_at" }) created_at!: number;
  @field({ role: "updated_at" }) updated_at!: number;
}
```

## Python

```python
@model(naming="snake_case", table="notes")
class Note:
    pk: str
    sk: str
    body: str

    created_at: int = field(role="created_at")
    updated_at: int = field(role="updated_at")
```

## Composes with optimistic locking

`updated_at` is populated alongside `version` on every successful write. A version conflict aborts the write — neither `version` nor `updated_at` advances. This guarantees `updated_at` is monotonic for any item that ever loaded successfully.

## Composes with TTL

`updated_at` does **not** influence the TTL attribute. TTL is computed from the field tagged `theorydb:"ttl"`, independent of when the item was last touched. If you want "extend TTL on every write," update the TTL field explicitly in your write path.

## Anti-patterns

- **Don't set `created_at` or `updated_at` in your application code.** The runtime overwrites them; manual values are silently lost.
- **Don't store these as strings.** The tag implies epoch-millisecond integers across all runtimes.
- **Don't rely on `created_at` for tie-breaking equal-version writes.** Use the version field; that's what it's for.

## Related

- [Optimistic Locking](optimistic-locking.md) — the `version` field that composes with these timestamps
- [TTL](ttl.md) — separate timestamp axis governed by its own tag
- [Contract Scenarios](../reference/contract-scenarios.md) — the full lifecycle-timestamps specification
