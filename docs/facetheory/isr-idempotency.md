# FaceTheory ISR Idempotency Patterns (Request-ID Driven Regeneration)

This document describes “exactly-once-ish” regeneration patterns for ISR by combining:

- an ISR lease/lock (`sk = "LOCK"`) and
- a request-scoped idempotency record (`sk = "REQ#<idempotency_key>"`)

The goal is to prevent double work (and stale overwrites) when requests, queues, or retries re-trigger regeneration.

## Choose an idempotency key

Your idempotency key MUST uniquely represent one regeneration intent.

Common choices:

- **Request ID**: `idempotency_key = <x-request-id>` (HTTP-triggered)
- **Queue message ID**: `idempotency_key = <sqs_message_id>` (async-triggered)
- **Cache key + version**: `idempotency_key = hash(<cache_key>|<deploy_id>|<policy_version>)`

Recommendation: include a deployment identifier or policy version when ISR behavior changes (so old retries don’t collide
with new semantics).

## Model the idempotency record

Recommended attributes on `REQ#...` items:

- `request_hash` (string): hash of the inputs that matter (tenant, cache key, policy knobs).
- `status` (string): `STARTED` / `COMPLETED` / `FAILED`.
- `result_s3_key` (string, optional): pointer to the generated output.
- `ttl` (epoch seconds): GC horizon for idempotency records.

## Replay safety: same key, different payload

If the same `idempotency_key` is replayed with different inputs, you MUST treat it as an error.

Pattern:

1. Compute `request_hash` deterministically from the important inputs.
2. On first attempt, create the idempotency record with `request_hash`.
3. On retries, read the record and verify the stored `request_hash` matches:
   - if it matches and status is `COMPLETED`, return the recorded result
   - if it matches and status is `STARTED`, avoid duplicate work (either wait or serve stale)
   - if it does not match, fail closed (inputs are inconsistent)

## Lock + idempotency interplay (avoid double work)

Idempotency records prevent duplicate work across retries; the lock prevents concurrent regeneration for a cache key.

A safe ordering is:

1. Read `META`:
   - if fresh, serve and return (no lock, no idempotency needed)
2. Create (or read) the idempotency record:
   - `PutItem` guarded by `attribute_not_exists(pk)` on `REQ#...`
   - if it already exists:
     - if `COMPLETED`, short-circuit and return the recorded result
     - if `STARTED`, short-circuit to “already in progress”
3. Acquire the lease (`LOCK`).
4. Regenerate and write body to S3.
5. Publish `META` and finalize idempotency in one transaction:
   - `ConditionCheck` the lease token (still owned + not expired)
   - `Put/Update` `META` (new pointer, etag, generated_at)
   - `Update` the idempotency record to `COMPLETED` with `result_s3_key`
   - `Delete` the lease (best-effort)

This prevents:

- two workers doing the same regeneration work (idempotency record)
- a stale worker overwriting newer metadata (lease token checks in the publish transaction)

## Notes

- Always TTL idempotency records; they are coordination state, not permanent history.
- Keep idempotency rows in the same table/partition as the cache key when possible (simpler transactions).
- If your “in progress” behavior is to serve stale content, do so without waiting on the lock holder.

