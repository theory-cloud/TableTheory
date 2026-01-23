# FaceTheory ISR Transaction Recipes (Metadata + Pointer Swap)

This document describes **correctness-first** transaction patterns for ISR regeneration when using:

- a **metadata item** (`sk = "META"`) and
- a **lease item** (`sk = "LOCK"`)

as defined in `docs/facetheory/isr-cache-schema.md`.

The goal is to ensure:

- only the current lock holder can publish new metadata
- stale writers (expired lock, stolen lock) cannot overwrite newer state
- releasing the lock is tied to publishing metadata (where appropriate)

## Recipe A: In-place metadata publish (single `META` row)

Use this when you only need one metadata record per cache key.

### Steps

1. Acquire the lease (`LOCK`) for `pk`.
2. Regenerate the body and write it to S3.
3. Publish metadata and release the lock with a single DynamoDB transaction:
   - `ConditionCheck` the lease row (`lease_token` matches AND `lease_expires_at > now`)
   - `Put` the metadata row (`META`) with the new pointer (`s3_key`), timestamps, etag, ttl
   - `Delete` the lease row (optionally conditioned on `lease_token`)

### Why the transaction matters

Without the transaction, a writer can:

- finish regeneration after its lease expires, and
- overwrite the metadata even though another contender acquired the lease.

The transaction makes publishing contingent on still owning the lease.

## Recipe B: Pointer swap (versioned metadata rows)

Use this when you want history, safer rollbacks, or deduping/observability across generations.

Recommended item roles (same `pk`):

- pointer row: `sk = "META"` with `current_sk = "VER#<id>"`
- version rows: `sk = "VER#<id>"` containing `s3_key`, `generated_at`, `etag`, etc.

Transaction sketch:

1. `ConditionCheck` the lease row (token + not expired).
2. `Put` the new version row (`VER#...`) guarded with `attribute_not_exists(pk)` to avoid duplicate IDs.
3. `Update` the pointer row (`META`) to set `current_sk` to the new version (optionally guard with optimistic `version`).
4. `Delete` the lease row (optionally conditioned on token).

## Stale-writer protection options

You can guard against stale writers in one (or multiple) of these ways:

- **Lease token check (recommended)**: the lease row stores `lease_token` and transactions require it.
- **Lease expiry check**: require `lease_expires_at > now` in the transaction.
- **Optimistic version check**: add a `version` field to the pointer row and require it matches before updating.
- **Monotonic timestamp check**: conditionally update only if `generated_at < :new_generated_at`.

## ETag update patterns

Recommended: publish `etag` alongside the pointer update.

- In Recipe A, store `etag` directly on `META` and overwrite it atomically in the transaction.
- In Recipe B, store `etag` on the version row and optionally denormalize the current `etag` onto the pointer row for
  faster reads.

Stale-writer guard: if you rely on monotonicity, require `generated_at < :new_generated_at` before setting a new `etag`.

## Multi-language examples

These examples focus on the **transaction shape**; model definitions are in `docs/facetheory/isr-cache-schema.md`.

### Go (TableTheory transaction builder)

```go
ctx := context.Background()
nowUnix := time.Now().Unix()

leaseItem := &models.FaceTheoryCacheLease{
	PK: "TENANT#t1#CACHE#abc",
	SK: "LOCK",
}

metaItem := &models.FaceTheoryCacheMetadata{
	PK:                leaseItem.PK,
	SK:                "META",
	S3Key:             "s3://bucket/key.html",
	GeneratedAt:       nowUnix,
	RevalidateSeconds: 60,
	ETag:              "\"abc123\"",
	TTL:               nowUnix + 86400,
}

err := db.TransactWrite(ctx, func(tx core.TransactionBuilder) error {
	tx.ConditionCheck(
		leaseItem,
		tabletheory.Condition("lease_token", "=", leaseToken),
		tabletheory.Condition("lease_expires_at", ">", nowUnix),
	)

	tx.Put(metaItem)

	// Optional: make the delete conditional on the same token.
	tx.Delete(leaseItem, tabletheory.Condition("lease_token", "=", leaseToken))
	return nil
})
```

### TypeScript (`TheorydbClient.transactWrite`)

```ts
await client.transactWrite([
  {
    kind: 'condition',
    model: 'FaceTheoryCacheLease',
    key: { pk, sk: 'LOCK' },
    conditionExpression: '#tok = :tok AND #exp > :now',
    expressionAttributeNames: { '#tok': 'lease_token', '#exp': 'lease_expires_at' },
    expressionAttributeValues: {
      ':tok': { S: leaseToken },
      ':now': { N: String(nowUnix) },
    },
  },
  {
    kind: 'put',
    model: 'FaceTheoryCacheMetadata',
    item: { pk, sk: 'META', s3_key, generated_at: nowUnix, revalidate_seconds, etag, ttl },
  },
  {
    kind: 'delete',
    model: 'FaceTheoryCacheLease',
    key: { pk, sk: 'LOCK' },
  },
]);
```

### Python (`Table.transact_write`)

```py
table.transact_write(
    [
        TransactConditionCheck(
            pk=pk,
            sk="LOCK",
            condition_expression="#tok = :tok AND #exp > :now",
            expression_attribute_names={"#tok": "lease_token", "#exp": "lease_expires_at"},
            expression_attribute_values={":tok": lease_token, ":now": now_unix},
        ),
        TransactPut(
            item=FaceTheoryCacheMetadata(
                pk=pk,
                sk="META",
                s3_key=s3_key,
                generated_at=now_unix,
                revalidate_seconds=revalidate_seconds,
                etag=etag,
                ttl=ttl,
            ),
        ),
        TransactDelete(
            pk=pk,
            sk="LOCK",
        ),
    ]
)
```

