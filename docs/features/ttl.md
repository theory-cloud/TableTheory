---
title: TTL
description: DynamoDB TimeToLive — set per-item, honored on reads, identical across all three runtimes.
---

# TTL

The `ttl` role marks an attribute as the DynamoDB TimeToLive value. The [TTL P0 scenario](https://github.com/theory-cloud/tabletheory/tree/main/contract-tests/scenarios) pins these guarantees:

- The tagged field's value (epoch seconds) becomes the item's `TimeToLive` attribute.
- Expired items return a typed not-found error on read, never an item with stale data.
- The TTL attribute name matches the DMS-declared shape.
- Cross-runtime: an item written with TTL by Go is honored by Python and TypeScript reads identically.

## Unit: epoch seconds

DynamoDB's TTL attribute is **epoch seconds**, not milliseconds. This is the one place TableTheory's clock unit may differ from your application's wall clock — be careful at the boundary.

## Go

```go
type Session struct {
    PK        string `theorydb:"pk"  json:"pk"`
    SK        string `theorydb:"sk"  json:"sk"`
    UserID    string `json:"user_id"`
    ExpiresAt int64  `theorydb:"ttl" json:"expires_at"` // epoch seconds
}

s := &Session{
    PK:        "TENANT#42",
    SK:        "SESSION#abc",
    UserID:    "USER#1",
    ExpiresAt: time.Now().Add(15 * time.Minute).Unix(),
}
db.Model(s).Create()
```

## TypeScript

```typescript
const Session = defineModel({
  name: 'Session',
  table: { name: 'sessions_contract' },
  keys: {
    partition: { attribute: 'PK', type: 'S' },
    sort:      { attribute: 'SK', type: 'S' },
  },
  attributes: [
    { attribute: 'PK',        type: 'S', roles: ['pk'] },
    { attribute: 'SK',        type: 'S', roles: ['sk'] },
    { attribute: 'userId',    type: 'S' },
    { attribute: 'expiresAt', type: 'N', roles: ['ttl'] }, // epoch seconds
  ],
});
```

## Python

```python
@dataclass(frozen=True)
class Session:
    pk:         str = theorydb_field(roles=["pk"])
    sk:         str = theorydb_field(roles=["sk"])
    user_id:    str = theorydb_field()
    expires_at: int = theorydb_field(roles=["ttl"])  # epoch seconds
```

## DynamoDB's TTL behavior

DynamoDB does not delete TTL-expired items immediately — its sweep is **eventually consistent**, often within 48 hours. TableTheory **does not paper over this** at the runtime level; it instead enforces the more important guarantee: that an expired item, even if still physically present in the table, **does not return data on a read**. Queries and scans may still observe physical expired items briefly; consumers that need strict expiration semantics should layer that in application code.

This conservative posture is part of the design — TableTheory does not hide DynamoDB's actual behavior, it just ensures consumers don't accidentally use stale data.

## Anti-patterns

- **Don't use milliseconds in the TTL field.** DynamoDB will treat them as far-future values and never expire.
- **Don't rely on instant deletion.** TTL is eventually consistent at the DynamoDB layer.
- **Don't combine `ttl` with `omit_empty` for a field that may legitimately be unset.** An expired-or-missing TTL value disables the expiration semantics — that's a separate access-pattern problem.

## Related

- [Lifecycle Timestamps](lifecycle-timestamps.md) — separate axis governed by its own roles
- [FaceTheory · TTL Cache Patterns](../facetheory/ttl-cache-patterns.md) — how FaceTheory uses TTL for ISR cache eviction
- [Contract Scenarios](../reference/contract-scenarios.md) — the full TTL specification
