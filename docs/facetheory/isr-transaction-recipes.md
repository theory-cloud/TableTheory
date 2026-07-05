---
title: FaceTheory ISR Transaction Recipes (Metadata + Pointer Swap)
---

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
   - `Put` the metadata row (`META`) with the new pointer (`s3_key`), timestamps, etag, ttl
   - `Delete` the lease row (`LOCK`) with a condition expression (`lease_token` matches AND `lease_expires_at > now`)

Note: DynamoDB transactions cannot include multiple operations on the same item. Prefer a conditional `Delete`/`Update`
on the lease row over a separate `ConditionCheck` + `Delete` on the same lease key.

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

1. `Put` the new version row (`VER#...`) guarded with `attribute_not_exists(pk)` to avoid duplicate IDs.
2. `Update` the pointer row (`META`) to set `current_sk` to the new version (optionally guard with optimistic `version`).
3. `Delete` the lease row (`LOCK`) with a condition expression (token + not expired).

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
	tx.Put(metaItem)

	tx.Delete(
		leaseItem,
		tabletheory.Condition("lease_token", "=", leaseToken),
		tabletheory.Condition("lease_expires_at", ">", nowUnix),
	)
	return nil
})
```

### TypeScript (`TheorydbClient.transactWrite`)

```ts
await client.transactWrite([
  {
    kind: 'put',
    model: 'FaceTheoryCacheMetadata',
    item: { pk, sk: 'META', s3_key, generated_at: nowUnix, revalidate_seconds, etag, ttl },
  },
  {
    kind: 'delete',
    model: 'FaceTheoryCacheLease',
    key: { pk, sk: 'LOCK' },
    conditionExpression: '#tok = :tok AND #exp > :now',
    expressionAttributeNames: { '#tok': 'lease_token', '#exp': 'lease_expires_at' },
    expressionAttributeValues: {
      ':tok': { S: leaseToken },
      ':now': { N: String(nowUnix) },
    },
  },
]);
```

### TypeScript (TableTheory helper: `FaceTheoryIsrMetaStore`)

TableTheory exports a small helper that implements the FaceTheory ISR metadata + lease operations using:

- `LeaseManager` for acquiring/releasing `LOCK` rows
- `TheorydbClient.transactWrite()` for atomic “publish META + release LOCK” (Recipe A)

```ts
import { DynamoDBClient } from '@aws-sdk/client-dynamodb';
import { createFaceTheoryIsrMetaStore } from '@theory-cloud/tabletheory-ts/facetheory';

const ddb = new DynamoDBClient({ region: process.env.AWS_REGION ?? 'us-east-1' });
const isr = createFaceTheoryIsrMetaStore({
  ddb,
  tableName: process.env.FACETHEORY_CACHE_TABLE_NAME!,
});

const lease = await isr.tryAcquireLease({
  cacheKey,
  leaseOwner: 'my-app-instance', // optional (for app logs)
  leaseDurationMs: 30_000,
});
if (!lease) return; // another contender is regenerating

await isr.commitGeneration({
  cacheKey,
  leaseOwner: 'my-app-instance', // optional (for app logs)
  leaseToken: lease.leaseToken,
  htmlPointer: s3Key,
  generatedAtMs: Date.now(),
  revalidateSeconds: 60,
  etag,
});
```

### Python (`Table.transact_write`)

```py
table.transact_write(
    [
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
            condition_expression="#tok = :tok AND #exp > :now",
            expression_attribute_names={"#tok": "lease_token", "#exp": "lease_expires_at"},
            expression_attribute_values={":tok": lease_token, ":now": now_unix},
        ),
    ]
)
```
