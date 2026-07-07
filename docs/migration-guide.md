---
title: TableTheory Migration Guide
---

# TableTheory Migration Guide

This guide assists in migrating existing Go applications to use TableTheory, focusing on transitions from raw AWS SDK calls or other ORMs.

TypeScript and Python ship their own runtime-specific migration guides as sibling package surfaces in the shared
TheoryCloud TableTheory subtree. This page is the Go migration guide.

## From Raw AWS SDK for Go (v2)

**Problem:** Directly using the AWS SDK for Go v2 for DynamoDB operations often leads to verbose code, manual attribute marshaling, and lacks type safety. It also requires explicit context management for every call.

**Solution:** Replace direct SDK calls with TableTheory's fluent, model-aware API. TableTheory handles marshaling/unmarshaling, context propagation, and error handling automatically; field-name and operator mistakes are still validated by the runtime rather than by a Go generic compile-time layer.

### Example: Creating an Item

```go
// ❌ OLD WAY: Raw AWS SDK v2
package main

import (
    "context"
    "log"

    "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/service/dynamodb"
    "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
    "github.com/aws/aws-sdk-go-v2/config"
)

type User struct {
    ID    string
    Email string
    Name  string
}

func createUserSDK(ctx context.Context, user User) error {
    cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion("us-east-1"))
    if err != nil {
        return err
    }
    svc := dynamodb.NewFromConfig(cfg)

    item := map[string]types.AttributeValue{
        "ID":    &types.AttributeValueMemberS{Value: user.ID},
        "Email": &types.AttributeValueMemberS{Value: user.Email},
        "Name":  &types.AttributeValueMemberS{Value: user.Name},
    }

    _, err = svc.PutItem(ctx, &dynamodb.PutItemInput{
        TableName: aws.String("users"),
        Item:      item,
    })
    return err
}

func main() {
    user := User{ID: "sdk_user_1", Email: "sdk@example.com", Name: "SDK User"}
    if err := createUserSDK(context.TODO(), user); err != nil {
        log.Fatalf("Failed to create user with SDK: %v", err)
    }
    log.Println("SDK user created.")
}
```

```go
// ✅ NEW WAY: TableTheory
package main

import (
    "context"
    "log"

    "github.com/theory-cloud/tabletheory/v2"
    "github.com/theory-cloud/tabletheory/v2/pkg/session"
)

type User struct {
    ID    string `theorydb:"pk" json:"id"`
    Email string `theorydb:"sk" json:"email"`
    Name  string `json:"name"`
}

func createUserTableTheory(ctx context.Context, db tabletheory.DB, user *User) error {
    return db.WithContext(ctx).Model(user).Create()
}

func main() {
    db, err := tabletheory.New(session.Config{Region: "us-east-1"})
    if err != nil {
        log.Fatalf("Failed to initialize TableTheory: %v", err)
    }

    user := &User{ID: "orm_user_1", Email: "orm@example.com", Name: "TableTheory User"}
    if err := createUserTableTheory(context.TODO(), db, user); err != nil {
        log.Fatalf("Failed to create user with TableTheory: %v", err)
    }
    log.Println("TableTheory user created.")
}
```

### Benefits of Migrating to TableTheory

- **Reduced Boilerplate**: Significantly less code required for common CRUD operations.
- **Type Safety**: Compile-time checks prevent common runtime errors related to attribute names and types.
- **Automatic Marshaling**: Handles conversion between Go structs and DynamoDB `AttributeValue` maps.
- **Lambda Optimization**: Built-in features for cold-start reduction and connection reuse in serverless environments.
- **Fluent API**: Chainable methods make queries and transactions more readable and maintainable.

## From split LambdaDB/core.DB timeout workarounds

Some Lambda consumers previously needed to keep both a raw `*tabletheory.LambdaDB` and a lower-level
`core.DB`/`ExtendedDB` reference so they could combine Lambda model-cache behavior with a custom timeout buffer.

After upgrading to a TableTheory release that includes `LambdaTimeoutConfig`, keep a single `*tabletheory.LambdaDB`:

```go
var db *tabletheory.LambdaDB

func init() {
    base, err := tabletheory.LambdaInit(&User{}, &Session{})
    if err != nil {
        panic(err)
    }

    db = base.WithLambdaTimeoutConfig(tabletheory.LambdaTimeoutConfig{
        Buffer: 500 * time.Millisecond,
    })
}

func handler(ctx context.Context) error {
    invocationDB := db.WithLambdaTimeout(ctx)
    var user User
    return invocationDB.Model(&User{}).Where("PK", "=", "USER#1").First(&user)
}
```

Migration checklist:

1. Replace the split raw-`LambdaDB`/`core.DB` holder with one `*tabletheory.LambdaDB`.
2. Move the custom buffer into cold-start initialization with `WithLambdaTimeoutConfig`.
3. Keep `WithLambdaTimeout(ctx)` in the handler so every invocation gets its own deadline-derived DB.
4. Validate against the release candidate before stable promotion; this is an additive migration and should not require a
   data migration.

## Go `MainExecutor` removal

`pkg/query.MainExecutor`, `pkg/query.NewExecutor`, and the legacy `pkg/query.DynamoDBAPI` executor seam were deprecated in
the 1.x line as compatibility/test seams and are removed by the v2 readiness branch. Production callers should construct
models through `tabletheory.New(...)`, `tabletheory.LambdaInit(...)`, and `DB.Model(...)`/`tabletheory.Model(...)` so
operations use the maintained runtime executor path.

If application code constructs `query.NewExecutor(...)` directly, migrate that construction to a normal TableTheory
`DB`/model flow before adopting v2. If tests used `MainExecutor` only for DynamoDB AttributeValue unmarshaling coverage,
call `query.UnmarshalItem(...)` or `query.UnmarshalItems(...)` directly; if tests need behavior, use `NewWithClient(...)`
with `pkg/testing/fakedb` or another implementation of the public `tabletheory.DynamoDBAPI` seam.

See the v2 migration guide at [`docs/migration/v2.md`](./migration/v2.md#6-go-query-executor-mainexecutor-is-removed)
for exact rewrites and downstream coordination.

## M6 Go Contract-Parity Compatibility Notes

M6 pins additional cross-runtime contract scenarios for number precision, GSI/projection behavior, pagination, and the
type matrix. Two Go parity repairs intentionally change observable Go behavior so Go matches TypeScript, Python, and
DynamoDB's own rules.

**Semver decision:** treat these Go parity repairs as breaking changes. They must be released only through the next
major release lane for this strengthening program, not as a patch or minor release.

### `ConsistentRead()` on GSI queries now fails closed

Go previously accepted `.ConsistentRead()` on a query that used a Global Secondary Index and then silently omitted the
flag before sending the DynamoDB request. Go now returns `ErrInvalidOperator`, matching TypeScript and Python, because
DynamoDB GSIs do not support strongly consistent reads.

The blast radius includes both explicit and implicit GSI reads:

- `.Index("gsi-name").ConsistentRead()` now fails before the request is sent when `gsi-name` is a GSI.
- A query that does not call `.Index(...)` can still fail if TableTheory's optimizer selects a GSI from the supplied key
  conditions and `.ConsistentRead()` was also enabled.

Migration checklist:

1. Audit Go query builders that combine `.ConsistentRead()` with `.Index(...)`.
2. Audit Go query builders that call `.ConsistentRead()` on non-primary-key access patterns; the optimizer may select a
   GSI even when the code did not name one explicitly.
3. For GSI read-after-write needs, remove `.ConsistentRead()` and use bounded retry/application verification instead.
4. For truly strong reads, query the base table (or an LSI where applicable) using primary-key conditions rather than a
   GSI access pattern.

### Go binary and set-tagged fields now write canonical DynamoDB shapes

Go writes now converge on the shared type-matrix contract:

- `[]byte` / `[]uint8` fields write as DynamoDB `B` instead of a list of per-byte numbers.
- Numeric slices tagged as sets, such as `theorydb:"set"` on `[]int`, `[]uint64`, or `[]float64`, write as `NS`.
- Binary set slices tagged as sets, such as `theorydb:"set"` on `[][]byte`, write as `BS`.
- Empty or nil set-tagged slices write as `NULL`, preserving DynamoDB's "no empty set" rule.
- Unsupported set-tagged element types now fail at write time instead of falling back to list persistence.

Read compatibility remains shape-driven: Go still reads legacy list-shaped items where the existing unmarshal path
supports that shape. The migration risk is newly heterogeneous data. Tables with old Go-written `L` values and new
canonical `B`/`NS`/`BS`/`NULL` values for the same logical field can produce different results for raw DynamoDB
filters, condition expressions, `attribute_type` checks, and downstream consumers that inspect AttributeValue shapes.

Migration checklist:

1. Inventory Go models with `[]byte` fields and `theorydb:"set"` slices.
2. Check filters, conditions, GSIs, stream processors, exports, and raw SDK consumers that assume the old list shape.
3. If a field must stay homogeneous for raw expressions or analytics, backfill legacy list-shaped values to the
   canonical `B`, `NS`, `BS`, or `NULL` shape before relying on those expressions.
4. For unsupported set-tagged element types, remove the `set` tag to keep list semantics or change the field to one of
   the supported string, numeric, or binary-set element types.

## From Legacy DynamORM

**Problem:** Legacy DynamORM models often used a mixed naming contract: the primary keys were always stored as uppercase `PK` and `SK`, while every other attribute used camelCase. TableTheory historically treated `theorydb:"pk"` and `theorydb:"sk"` as roles only, so a model like `UserID string 'theorydb:"pk"'` would default to the attribute name `userID` instead of `PK`.

**Solution:** Opt into the legacy naming mode with `theorydb:"naming:dynamorm"`. In that mode, TableTheory preserves `PK` and `SK` for the table keys and continues to derive camelCase names for non-key fields unless you override them with `attr:`.

```go
// ❌ BEFORE: default naming would map keys to userID/entity
type LegacyUser struct {
    UserID    string `theorydb:"pk" json:"PK"`
    Entity    string `theorydb:"sk" json:"SK"`
    FirstName string `json:"firstName"`
}
```

```go
// ✅ AFTER: DynamORM-compatible naming keeps PK/SK uppercase
type LegacyUser struct {
    _ struct{} `theorydb:"naming:dynamorm"`

    UserID    string `theorydb:"pk" json:"PK"`
    Entity    string `theorydb:"sk" json:"SK"`
    FirstName string `json:"firstName"`
}
```

This mode is intended for first-class support of legacy tables. Use it when all of these are true:

- the table keys must remain uppercase `PK` and `SK`
- non-key attributes should continue to use camelCase
- you want queries, unmarshaling, schema generation, and DMS metadata to agree on that contract without repeating `attr:PK` and `attr:SK` everywhere

## From Other ORMs (e.g., GORM for SQL)

**Problem:** SQL ORMs are designed for relational databases and do not translate well to DynamoDB's NoSQL, key-value, and document-oriented model. Concepts like joins and complex secondary indexes are fundamentally different.

**Solution:** Adapt your data models and query patterns to be DynamoDB-native. TableTheory provides an ORM-like experience while respecting DynamoDB's strengths.

### Key Differences and Adaptations

1.  **Data Modeling**: Think about Partition Keys (PK) and Sort Keys (SK) for efficient access patterns, not just primary keys.
    - **SQL**: `id INT PRIMARY KEY`, `name VARCHAR(255)`
    - **DynamoDB (TableTheory)**: `ID string `theorydb:"pk"`, `SK string `theorydb:"sk"`

2.  **Joins**: DynamoDB does not support joins. Denormalize data or use multiple `BatchGet` calls.

3.  **Querying**: Prioritize queries by PK/SK. Use Global Secondary Indexes (GSIs) for alternate access patterns.

```go
// ❌ OLD WAY: GORM (SQL-like)
func getActiveUsersGORM(db *gorm.DB) ([]User, error) {
    var users []User
    result := db.Where("status = ?", "active").Find(&users)
    return users, result.Error
}

// ✅ NEW WAY: TableTheory (DynamoDB-native)
// Assumes a GSI named "status-index" with 'Status' as its PK
func getActiveUsersTableTheory(db tabletheory.DB) ([]User, error) {
    var users []User
    err := db.Model(&User{}).
        Index("status-index").        // Explicitly use the GSI
        Where("Status", "=", "active").
        All(&users)
    return users, err
}
```

## Go Encrypted Read Fail-Closed Restoration

**Problem:** A recent Go compatibility path allowed encrypted-tag reads to accept legacy plaintext when
`THEORYDB_ENCRYPTED_STRICT=false`. That softened TableTheory's fail-closed encryption model by allowing
non-envelope values to bypass decryption and flow into application structs as accepted plaintext.

**Current contract:** Go encrypted-tag reads are fail-closed again. Encrypted fields must be stored as
valid TableTheory encryption envelopes. If a read encounters plaintext or any other non-envelope value for
an encrypted field, the read returns an `EncryptedFieldError` that wraps
`pkg/errors.ErrInvalidEncryptedEnvelope`.

### Migration impact

- `THEORYDB_ENCRYPTED_STRICT=false` no longer permits plaintext fallback for encrypted-tag reads.
- Deployments that still have legacy plaintext in encrypted attributes must backfill those records into
  valid envelopes before upgrading to this behavior.
- If plaintext remains in storage, reads now fail instead of silently hydrating application models with
  accepted plaintext.

### Safe upgrade path

1. Identify every encrypted attribute that may still contain legacy plaintext.
2. Backfill those attributes into valid TableTheory envelopes with the intended KMS key.
3. Verify the backfill before rollout so production reads do not trip `EncryptedFieldError` after upgrade.
4. Remove any operational reliance on `THEORYDB_ENCRYPTED_STRICT=false`; it is no longer a supported
   compatibility escape hatch for encrypted-tag reads.

## Go Anonymous Embedded Struct Helper Compatibility

**Problem:** Some Go helper surfaces historically walked only direct struct fields. For models with exported anonymous embedded
base structs, flat payloads such as `id`, `type`, and `to` could hydrate through the metadata-driven ORM path while generic
helper decoders looked only for a nested anonymous-container map such as `BaseObject: { id, type, to }`.

**Current compatibility contract:** Go helper reads now accept both shapes, and default helper writes stay legacy-safe.

### What is guaranteed now

- **Broadened decode support**: Go helper decoders accept both:
  - flat promoted-field payloads such as `id`, `type`, `to`, `actor`, `object`
  - legacy nested anonymous-container payloads such as `BaseObject: { id, type, to }`
- **Public helper coverage**:
  - `tabletheory.UnmarshalItem(...)`
  - `tabletheory.UnmarshalStreamImage(...)`
  - `pkg/types.Converter.FromAttributeValue(...)`
- **Query helper coverage**: Named Go field updates such as `Update("Status")` and batch helper key/field selection now
  resolve promoted fields that live on exported anonymous embedded structs.
- **Default write compatibility**: Historical helper paths that encoded anonymous embedded structs under a container such as
  `BaseObject` continue to do so by default. This repair does not silently flatten helper write output.
- **Anonymous-embed hook precedence**: When an anonymous embedded field has a registered custom converter or a
  `MarshalDynamoDBAttributeValue()` hook, Go helper write paths marshal that embedded container through the hook before
  any promoted-field traversal or flat anonymous-embed encoding is applied.

### Opting in to flat helper writes

New Go code can now request flat promoted-field helper encoding explicitly:

```go
converter := pkgtypes.NewConverter().WithFlatAnonymousEmbedEncoding()

av, err := converter.ToAttributeValue(activity)
if err != nil {
    return err
}

q := query.New(&activity, metadata, executor).WithConverter(converter)
_ = av
_ = q
```

This opt-in is additive:

- `pkg/types.Converter.ToAttributeValue(...)` flattens exported promoted fields from anonymous embeds
- helper surfaces that accept the configured converter (for example `Query.WithConverter(...)` and marshaler factories
  built with that converter) use the same flat helper encoding
- default helper writes stay legacy-compatible when you do nothing

### Why this is the safe path

TableTheory is already used by production systems. The lowest-risk repair is therefore:

1. broaden reads so old and new payload shapes both hydrate correctly
2. preserve the current default helper write shape
3. make any future flat helper encode mode additive and explicit rather than silent

This avoids mixed-deployment write-shape surprises while still repairing real-world decode failures.

### Verification coverage

This compatibility contract is pinned by:

- focused Go regression coverage in `pkg/types`, `pkg/query`, `internal/expr`, and `pkg/marshal`
- public API verification for `tabletheory.UnmarshalItem(...)` and `tabletheory.UnmarshalStreamImage(...)`
- integration coverage for promoted-field query/update behavior on embedded models

### Release posture

This repair is intended to remain **patch-compatible** on the current line:

- decode support broadens
- default helper writes remain unchanged
- no default encode-shape flip is part of this repair

Flat helper encoding now exists only as an **explicit opt-in** for new Go consumers. Any future default encode-shape
convergence would still require migration notes and downstream coordination before it could even be considered for a future
major release.
