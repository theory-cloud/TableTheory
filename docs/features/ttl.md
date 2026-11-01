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

## Provisioning detects TTL; it never enables TTL on an existing table

Creating a table from a model also enables TTL for the model's `ttl` role.
Enabling TTL on a table the call just created is safe: there is no existing
data, so nothing can expire.

Provisioning against a table that **already exists** only inspects TTL. It never
turns TTL on, because enabling TTL makes DynamoDB delete items whose TTL
attribute is already in the past — a forced data change on upgrade.

| Existing table state | Provisioning outcome |
| --- | --- |
| TTL enabled on the model's `ttl` attribute | idempotent success |
| TTL enabled on a **different** attribute | typed validation error naming both attributes |
| TTL disabled, or never configured | success plus a warning; TTL stays disabled |

The warning is non-fatal by design: a schema bootstrap that reuses an existing
table keeps working, while the operator sees that the model's expiry is not
active. Each runtime reports the same message on its native warning channel.

- **Go:** `DB.CreateTable`, `DB.EnsureTable`, and `schema.Manager.UpdateTable`
  detect only; `schema.Manager.VerifyTTL` is the detection helper, and
  `log.Printf` carries the warning. `schema.Manager.EnableTTL` is the explicit
  opt-in that turns TTL on.
- **TypeScript:** `createTable`, `ensureTable`, and `verifyTimeToLive` detect
  only; `console.warn` carries the warning. `updateTimeToLive` is the explicit
  opt-in.
- **Python:** `create_table`, `ensure_table`, and `verify_time_to_live` detect
  only; `warnings.warn` carries the warning. `update_time_to_live` is the
  explicit opt-in.

If the table's TTL is already enabled on a **different** attribute, the call
fails with a validation error that names both the model attribute and the
attribute that is currently enabled. TableTheory never silently switches the TTL
attribute: disable TTL on the existing attribute first if the change is
intended. Item storage is unchanged by this provisioning behavior.

The error is typed in every runtime:

| Runtime | Error surface | Classification |
| --- | --- | --- |
| Go | `errors.ErrTTLAttributeMismatch` (`errors.Is`-comparable) | also matches `errors.ErrInvalidModel` |
| TypeScript | `TheorydbError` with `code: "ErrInvalidModel"` | `ErrInvalidModel` |
| Python | `ValidationError` | `ValidationError` |

`CreateTable` and `EnsureTable` surface the same error in all three runtimes for
this conflict; the shared contract scenario
`p0/22-ttl-different-attribute-error.yml` enforces it.

### Enabling TTL deliberately

Because provisioning leaves an existing table untouched, enabling TTL is an
explicit step. Use the runtime helper, the DynamoDB `UpdateTimeToLive` API, or
your infrastructure-as-code:

- Go: `schema.Manager.EnableTTL(model)`
- TypeScript: `updateTimeToLive(ddb, model)`
- Python: `update_time_to_live(model)`

Enabling TTL on a table that already holds items is a data-affecting change:
DynamoDB starts deleting items whose tagged attribute is in the past. That is
why it is never implicit.

> **Go behavior change:** `DB.EnsureTable` still leaves an existing table's TTL
> alone, but it can now return `errors.ErrTTLAttributeMismatch` when the table's
> TTL is enabled on a different attribute than the model's `ttl` role, where it
> previously returned `nil`. Callers that relied on `EnsureTable` succeeding
> regardless of TTL state must handle that typed error.

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
