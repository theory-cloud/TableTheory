# Migration Guide (TypeScript)

This guide helps migrate from raw AWS SDK v3 DynamoDB usage to `@theory-cloud/tabletheory-ts`.

## Migration: raw `PutItem` → `db.create`

### Problem

Hand-authored `PutItemCommand` payloads are verbose and drift-prone across services.

### Solution

Define a model once and use typed helpers.

```ts
// ✅ CORRECT: TableTheory model + create
const db = new TheorydbClient(ddb).register(User);
await db.create('User', { PK: 'U#1', SK: 'PROFILE' }, { ifNotExists: true });
```

## Migration: manual `LastEvaluatedKey` → opaque `cursor`

### Problem

Exposing raw DynamoDB keys couples clients to your schema.

### Solution

Use the opaque cursor returned by `page()` and pass it back to the next request.
