---
title: Transactions
description: Real DynamoDB TransactWriteItems — composed with the rest of the contract.
---

# Transactions

This page documents TableTheory's public write-transaction surfaces, which use the actual DynamoDB `TransactWriteItems` API. There is no app-level lock, no optimistic-concurrency-over-HTTP simulation, and no hidden retry sleep loop.

## What a transaction guarantees

- **Atomicity** across all items in the group: either every write applies or none.
- **Condition evaluation** is server-side: each write item carries its own conditional expression, and a single failed condition aborts the whole transaction.
- **Optimistic-lock composition**: a versioned item in the group asserts its expected version; a version mismatch aborts the transaction atomically.
- **Encryption composition**: encrypted fields are encrypted before the transaction is submitted; a KMS failure aborts before any write hits DynamoDB.
- **Marshaling parity**: transaction puts preserve non-nil empty lists and maps, including inside nested Go structs, unless the field explicitly uses `omitempty`.
- **Cross-runtime parity**: a write transaction submitted from Python sees the same atomicity guarantees as one submitted from Go.

## Write transaction limits to know

| Limit                                             | Value / behavior                            |
|---------------------------------------------------|---------------------------------------------|
| DynamoDB `TransactWriteItems` item limit           | 100 items per service call                  |
| Go `core.TransactionBuilder` operation cap         | 100 operations                              |
| TypeScript `TheorydbClient.transactWrite` cap      | DynamoDB enforces service-call limits       |
| Python `Table.transact_write` action cap           | 100 actions                                 |
| Maximum DynamoDB transaction payload size          | 4 MB                                        |
| Items addressed across multiple tables             | allowed by DynamoDB                         |
| Same item appearing twice in one call              | not allowed by DynamoDB                     |

TableTheory **does not auto-chunk** write transactions across multiple
`TransactWriteItems` calls — that would silently break atomicity. If you exceed
the runtime guard or DynamoDB service limit, redesign the access pattern instead
of splitting one logical transaction into multiple calls.

## Go

```go
// Conditional writes composed with a peer create inside one TransactWriteItems call.
err := db.TransactWrite(ctx, func(tx core.TransactionBuilder) error {
    tx.UpdateWithBuilder(&fromAcct, func(u core.UpdateBuilder) error {
        u.Set("Balance", fromBal)
        u.Add("Version", int64(1))
        u.ConditionVersion(fromVersion)
        return nil
    })

    tx.UpdateWithBuilder(&toAcct, func(u core.UpdateBuilder) error {
        u.Set("Balance", toBal)
        u.Add("Version", int64(1))
        u.ConditionVersion(toVersion)
        return nil
    })

    tx.Create(&audit)
    return nil
})
```

> Use `db.TransactWrite(ctx, func(core.TransactionBuilder) error)` or the
> fluent `db.Transact()` builder followed by `Execute()` for DynamoDB
> transactions. In v2, the removed `db.Transaction(func(*core.Tx) error)` and
> `db.TransactionFunc(...)` compatibility helpers no longer exist; migrate to
> the atomic `Transact()` surface.

### Legacy one-argument `Transaction.Update(model)`

The legacy `(*transaction.Transaction).Update(model)` path is a whole-model,
implicit-selection surface. As of v3.0.1, its implicit candidate list excludes
the five library-managed fields: `PK`, `SK`, `created_at`, `updated_at`, and
`version`. It silently ignores caller-set lifecycle values: `created_at` is not
assigned, and `updated_at` is assigned only by the library.

Managed assignments are appended after implicit selection. An `updated_at`
field always receives the library timestamp, and a non-zero `version` adds both
an expected-version condition and the managed increment. A zero version adds
neither. This differs from v3.0.0 and every v2.x release, where a non-empty
caller-set `updated_at` was preserved instead of refreshed.

When no caller field is selected and no managed assignment qualifies,
`Update(model)` returns `no non-key fields to update` and does not queue a
write. Earlier releases could commit an all-managed-field model by silently
writing its caller-set `created_at`, which could clobber the stored creation
timestamp. Account for both changes in deterministic-timestamp tests and
backfills.

The legacy implicit path does not reject caller-populated lifecycle fields;
it ignores them. Rejection of `created_at`, `updated_at`, and version is limited
to the explicit named-field transaction surfaces in Go, TypeScript, and Python.
Use a deliberate caller-controlled low-level surface rather than the legacy
whole-model path when a backfill must set a historical lifecycle value.

Go model-based `Update(model, fields)` transactions reject explicit selection
of the library-owned `created_at`, `updated_at`, and version fields with
`ErrInvalidModel`. The transaction runtime remains the single writer of
`updated_at`, so the generated expression cannot contain overlapping lifecycle
document paths. `UpdateWithBuilder` remains a caller-controlled low-level
surface. The model-based path does not increment version or add a version
condition; use `UpdateWithBuilder` for optimistic locking in a transaction.

## TypeScript

`TheorydbClient.transactWrite(actions: TransactAction[])` accepts a list of
`{ kind: 'put' | 'update' | 'delete' | 'condition', model, ... }` actions.
Update actions provide either `item` plus an explicit `fields` selection, or
`key` plus a raw `updateExpression` or an `updateFn` that uses the
`UpdateBuilder` DSL. The model-based `item` + `fields` action removes a selected
empty `omit_empty` attribute, sets every other selected attribute, and advances
the model's library-owned `updatedAt` role. It rejects `createdAt` and version
in `fields`. It also rejects caller-selected `updatedAt`, leaving the injected
lifecycle refresh as the single writer. The builder and raw expression variants
remain caller-controlled.

```typescript
await db.transactWrite([
  {
    kind: 'update',
    model: 'Account',
    key: fromKey,
    updateFn: (u) => {
      u.set('balance', fromBal).add('version', 1).conditionVersion(fromVersion);
    },
  },
  {
    kind: 'update',
    model: 'Account',
    key: toKey,
    updateFn: (u) => {
      u.set('balance', toBal).add('version', 1).conditionVersion(toVersion);
    },
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
`TransactPut`, `TransactUpdate`, `TransactDelete`, `TransactConditionCheck` — all importable from `tabletheory_py`.
Model-shaped `TransactUpdate.updates` rejects the Python fields carrying
`created_at`, `updated_at`, and version roles with `ValidationError`. It does not
increment version or add a version condition. Python has no partial-update
version path on the transactional surface; transactional optimistic locking
requires a `TransactPut` of the item with an explicitly incremented version plus
a `condition_expression` on the current version.

```python
from tabletheory_py import TransactPut, TransactUpdate

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

See [`py/src/tabletheory_py/transaction.py`](https://github.com/theory-cloud/tabletheory/blob/main/py/src/tabletheory_py/transaction.py) for the full action-dataclass shapes.

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
