---
title: Optimistic Locking
description: Version-conditional writes — populated on read, incremented on write, guarded across all three runtimes.
---

# Optimistic Locking

The `version` field is TableTheory's optimistic concurrency control. The semantics are simple and pinned by the [optimistic-locking P0 scenario](https://github.com/theory-cloud/tabletheory/tree/main/contract-tests/scenarios):

1. A field tagged `theorydb:"version"` is **populated on read**.
2. On write, the runtime emits a conditional expression: `version = :expected_version` where `:expected_version` is the value loaded on the most recent read.
3. On a successful write, the runtime **increments the field** in the persisted item.
4. On a version mismatch (another writer raced ahead), the runtime returns a **typed conflict error** — never a silent overwrite.

## Why this matters

Every Theory Cloud consumer that ships versioned writes — AppTheory's idempotency state, Autheory's session refreshes, theory-mcp-server's agent memory cursor — depends on this behavior being identical across runtimes. A Go writer racing a Python writer must see one of them lose deterministically.

## Go

```go
type Counter struct {
    PK      string `theorydb:"pk"      json:"pk"`
    SK      string `theorydb:"sk"      json:"sk"`
    Value   int64  `json:"value"`
    Version int64  `theorydb:"version" json:"version"`
}

var c Counter
if err := db.Get(ctx, &c, "tenant#42", "counter#impressions"); err != nil {
    return err
}
c.Value++
if err := db.Put(ctx, &c); err != nil {
    if tabletheory.IsVersionConflict(err) {
        // Reload, recompute, retry.
        return retry()
    }
    return err
}
```

## TypeScript

```typescript
const c = await db.get(Counter, "tenant#42", "counter#impressions");
c.value += 1;

try {
  await db.put(c);
} catch (err) {
  if (TableTheory.isVersionConflict(err)) {
    return retry();
  }
  throw err;
}
```

## Python

```python
c = db.get(Counter, "tenant#42", "counter#impressions")
c.value += 1

try:
    db.put(c)
except TableTheory.VersionConflict:
    return retry()
```

## Versioning a newly-created item

The first `Put` of an item that does not yet exist starts at `version = 1`. Subsequent writes increment. A `Put` that asserts `version = 0` against a non-existent item is the contract-defined "create if absent" form.

## Versioning under transactions

Optimistic locking composes with DynamoDB transactions. A transactional write group that includes a versioned item asserts the version on the item's condition; if any condition fails, the whole transaction fails atomically. See [Transactions](../transactions/).

## Anti-patterns

- **Don't read with one runtime, write with another, and modify the version field manually in between.** TableTheory is the only thing that should mutate `version`.
- **Don't catch the conflict error and retry indefinitely.** Bound the retry loop. The contract guarantees deterministic loss, not deterministic eventual success.
- **Don't omit the `version` field from your model and expect "best effort" concurrency.** No `version` tag means no locking — the field is required to opt in.

## Related

- [Lifecycle Timestamps](../lifecycle-timestamps/) — automatic `updated_at` populated on every versioned write
- [Transactions](../transactions/) — composing versioned writes into atomic groups
- [Contract Scenarios](../../reference/contract-scenarios/) — the full optimistic-locking specification
