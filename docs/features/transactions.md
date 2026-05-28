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
// Conditional write composed with peer writes inside a transaction.
err := db.Transaction(func(tx *core.Tx) error {
    if err := tx.Model(&fromAcct).Update("balance"); err != nil {
        return err
    }
    if err := tx.Model(&toAcct).Update("balance"); err != nil {
        return err
    }
    return tx.Model(&audit).Create()
})
```

> The transaction helper signature is `db.Transaction(func(tx *core.Tx) error) error`. See [`pkg/transaction/`](https://github.com/theory-cloud/tabletheory/tree/main/pkg/transaction) for the canonical Tx API.

## TypeScript

`TheorydbClient.transactWrite(actions: TransactAction[])` accepts a list of
`{ kind: 'put' | 'update' | 'delete' | 'conditionCheck', model, ... }` actions.

```typescript
await db.transactWrite([
  {
    kind: 'update',
    model: 'Account',
    item: { ...fromKey, balance: fromBal, version: fromVersion },
    fields: ['balance'],
  },
  {
    kind: 'update',
    model: 'Account',
    item: { ...toKey, balance: toBal, version: toVersion },
    fields: ['balance'],
  },
  {
    kind: 'put',
    model: 'Audit',
    item: auditItem,
    ifNotExists: true,
  },
]);
```

See [`ts/src/transaction.ts`](https://github.com/theory-cloud/tabletheory/blob/main/ts/src/transaction.ts) for the full `TransactAction` shape.

## Python

`Table.transact_write(actions)` accepts a list of dataclass actions —
`TransactPut`, `TransactUpdate`, `TransactDelete`, `TransactConditionCheck` — all importable from `theorydb_py`.

```python
from theorydb_py import TransactPut, TransactUpdate

table.transact_write([
    TransactUpdate(
        pk="ACCOUNT#A", sk="v1",
        updates={"balance": from_bal},
    ),
    TransactUpdate(
        pk="ACCOUNT#B", sk="v1",
        updates={"balance": to_bal},
    ),
    TransactPut(item=audit_item),
])
```

See [`py/src/theorydb_py/transaction.py`](https://github.com/theory-cloud/tabletheory/blob/main/py/src/theorydb_py/transaction.py) for the full action-dataclass shapes.

## Common patterns

- **Transferring state between two items**: update both with conditional expressions.
- **Atomic create-if-absent of multiple items**: create each with `attribute_not_exists(pk)`.
- **Composite invariants across a parent and its children**: condition-check the parent's version while writing children atomically.

## Anti-patterns

- **Don't simulate transactions with successive single writes.** DynamoDB transactions exist for exactly this reason; rolling your own loses atomicity.
- **Don't catch the abort error and retry without recomputing.** A condition failure means your snapshot is stale; reload state before the next attempt.
- **Don't put the same item twice in one transaction.** DynamoDB rejects it. Combine the changes locally first.

## Related

- [Optimistic Locking](optimistic-locking.md) — versioned writes inside and outside transactions
- [Core Patterns](../core-patterns.md) — end-to-end transaction recipes
- [FaceTheory · ISR Transaction Recipes](../facetheory/isr-transaction-recipes.md) — production transactions used by the ISR cache layer
