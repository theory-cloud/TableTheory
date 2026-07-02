# API Reference (TypeScript)

<!-- AI Training: This is the API reference for the TableTheory TypeScript SDK -->

**This document describes the public API surface of `@theory-cloud/tabletheory-ts` at a signature-and-shape level.**

## Imports

```ts
import {
  DEFAULT_LAMBDA_TIMEOUT_BUFFER_MS,
  TheorydbClient,
  createLambdaDynamoDBClient,
  createLambdaTimeoutSignal,
  defineModel,
  withLambdaTimeout,
  type BatchGetResult,
  type BatchWriteResult,
  type EncryptionProvider,
  type LambdaContextLike,
  type Model,
  type Page,
  type SendOptions,
  type TransactAction,
  type UpdateBuilder,
} from '@theory-cloud/tabletheory-ts';
import { createMockDynamoDBClient } from '@theory-cloud/tabletheory-ts/testkit';
```

## Model Definition

### `defineModel(definition: ModelSchema): Model`

Defines a model with explicit attribute names and roles.

**Core fields:**

- `name`: model name used by the client registry
- `table.name`: DynamoDB table name
- `keys.partition` / `keys.sort`: key attribute names + DynamoDB scalar types
- `attributes`: list of attribute definitions (`attribute`, `type`, `roles`, `optional`, `omit_empty`, `encrypted`, etc.)
- `indexes`: optional GSI/LSI definitions with explicit key attributes
- `write_policy`: optional write-once/protected-attribute policy

`defineModel` returns a runtime model descriptor. It does not currently infer a schema-specific TypeScript item type for
`TheorydbClient`; client item payloads are still `Record<string, unknown>`.

See [Core Patterns](./core-patterns.md) for canonical model definitions.

## Client

### `new TheorydbClient(ddb, options?)`

Creates a client bound to an AWS SDK v3 `DynamoDBClient`.

Options:

```ts
{
  encryption?: EncryptionProvider;
  now?: () => string;
  sendOptions?: SendOptions;
}
```

- `encryption`: required when a registered model contains encrypted attributes
- `now`: injected RFC3339-nano clock for deterministic tests
- `sendOptions.abortSignal`: optional SDK send option, commonly provided by Lambda timeout helpers

### `register(...models: Model[]): this`

Registers one or more model definitions and returns the same client.

### `withEncryption(provider: EncryptionProvider): this`

Attaches or replaces the encryption provider on the same client.

### `withSendOptions(sendOptions?: SendOptions): TheorydbClient`

Returns a new client with the same DynamoDB client, models, clock, and encryption provider, plus the supplied send options.

### `withDynamoDBClient(ddb): TheorydbClient`

Returns a new client with the same models, clock, encryption provider, and send options, but a different AWS SDK v3
`DynamoDBClient`.

### CRUD

Actual signatures:

- `create(modelName: string, item: Record<string, unknown>, opts?: { ifNotExists?: boolean }): Promise<void>`
- `save(modelName: string, item: Record<string, unknown>): Promise<void>`
- `get(modelName: string, key: Record<string, unknown>): Promise<Record<string, unknown>>`
- `update(modelName: string, item: Record<string, unknown>, fields: string[], opts?: WritePolicyOptions): Promise<void>`
- `delete(modelName: string, key: Record<string, unknown>): Promise<void>`

Notes:

- `create(..., { ifNotExists: true })` adds an `attribute_not_exists(pk)` condition. Write-once/protected models also
  require create-if-absent automatically.
- `get` performs a consistent read and raises `ErrItemNotFound` when DynamoDB returns no item.
- `update` requires the model to define a `version` role and requires the current version value in `item[versionAttr]`.
  It increments the version using DynamoDB `ADD` and condition-checks the expected version.
- CRUD `update` does not accept raw condition-expression options. Use `updateBuilder(...)` or transaction update actions
  when you need expression-level conditions.

### `updateBuilder(modelName: string, key: Record<string, unknown>): UpdateBuilder`

Creates the shipped update-builder DSL for direct DynamoDB updates. The builder supports `set`, `setIfNotExists`, `add`,
`increment`, `decrement`, `remove`, set `delete`, list append/prepend/index operations, condition chaining, version
conditions, `returnValues`, `build`, and `execute`.

## Lambda runtime helpers

### `createLambdaDynamoDBClient(options?)`

Creates an AWS SDK v3 `DynamoDBClient` configured for Lambda-friendly connection reuse and timeout defaults.

### `DEFAULT_LAMBDA_TIMEOUT_BUFFER_MS`

The default buffer, `1000`, left before the Lambda hard timeout when deriving an invocation-scoped client.

### `createLambdaTimeoutSignal(ctx, options?)`

Creates an `AbortSignal` that aborts before the Lambda deadline.

```ts
const { signal, cleanup } = createLambdaTimeoutSignal(ctx, {
  bufferMs: 500,
});
```

### `withLambdaTimeout(client, ctx, options?)`

Returns `{ client, cleanup }`, where the derived `TheorydbClient` preserves the registered models and send options while
adding an invocation-scoped `abortSignal`. Always call `cleanup()` before the handler returns.

```ts
const baseDb = new TheorydbClient(createLambdaDynamoDBClient()).register(User);

export async function handler(event: unknown, ctx: LambdaContextLike) {
  const { client: db, cleanup } = withLambdaTimeout(baseDb, ctx, {
    bufferMs: 500,
  });
  try {
    return await db.get('User', { PK: 'USER#1', SK: 'PROFILE' });
  } finally {
    cleanup();
  }
}
```

## Query and scan builders

### `query(modelName: string): QueryBuilder`

Creates a DynamoDB Query builder.

Key, cursor, and page methods:

- `usingIndex(name: string): this`
- `partitionKey(value: unknown): this`
- `sortKey(op: '=' | '<' | '<=' | '>' | '>=' | 'between' | 'begins_with', ...values: unknown[]): this`
- `sort(direction: 'ASC' | 'DESC'): this`
- `consistentRead(enabled = true): this` (rejected for GSIs)
- `limit(n: number): this`
- `projection(fields: string[]): this`
- `cursor(encoded: string): this`
- `page(): Promise<Page>`
- `all(): Promise<Array<Record<string, unknown>>>`
- `pageWithRetry(options?): Promise<Page>`
- `describe(): BuilderShape`

Filter expressions are implemented:

- `filter(field: string, op: string, ...values: unknown[]): this`
- `orFilter(field: string, op: string, ...values: unknown[]): this`
- `filterGroup(fn: (b: FilterGroupBuilder) => void): this`
- `orFilterGroup(fn: (b: FilterGroupBuilder) => void): this`

Supported filter operators include `=`, `!=`/`<>`, `<`, `<=`, `>`, `>=`, `BETWEEN`, `IN`, `BEGINS_WITH`, `CONTAINS`,
`ATTRIBUTE_EXISTS`, and `ATTRIBUTE_NOT_EXISTS`. Encrypted attributes cannot be filtered.

Typical chain:

```ts
const page = await db.query('User').partitionKey('USER#1').limit(25).page();
const next = page.cursor
  ? await db.query('User').partitionKey('USER#1').cursor(page.cursor).page()
  : null;
```

### `scan(modelName: string): ScanBuilder`

Creates a DynamoDB Scan builder. `ScanBuilder` supports the same filter, cursor, limit, projection, `page`, `all`, retry,
and aggregation helpers as `QueryBuilder`, plus:

- `parallelScan(segment: number, totalSegments: number): this`
- `scanAllSegments(totalSegments: number, opts?: { concurrency?: number }): Promise<Array<Record<string, unknown>>>`

## Client-side aggregation helpers

`QueryBuilder` and `ScanBuilder` expose convenience helpers:

- `sum(field)`
- `average(field)`
- `min(field)`
- `max(field)`
- `aggregate(...fields)`
- `countDistinct(field)`
- `groupBy(field).count(alias).sum(field, alias).avg(field, alias).min(field, alias).max(field, alias).execute()`

These are **client-side** helpers. Each query/scan aggregation calls `all()` (or, for `groupBy`, calls `all()` when
`execute()` runs), follows every page, and materializes every matching item in memory before computing the result. Use
only for bounded result sets. There is no native server-side `count()` in this release; when the planned native `count()`
API lands, prefer it for count-only paths.

## Batch + Transactions

Actual signatures:

- `batchGet(modelName: string, keys: Array<Record<string, unknown>>, opts?: RetryOptions & { consistentRead?: boolean }): Promise<BatchGetResult>`
- `batchWrite(modelName: string, request: { puts?: Array<Record<string, unknown>>; deletes?: Array<Record<string, unknown>> }, opts?: RetryOptions): Promise<BatchWriteResult>`
- `transactWrite(actions: TransactAction[]): Promise<void>`

`batchGet` defaults to `consistentRead: true`, `maxAttempts: 5`, and `baseDelayMs: 25`. `batchWrite` defaults to
`maxAttempts: 5` and `baseDelayMs: 25`.

`TransactAction` is a discriminated union of:

- `{ kind: 'put', model, item, ifNotExists? }`
- `{ kind: 'update', model, key, updateExpression, conditionExpression?, expressionAttributeNames?, expressionAttributeValues? }`
- `{ kind: 'update', model, key, updateFn }`
- `{ kind: 'delete', model, key, conditionExpression?, expressionAttributeNames?, expressionAttributeValues? }`
- `{ kind: 'condition', model, key, conditionExpression, expressionAttributeNames?, expressionAttributeValues? }`

For models with encrypted attributes, transaction `update` actions must use `updateFn`. Raw `updateExpression`
transaction updates are rejected because they would bypass `UpdateBuilder` encryption and validation.

## Streams

- `unmarshalStreamRecord(model, record, options?)`

## Encryption

### `EncryptionProvider`

Models with encrypted attributes require an encryption provider. Encrypted payloads are stored as an envelope map
containing version + ciphertext metadata.

See [Core Patterns](./core-patterns.md) for behavior-level semantics and constraints (e.g., encrypted attributes cannot be
PK/SK or index keys).
