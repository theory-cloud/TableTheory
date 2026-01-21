# TableTheory: FaceTheory Enablement Roadmap (ISR Locks + Cache Metadata)

This roadmap captures what TableTheory should provide so **FaceTheory** can implement **SSG + ISR** on AWS with
correctness-first semantics and without per-app “glue code” re-implementations.

Source document: `FaceTheory/docs/WISHLIST.md` (TableTheory wishlist section).

## Baseline (current)

As of TableTheory `v1.1.5`:

- TableTheory has strong DynamoDB primitives (conditional writes, transactions) and a contract-test posture.
- TTL encoding is covered by the contract suite (`ttl` as epoch seconds) and is safe to build cache tables around.
- Documentation already contains **idempotency/locking patterns** (see `docs/cdk/README.md`), but these patterns are not
  yet packaged as small, canonical, multi-language helpers.

## Goals

- Provide a **canonical** and **portable** implementation for ISR regeneration locks (lease/lock).
- Provide safe “transaction recipes” for cache metadata updates (pointer swap, freshness, etag) that apps can copy or call.
- Provide TTL-first cache schema guidance so teams don’t invent incompatible models.
- Keep TableTheory’s supply-chain posture: **GitHub releases only** for TS/Py assets; no registry tokens.

## Non-goals (initially)

- A full “FaceTheory cache framework” inside TableTheory (FaceTheory owns ISR policies and rendering orchestration).
- Storing full HTML in DynamoDB (FaceTheory stores bodies in S3; DynamoDB is metadata + locks only).

## Milestones

### FT-T0 — ISR cache schema guidance (docs + recommended model shapes)

**Goal:** standardize the DynamoDB model shape used for ISR cache metadata and regeneration locks.

**Acceptance criteria**
- A doc exists describing recommended roles for a “FaceTheory cache metadata” table:
  - key design (cache key partitioning, tenant partitioning guidance)
  - metadata attributes (S3 object key pointer, `generated_at`, `revalidate_seconds`, `etag`, etc)
  - TTL strategy and pitfalls (TTL lag, safety buffer)
- The doc includes at least one runnable model definition per language (Go struct, TS type, Py dataclass/pydantic) showing:
  - canonical tags/roles (`pk`, optional `sk`, `ttl`)
  - attribute naming conventions
- Guidance explicitly calls out where transactions are required vs optional.

---

### FT-T1 — Lease/lock helper (canonical, multi-language)

**Goal:** ship a small helper for distributed regeneration locks with safe defaults and predictable failure modes.

**Wishlist mapping:** “Lease/lock helper patterns” (FaceTheory P0).

**Acceptance criteria**
- Go/TS/Py each expose a small, focused lease API (names TBD) that supports:
  - acquire via conditional write with lease expiry
  - optional refresh/extend
  - best-effort release
- The helper is safe-by-default:
  - lease ownership token required for refresh/release
  - time/clock source is injectable for deterministic tests
  - failures are typed enough for callers to distinguish “lock held” vs “unexpected error”
- Tests exist:
  - unit tests for condition-expression correctness and edge cases
  - DynamoDB Local integration test proving two contenders behave correctly

---

### FT-T2 — Transactional update recipes (metadata + pointer swap)

**Goal:** make it easy to implement “update metadata + update pointer” safely under concurrent regeneration.

**Wishlist mapping:** “Transactional update recipes” (FaceTheory P0).

**Acceptance criteria**
- A doc (and optionally helper functions) exists that covers:
  - transactional update for (lock held) → write new metadata pointer → release
  - guarding against stale writers (lease token check, version check, or conditional expressions)
  - etag update patterns
- If helpers are added, they exist in Go/TS/Py with equivalent semantics and tests.

---

### FT-T3 — TTL-first cache table patterns (high-leverage docs + examples)

**Goal:** reduce schema drift and operational pitfalls for TTL-based cache metadata tables.

**Wishlist mapping:** “TTL-first cache schema guidance” (FaceTheory P1).

**Acceptance criteria**
- Guidance covers:
  - selecting TTL attribute and TTL window strategies
  - how to represent “freshness” vs “expiry”
  - dealing with DynamoDB TTL deletion lag
  - operational recommendations (alarms, hot partitions, sizing)
- At least one example exists demonstrating:
  - writing metadata with TTL
  - reading and deciding fresh/stale based on `generated_at` and `revalidate_seconds`

---

### FT-T4 — Idempotency patterns (request-id driven regeneration)

**Goal:** standardize “exactly-once-ish” regeneration and async revalidation triggers.

**Wishlist mapping:** “Idempotency patterns” (FaceTheory P1).

**Acceptance criteria**
- A doc exists that maps regeneration operations to idempotency keys (request ID, cache key + version, etc).
- The doc includes guidance for:
  - replay safety (same request key, different payload)
  - lock + idempotency interplay (avoid double work)
- If helpers are added, they build on existing TableTheory patterns and are multi-language with tests.

