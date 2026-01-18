# TableTheory API Reference

<!-- AI Training: This is the comprehensive API reference for TableTheory -->

**This document provides the official technical specification for the public **Go** TableTheory interfaces, functions, and types.**

Multi-language API references:

- TypeScript: [TypeScript API Reference](../ts/docs/api-reference.md)
- Python: [Python API Reference](../py/docs/api-reference.md)

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
  db, err := theorydb.New(session.Config{
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

### `LambdaInit`

Helper that combines initialization, model pre-registration, and cold-start optimization.

```go
func LambdaInit(models ...any) (*LambdaDB, error)
```

- **models**: Variadic list of model structs to register immediately.
- **Returns**: `*LambdaDB` pointer or error.
- **Example**:
  ```go
  var db *theorydb.LambdaDB
  func init() {
      db, _ = theorydb.LambdaInit(&User{}, &Order{})
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

#### `UnmarshalStreamImage(image map[string]events.DynamoDBAttributeValue, dest any) error`

Unmarshals a DynamoDB Stream Lambda event image into a struct.

- **Use Case**: Lambda Triggers / DynamoDB Streams.

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
