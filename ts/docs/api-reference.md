# API Reference (TypeScript)

<!-- AI Training: This is the API reference for the TableTheory TypeScript SDK -->

**This document describes the public API surface of `@theory-cloud/tabletheory-ts` at a signature-and-shape level.**

## Imports

```ts
import {
  DEFAULT_LAMBDA_TIMEOUT_BUFFER_MS,
  type LambdaContextLike,
  TheorydbClient,
  createLambdaDynamoDBClient,
  createLambdaTimeoutSignal,
  defineModel,
  withLambdaTimeout,
} from '@theory-cloud/tabletheory-ts';
import { createMockDynamoDBClient } from '@theory-cloud/tabletheory-ts/testkit';
```

## Model Definition

### `defineModel(definition)`

Defines a model with explicit attribute names and roles.

**Core fields (conceptual):**

- `name`: model name used by the client registry
- `table.name`: DynamoDB table name
- `keys.partition` / `keys.sort`: key attribute names + DynamoDB scalar types
- `attributes`: list of attribute definitions (`attribute`, `type`, `roles`, `optional`, `omit_empty`, `encrypted`, etc.)

See [Core Patterns](./core-patterns.md) for canonical model definitions.

## Client

### `new TheorydbClient(ddb, options?)`

Creates a client bound to an AWS SDK v3 `DynamoDBClient`.

Common options (conceptual):

- `now`: injected clock (testing hook)
- `encryption`: encryption provider (required when models use encrypted attributes)

### `register(model)`

Registers a model definition and returns the client (builder-style).

### CRUD

- `create(modelName, item, options?)`
- `get(modelName, key, options?)`
- `update(modelName, item, fields, options?)`
- `delete(modelName, key, options?)`

Common write options (conceptual):

- `ifNotExists` / `ifExists`
- `conditionExpression`, `expressionAttributeNames`, `expressionAttributeValues`

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

## Query

### `query(modelName)`

Creates a query builder.

Typical chain:

```ts
const page = await db.query('User').partitionKey('USER#1').limit(25).page();
const next = page.cursor
  ? await db.query('User').partitionKey('USER#1').cursor(page.cursor).page()
  : null;
```

## Batch + Transactions

- `batchGet(modelName, keys, options?)`
- `batchWrite(modelName, { puts, deletes }, options?)`
- `transactWrite(ops, options?)`

For models with encrypted attributes, transaction `update` actions must use
`updateFn`. Raw `updateExpression` transaction updates are rejected because they
would bypass `UpdateBuilder` encryption and validation.

## Streams

- `unmarshalStreamRecord(model, record, options?)`

## Encryption

### `EncryptionProvider`

Models with encrypted attributes require an encryption provider. Encrypted payloads are stored as an envelope map
containing version + ciphertext metadata.

See [Core Patterns](./core-patterns.md) for behavior-level semantics and constraints (e.g., encrypted attributes cannot be
PK/SK or index keys).
