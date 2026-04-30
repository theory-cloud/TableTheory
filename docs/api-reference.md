# TableTheory API Reference

<!-- AI Training: This is the comprehensive API reference for TableTheory -->

**This document provides the official technical specification for the public **Go** TableTheory interfaces, functions, and types.**

TypeScript and Python ship their own runtime-specific API references as sibling package surfaces in the shared
TheoryCloud TableTheory subtree. This page is the Go API reference.

## Table of Contents

- [Initialization](#initialization)
- [Configuration](#configuration)
- [Core Interfaces](#core-interfaces)
  - [DB Interface](#db-interface)
  - [LambdaDB](#lambdadb-struct)
- [Query Builder](#query-builder)
- [Transaction Builder](#transaction-builder)
- [Update Builder](#update-builder)
- [Schema Management](#schema-management)
- [Utilities](#utilities)
- [Error Handling](#error-handling)
- [Multi-Account Access](#multi-account-access)

---

## Initialization

### `New`

Initializes a standard TableTheory session.

```go
func New(config session.Config) (core.ExtendedDB, error)
```

- **config**: `session.Config` struct defining connection parameters.
- **Returns**: `core.ExtendedDB` interface or error.
- **Example**:
  ```go
  db, err := tabletheory.New(session.Config{
      Region: "us-east-1",
  })
  ```

### `NewLambdaOptimized`

Initializes a singleton, thread-safe session optimized for AWS Lambda. Reuses the global connection pool.

```go
func NewLambdaOptimized() (*LambdaDB, error)
```

- **Returns**: `*LambdaDB` pointer or error.
- **Best Practice**: Call this in your `init()` function or global variable declaration.
- **Encryption**: If any registered model includes `theorydb:"encrypted"`, set `KMS_KEY_ARN` (or `TABLETHEORY_KMS_KEY_ARN`) in your Lambda environment.

### `LambdaInit`

Helper that combines initialization, model pre-registration, and cold-start optimization.

```go
func LambdaInit(models ...any) (*LambdaDB, error)
```

- **models**: Variadic list of model structs to register immediately.
- **Returns**: `*LambdaDB` pointer or error.
- **Example**:
  ```go
  var db *tabletheory.LambdaDB
  func init() {
      db, _ = tabletheory.LambdaInit(&User{}, &Order{})
  }
  ```

### `LambdaTimeoutConfig`

Configures the buffer TableTheory leaves before the Lambda invocation deadline when `LambdaDB.WithLambdaTimeout`
derives an invocation-scoped DB.

```go
type LambdaTimeoutConfig struct {
    Buffer time.Duration
}
```

- **Default**: `WithLambdaTimeout` uses a 1 second buffer when no positive custom buffer is configured.
- **Custom buffer**: call `WithLambdaTimeoutConfig` once at cold start, then call `WithLambdaTimeout(ctx)` per invocation.
- **Non-positive buffers**: preserve the default behavior.

```go
var db *tabletheory.LambdaDB

func init() {
    base, err := tabletheory.LambdaInit(&User{}, &Order{})
    if err != nil {
        panic(err)
    }

    db = base.WithLambdaTimeoutConfig(tabletheory.LambdaTimeoutConfig{
        Buffer: 500 * time.Millisecond,
    })
}

func handler(ctx context.Context) error {
    invocationDB := db.WithLambdaTimeout(ctx)
    return invocationDB.Model(&User{}).Create()
}
```

---

## Configuration

### `session.Config`

The configuration struct used in `New()`.

| Field            | Type                | Description                                                                                             | Default     |
| ---------------- | ------------------- | ------------------------------------------------------------------------------------------------------- | ----------- |
| `Region`         | `string`            | AWS Region (e.g., "us-east-1")                                                                          | "us-east-1" |
| `Endpoint`       | `string`            | Custom endpoint URL (for DynamoDB Local)                                                                | ""          |
| `KMSKeyARN`      | `string`            | AWS KMS key ARN used for `theorydb:"encrypted"` fields (required if any encrypted fields exist)         | ""          |
| `KMSClient`      | `session.KMSClient` | Optional injected KMS client (testing hook; avoids real AWS KMS calls)                                  | `nil`       |
| `EncryptionRand` | `io.Reader`         | Optional injected randomness source for encryption nonces (testing hook; default is crypto/rand.Reader) | `nil`       |
| `Now`            | `func() time.Time`  | Optional injected clock for lifecycle timestamps (createdAt/updatedAt)                                  | `nil`       |
| `MaxRetries`     | `int`               | Max SDK retries for failed requests                                                                     | 3           |
| `DefaultRCU`     | `int64`             | Read Capacity Units for new tables                                                                      | 5           |
| `DefaultWCU`     | `int64`             | Write Capacity Units for new tables                                                                     | 5           |
| `AutoMigrate`    | `bool`              | If true, creates tables on registration                                                                 | false       |
| `EnableMetrics`  | `bool`              | If true, logs internal metrics                                                                          | false       |

---

## Core Interfaces

### `DB` Interface

The primary interface for database operations.

#### `Model(entity any) Query`

Creates a fluent query builder for the given entity.

- **entity**: Pointer to a struct (e.g., `&User{}`) or struct instance.

#### `Transaction(fn func(*Tx) error) error`

Executes a function within a simple transaction scope.

- **fn**: Closure receiving a `*Tx` handle.

#### `Transact() TransactionBuilder`

Returns a fluent builder for complex DynamoDB transactions (`TransactWriteItems`).

#### `TransactWrite(ctx context.Context, fn func(TransactionBuilder) error) error`

Helper that initializes a transaction builder, runs your function, and executes the transaction.

### `LambdaDB` Struct

Wraps `DB` with Lambda-specific features.

#### `PreRegisterModels(models ...any) error`

Registers models during initialization to avoid runtime reflection costs.

#### `WithLambdaTimeout(ctx context.Context) *LambdaDB`

Returns a DB instance that respects Lambda execution time, cancelling requests before a hard timeout occurs.
By default, TableTheory leaves 1 second before the Lambda deadline for cleanup and response handling. Configure a
different buffer with `WithLambdaTimeoutConfig` during cold start:

```go
db = db.WithLambdaTimeoutConfig(tabletheory.LambdaTimeoutConfig{
    Buffer: 750 * time.Millisecond,
})
```

Use `WithLambdaTimeoutConfig` on `LambdaDB` rather than the lower-level `core.DB.WithLambdaTimeoutBuffer`; this keeps the
Lambda-specific model cache and cold-start metadata attached to the derived `LambdaDB`.

#### `WithLambdaTimeoutConfig(config LambdaTimeoutConfig) *LambdaDB`

Returns a `LambdaDB` configured with the invocation timeout buffer that subsequent `WithLambdaTimeout(ctx)` calls apply.
The method is additive and preserves registered model caches, session state, converters, marshaling metadata, and Lambda
memory/X-Ray configuration.

#### `GetMemoryStats() LambdaMemoryStats`

Returns memory usage statistics useful for tuning Lambda memory allocation.

---

## Query Builder

The `Query` interface is returned by `db.Model()`.

### Filtering

#### `Where(field string, op string, value any) Query`

Adds a condition. Translates to `KeyConditionExpression` if field is a key, or `FilterExpression` otherwise.

- **op**: `=`, `>`, `<`, `>=`, `<=`, `BEGINS_WITH`, `BETWEEN`.

#### `Index(name string) Query`

Specifies a Global Secondary Index (GSI) or Local Secondary Index (LSI).

#### `Filter(field string, op string, value any) Query`

Explicitly adds a `FilterExpression` (scans result set).

#### `Limit(n int) Query`

Sets `Limit` parameter.

#### `ConsistentRead() Query`

Enables strong consistency (consumes 2x RCU).

### Execution

#### `First(dest any) error`

Retrieves the first matching item.

- **dest**: Pointer to a struct.

#### `All(dest any) error`

Retrieves all matching items.

- **dest**: Pointer to a slice of structs.

#### `Count() (int64, error)`

Returns the count of matching items.

#### `Create() error`

Inserts the item used in `Model()`.

#### `Update(fields ...string) error`

Updates specific fields of the item used in `Model()`. If `fields` is empty, updates all non-key fields.

- **Named-field compatibility**: On Go models with exported anonymous embedded structs, explicit field updates such as
  `Update("Status")` continue to resolve promoted fields even when the value lives on an embedded base struct.
- **Legacy write-shape compatibility**: When `fields` is empty and the query falls back to the tag-driven all-fields
  helper path, anonymous embedded struct containers keep their historical nested write shape rather than flattening
  promoted fields by default.
- **Opt-in flat helper writes (Go)**: If you explicitly want flat promoted-field helper encoding, provide
  `Query.WithConverter(pkgtypes.NewConverter().WithFlatAnonymousEmbedEncoding())`. The default helper write shape does
  not change unless you opt in.
- **Anonymous-embed hook precedence (Go)**: If an anonymous embedded field has a registered custom converter or a
  `MarshalDynamoDBAttributeValue()` hook, Go helper writes apply that hook to the embedded container before any
  promoted-field traversal or flat anonymous-embed encoding is considered.

#### `Delete() error`

Deletes the item identified by the primary key in `Model()`.

### Batch Operations

#### `BatchGet(keys []any, dest any) error`

Fetches up to 100 items by primary key in parallel chunks.

- **keys**: Slice of structs or primitive keys.
- **dest**: Pointer to a slice of structs.

#### `BatchCreate(items any) error`

Creates up to 25 items in a single request.

- **items**: Slice of structs.

#### `BatchDelete(keys []any) error`

Deletes up to 25 items by primary key.

### Conditional Writes

#### `IfNotExists() Query`

Guards `Create()`: succeeds only if the primary key does not exist.

#### `IfExists() Query`

Guards `Update()`/`Delete()`: succeeds only if the primary key exists.

#### `WithCondition(field, op string, value any) Query`

Adds a lightweight condition to a write operation.

---

## Transaction Builder

The `TransactionBuilder` interface allows composing up to 100 operations atomically.

### Methods

#### `Put(model any, conditions ...TransactCondition) TransactionBuilder`

Adds a `PutItem` operation.

#### `Update(model any, fields []string, conditions ...TransactCondition) TransactionBuilder`

Adds an `UpdateItem` operation for specific fields.

#### `Delete(model any, conditions ...TransactCondition) TransactionBuilder`

Adds a `DeleteItem` operation.

#### `ConditionCheck(model any, conditions ...TransactCondition) TransactionBuilder`

Adds a `ConditionCheck` (doesn't modify data, just verifies state).

#### `Execute() error`

Commits the transaction.

### Helper Functions

Found in `theorydb` package.

- `IfNotExists()`: Condition ensuring item is new.
- `IfExists()`: Condition ensuring item exists.
- `AtVersion(v int64)`: Optimistic locking condition.
- `Condition(field, op, value)`: Generic field condition.

---

## Update Builder

Returned by `Query.UpdateBuilder()`, this interface allows building fine-grained update expressions.

### Methods

#### `Set(field string, value any)`

Sets a field to a value (`SET #f = :v`).

#### `SetIfNotExists(field string, value, defaultVal any)`

Sets a field only if it doesn't exist (`SET #f = if_not_exists(#f, :d)`).

#### `Increment(field string)`

Increments a number (`SET #n = #n + :1`).

#### `Add(field string, value any)`

Adds to a set or number (`ADD #f :v`).

#### `Remove(field string)`

Removes an attribute (`REMOVE #f`).

#### `AppendToList(field string, values any)`

Appends elements to a list.

---

## Schema Management

#### `CreateTable(model any, opts ...any) error`

Creates a table based on struct tags.

- **Warning**: For development use. Production should use Terraform/CDK.

#### `AutoMigrate(models ...any) error`

Checks if tables exist and creates them if missing.

#### `EnsureTable(model any) error`

Idempotent check-and-create.

---

## Utilities

#### `UnmarshalItem(item map[string]types.AttributeValue, dest any) error`

Unmarshals a raw AWS SDK item map into a struct.

- **Use Case**: Processing manual SDK calls.
- **Anonymous embedded struct compatibility (Go)**: Exported promoted fields decode from both:
  - flat payloads such as `id`, `type`, `to`
  - legacy nested anonymous-container payloads such as `BaseObject: { id, type, to }`
- **Verification coverage**: The public API verifier and focused Go regression tests cover promoted-field decode on both
  raw item maps and stream images.
- **Migration guidance**: See the [migration guide's anonymous embedded helper compatibility section](./migration-guide.md#go-anonymous-embedded-struct-helper-compatibility).

#### `UnmarshalStreamImage(image map[string]events.DynamoDBAttributeValue, dest any) error`

Unmarshals a DynamoDB Stream Lambda event image into a struct.

- **Use Case**: Lambda Triggers / DynamoDB Streams.
- **Anonymous embedded struct compatibility (Go)**: Matches `UnmarshalItem` semantics for exported promoted fields,
  including legacy nested anonymous-container decode compatibility.

---

## Error Handling

TableTheory exports sentinel errors in `github.com/theory-cloud/tabletheory/pkg/errors`.

### Common Errors

| Error Variable       | Description                                          |
| -------------------- | ---------------------------------------------------- |
| `ErrItemNotFound`    | Returned by `First()` when no item matches.          |
| `ErrConditionFailed` | Returned when a conditional write/transaction fails. |
| `ErrInvalidModel`    | Returned when a struct lacks `theorydb:"pk"` tags.   |
| `ErrTableNotFound`   | Returned when the table does not exist in AWS.       |

### Custom Error Types

#### `TransactionError`

Returned when a transaction fails. Contains:

- `OperationIndex`: The index of the operation that failed (0-based).
- `Reason`: The cancellation reason from DynamoDB.

#### `TheorydbError`

Wraps internal errors with context (Model name, Operation type).

---

## Multi-Account Access

### `NewMultiAccount`

```go
func NewMultiAccount(accounts map[string]AccountConfig) (*MultiAccountDB, error)
```

Creates a multi-account DynamoDB client that manages cross-account access via STS AssumeRole. Each partner account
gets its own `LambdaDB` instance with cached credentials that refresh automatically in the background.

**Parameters:**

- `accounts`: Map of partner ID to `AccountConfig`. The partner ID is an arbitrary key used to retrieve the
  connection later via `Partner()`.

**Returns:** A `*MultiAccountDB` with a base DB (current account) and per-partner connections created on demand.

### `AccountConfig`

```go
type AccountConfig struct {
    RoleARN         string
    ExternalID      string
    Region          string
    SessionDuration time.Duration // Optional: defaults to 1 hour
}
```

Configuration for a partner account. `RoleARN` and `ExternalID` are used for STS AssumeRole. `Region` determines
the DynamoDB endpoint for that partner's tables.

### `MultiAccountDB` Methods

#### `Partner`

```go
func (mdb *MultiAccountDB) Partner(partnerID string) (*LambdaDB, error)
```

Returns a `LambdaDB` for the specified partner account. Connections are cached and credentials are refreshed
automatically before expiry. An empty `partnerID` returns the base DB (current account).

#### `AddPartner` / `RemovePartner`

```go
func (mdb *MultiAccountDB) AddPartner(partnerID string, config AccountConfig)
func (mdb *MultiAccountDB) RemovePartner(partnerID string)
```

Dynamically add or remove partner configurations at runtime. `RemovePartner` also clears the cached connection.

#### `WithContext`

```go
func (mdb *MultiAccountDB) WithContext(ctx context.Context) *MultiAccountDB
```

Returns a new `MultiAccountDB` that passes the context through to the base DB for Lambda timeout awareness.
Shares the same credential cache as the original.

#### `Close`

```go
func (mdb *MultiAccountDB) Close() error
```

Stops background credential refresh and closes the base DB connection.

### Partner Context Helpers

```go
func PartnerContext(ctx context.Context, partnerID string) context.Context
func GetPartnerFromContext(ctx context.Context) string
```

Attach and retrieve a partner ID from `context.Context` for tracing and request-scoped routing.

### Example

```go
accounts := map[string]tabletheory.AccountConfig{
    "partner-a": {
        RoleARN:    "arn:aws:iam::123456789012:role/TableTheoryAccess",
        ExternalID: "partner-a-external-id",
        Region:     "us-east-1",
    },
}

mdb, err := tabletheory.NewMultiAccount(accounts)
if err != nil {
    log.Fatal(err)
}
defer mdb.Close()

// Use partner-scoped DB
db, err := mdb.Partner("partner-a")
if err != nil {
    log.Fatal(err)
}

// db is a *LambdaDB — use it like any other TableTheory DB
err = db.Put(ctx, &myModel)
```
