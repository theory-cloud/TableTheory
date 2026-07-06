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

## Schema migration (`autoMigrate`)

This mirrors the Go [migration guide](https://github.com/theory-cloud/tabletheory/blob/main/docs/migration-guide.md)
schema-migration story: move data from a v1 table shape to a v2 shape, optionally backing up the source first and
transforming each item on the way. It is reproducible against DynamoDB Local — no AWS account required.

`autoMigrate(ddb, sourceModel, options)` ensures the target table exists and, when `dataCopy` is set, scans the source
table and writes each item into the target (through an optional `transform`). Item-level transforms mirror the Go
helpers:

- `renameField(oldName, newName)`
- `addField(name, value)`
- `removeField(name)`
- `copyAllFields()`
- `chainTransforms(...transforms)` composes them left to right

### Walkthrough

```ts
import {
  addField,
  autoMigrate,
  chainTransforms,
  defineModel,
  ensureTable,
  renameField,
} from '@theory-cloud/tabletheory-ts';
import { DynamoDBClient, PutItemCommand } from '@aws-sdk/client-dynamodb';

const ddb = new DynamoDBClient({
  region: process.env.AWS_REGION ?? 'us-east-1',
  endpoint: process.env.DYNAMODB_ENDPOINT ?? 'http://localhost:8000',
  credentials: { accessKeyId: 'dummy', secretAccessKey: 'dummy' },
});

const userV1 = defineModel({
  name: 'UserV1',
  table: { name: 'users_migration_v1' },
  keys: {
    partition: { attribute: 'PK', type: 'S' },
    sort: { attribute: 'SK', type: 'S' },
  },
  attributes: [
    { attribute: 'PK', type: 'S', roles: ['pk'] },
    { attribute: 'SK', type: 'S', roles: ['sk'] },
    { attribute: 'name', type: 'S' },
  ],
});

const userV2 = defineModel({
  name: 'UserV2',
  table: { name: 'users_migration_v2' },
  keys: {
    partition: { attribute: 'PK', type: 'S' },
    sort: { attribute: 'SK', type: 'S' },
  },
  attributes: [
    { attribute: 'PK', type: 'S', roles: ['pk'] },
    { attribute: 'SK', type: 'S', roles: ['sk'] },
    { attribute: 'displayName', type: 'S' },
    { attribute: 'status', type: 'S' },
  ],
});

// Seed the v1 table.
await ensureTable(ddb, userV1);
await ddb.send(
  new PutItemCommand({
    TableName: userV1.tableName,
    Item: { PK: { S: 'USER#1' }, SK: { S: 'v1' }, name: { S: 'Ada' } },
  }),
);

// Back up v1, then migrate v1 -> v2: rename `name` -> `displayName` and add `status`.
await autoMigrate(ddb, userV1, {
  targetModel: userV2,
  dataCopy: true,
  backupTable: 'users_migration_backup',
  transform: chainTransforms(
    renameField('name', 'displayName'),
    addField('status', { S: 'active' }),
  ),
});
// users_migration_v2 now holds { PK, SK, displayName: 'Ada', status: 'active' };
// users_migration_backup holds the original { PK, SK, name: 'Ada' }.
```

A runnable version of this walkthrough lives in
`ts/test/integration/schema-migration.test.ts` and runs on the integration lane against DynamoDB Local.
