---
title: Transactions
description: Real DynamoDB TransactWriteItems / TransactGetItems — composed with the rest of the contract.
---

# Transactions

TableTheory transactions use the actual DynamoDB transaction APIs — `TransactWriteItems` and `TransactGetItems`. There is no app-level lock, no optimistic-concurrency-over-HTTP simulation, and no hidden retry sleep loop.

## What a transaction guarantees

- **Atomicity** across all items in the group: either every write applies or none.
- **Condition evaluation** is server-side: each write item carries its own conditional expression, and a single failed condition aborts the whole transaction.
- **Optimistic-lock composition**: a versioned item in the group asserts its expected version; a version mismatch aborts the transaction atomically.
- **Encryption composition**: encrypted fields are encrypted before the transaction is submitted; a KMS failure aborts before any write hits DynamoDB.
- **Cross-runtime parity**: a transaction submitted from Python sees the same atomicity guarantees as one submitted from Go.

## DynamoDB transaction limits to know

| Limit                                     | Value         |
|-------------------------------------------|---------------|
| Items per `TransactWriteItems`            | 100           |
| Items per `TransactGetItems`              | 100           |
| Maximum total payload size                | 4 MB          |
| Items addressed across multiple tables    | allowed       |
| Same item appearing twice in one call     | not allowed   |

TableTheory **does not auto-chunk** transactions across multiple `TransactWriteItems` calls — that would silently break atomicity. If you exceed the 100-item or 4 MB limit, the runtime returns a typed error and you redesign the access pattern.

## Go

```go
err := db.Transaction(ctx, func(tx *tabletheory.Tx) error {
    if err := tx.Put(&from); err != nil { return err }
    if err := tx.Put(&to);   err != nil { return err }
    return tx.ConditionCheck(&audit, "version = :v", map[string]any{":v": expectedVersion})
})
if tabletheory.IsTransactionAborted(err) {
    // One of the items had a condition failure; reload and retry.
}
```

## TypeScript

```typescript
await db.transaction(async (tx) => {
  await tx.put(from);
  await tx.put(to);
  await tx.conditionCheck(audit, "version = :v", { ":v": expectedVersion });
});
```

## Python

```python
with db.transaction() as tx:
    tx.put(from_acct)
    tx.put(to_acct)
    tx.condition_check(audit, "version = :v", {":v": expected_version})
```

## Common patterns

- **Transferring state between two items**: put both with conditional expressions.
- **Atomic create-if-absent of multiple items**: put each with `attribute_not_exists(pk)`.
- **Composite invariants across a parent and its children**: condition-check the parent's version while writing children atomically.

## Anti-patterns

- **Don't simulate transactions with successive single writes.** DynamoDB transactions exist for exactly this reason; rolling your own loses atomicity.
- **Don't catch the abort error and retry without recomputing.** A condition failure means your snapshot is stale; reload state before the next attempt.
- **Don't put the same item twice in one transaction.** DynamoDB rejects it. Combine the changes locally first.

## Related

- [Optimistic Locking](../optimistic-locking/) — versioned writes inside and outside transactions
- [Core Patterns](../../core-patterns/) — end-to-end transaction recipes
- [FaceTheory · ISR Transaction Recipes](../../facetheory/isr-transaction-recipes/) — production transactions used by the ISR cache layer
