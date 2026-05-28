---
title: TTL
description: DynamoDB TimeToLive attributes — persisted as epoch seconds; deletion remains DynamoDB-eventual.
---

# TTL

The `ttl` role marks the attribute that DynamoDB Time To Live should use for
item expiration. The [TTL P0 scenario](https://github.com/theory-cloud/tabletheory/blob/main/contract-tests/scenarios/p0/05-ttl-epoch-seconds.yml)
pins the portable persistence contract:

- The tagged field's value is stored as a numeric DynamoDB attribute.
- The unit is **epoch seconds**, not milliseconds.
- The TTL attribute name matches the DMS-declared shape.
- A value written by one runtime is read back with the same value and DynamoDB
  number type by the other runtimes.

TableTheory does **not** currently mask physically present expired items at read
time. If DynamoDB has not yet swept the item, `get` / `First` / `Table.get` can
still return it.

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

DynamoDB does not delete TTL-expired items immediately — its sweep is
**eventually consistent**, often within 48 hours. TableTheory configures and
persists the TTL attribute, but the current read paths do not add an expiration
filter and do not convert still-present expired items into typed not-found
errors.

Consumers that need strict read-time expiration must include that check in their
access pattern, for example by comparing the TTL field to the current epoch
seconds after reading or by adding an explicit condition/filter where the access
pattern allows it.

## Anti-patterns

- **Don't use milliseconds in the TTL field.** DynamoDB will treat them as far-future values and never expire.
- **Don't rely on instant deletion.** TTL is eventually consistent at the DynamoDB layer.
- **Don't assume TableTheory hides physically present expired items.** Current reads return the item DynamoDB returns.
- **Don't combine `ttl` with `omit_empty` for a field that may legitimately be unset.** An expired-or-missing TTL value disables the expiration semantics — that's a separate access-pattern problem.

## Related

- [Lifecycle Timestamps](lifecycle-timestamps.md) — separate axis governed by its own roles
- [FaceTheory · TTL Cache Patterns](../facetheory/ttl-cache-patterns.md) — how FaceTheory uses TTL for ISR cache eviction
- [Contract Scenarios](../reference/contract-scenarios.md) — the full TTL specification
