# TTL-First Cache Table Patterns (FaceTheory ISR)

This document describes operationally safe patterns for TTL-based cache metadata tables used for ISR:

- how to choose a TTL attribute and window
- how to separate “freshness” from “expiry”
- how to account for DynamoDB TTL deletion lag
- operational recommendations to avoid surprises in production

Schema context: `docs/facetheory/isr-cache-schema.md`.

## Choose a TTL attribute

Recommended:

- Use a single numeric attribute named `ttl` that stores **epoch seconds**.
- Treat it as **garbage collection**, not as correctness logic.

Why:

- TTL is asynchronous and can delete later than the timestamp you set.
- A stable `ttl` attribute name keeps Go/TypeScript/Python models aligned.

## Freshness vs expiry (two clocks)

Use two separate concepts:

- **Freshness window** (correctness boundary):
  - `fresh_until = generated_at + revalidate_seconds`
- **Expiry / GC horizon** (storage boundary):
  - `ttl = generated_at + retention_seconds (+ safety_buffer)`

Rules of thumb:

- `revalidate_seconds` is usually **minutes to hours**.
- `retention_seconds` is usually **days to weeks** (enough for debuggability and rollback).
- `ttl` SHOULD be **much larger** than `revalidate_seconds`.

## DynamoDB TTL deletion lag

TTL deletion is best-effort; items can remain after their `ttl` passes.

Implications:

- ✅ Treat `ttl` as “eligible for deletion”, not “will be deleted immediately”.
- ✅ Readers MUST decide fresh/stale based on `generated_at` and `revalidate_seconds`.
- ❌ Never treat “item missing” as “item never existed”; it may have been deleted earlier or later than expected.

## Operational recommendations

- **Hot partitions**: avoid putting raw URL paths directly in `pk`; use a stable hash to reduce common-prefix hotspots.
- **Capacity**: for ISR tables, on-demand billing is often simplest; watch `ThrottledRequests` and adjust.
- **Alarms**: alert on read/write throttles, elevated `UserErrors`, and high latency; consider tracking
  `TimeToLiveDeletedItemCount` as a sanity signal (not a correctness signal).
- **Retention**: choose a retention long enough for investigations (and long enough to survive TTL lag), but not so long
  that storage cost grows unbounded.

## Example: write metadata with TTL, then read and decide fresh/stale

This example uses the `META` row pattern (`sk = "META"`).

### Write

1. Compute `generated_at = now_unix`.
2. Set `ttl = generated_at + retention_seconds`.
3. Store `revalidate_seconds` alongside the metadata.

### Read

1. Fetch the `META` item by primary key.
2. Compute `fresh_until = generated_at + revalidate_seconds`.
3. Consider it fresh iff `now_unix < fresh_until`.

### Go example

```go
nowUnix := time.Now().Unix()

meta := &FaceTheoryCacheMetadata{
	PK:                "TENANT#t1#CACHE#abc",
	SK:                "META",
	S3Key:             "pages/t1/abc.html",
	GeneratedAt:       nowUnix,
	RevalidateSeconds: 60,              // freshness window
	TTL:               nowUnix + 86400, // GC horizon (1 day)
}

if err := db.Model(meta).CreateOrUpdate(); err != nil {
	return err
}

var got FaceTheoryCacheMetadata
if err := db.Model(&FaceTheoryCacheMetadata{}).
	Where("PK", "=", meta.PK).
	Where("SK", "=", "META").
	First(&got); err != nil {
	return err
}

freshUntil := got.GeneratedAt + got.RevalidateSeconds
isFresh := time.Now().Unix() < freshUntil
```

