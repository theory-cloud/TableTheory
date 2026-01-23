# Core Patterns (TypeScript)

This document provides copy-pasteable patterns for the TableTheory TypeScript SDK.

## Pattern: Define a DMS-friendly model

```ts
import { defineModel } from '@theory-cloud/tabletheory-ts';

// ✅ CORRECT: explicit key attributes + roles
export const User = defineModel({
  name: 'User',
  table: { name: 'users_contract' },
  keys: {
    partition: { attribute: 'PK', type: 'S' },
    sort: { attribute: 'SK', type: 'S' },
  },
  attributes: [
    { attribute: 'PK', type: 'S', roles: ['pk'] },
    { attribute: 'SK', type: 'S', roles: ['sk'] },
    { attribute: 'nickname', type: 'S', optional: true, omit_empty: true },
    { attribute: 'version', type: 'N', roles: ['version'] },
  ],
});
```

## Pattern: CRUD with idempotency

```ts
import { DynamoDBClient } from '@aws-sdk/client-dynamodb';
import { TheorydbClient } from '@theory-cloud/tabletheory-ts';
import { User } from './models/user';

const ddb = new DynamoDBClient({
  region: 'us-east-1',
  endpoint: process.env.DYNAMODB_ENDPOINT,
});
const db = new TheorydbClient(ddb).register(User);

// ✅ CORRECT: use `ifNotExists` for idempotent creates
await db.create(
  'User',
  { PK: 'U#1', SK: 'PROFILE', nickname: 'Al' },
  { ifNotExists: true },
);
const item = await db.get('User', { PK: 'U#1', SK: 'PROFILE' });
await db.delete('User', { PK: 'U#1', SK: 'PROFILE' });
```

## Pattern: Cursor pagination

```ts
const page1 = await db.query('User').partitionKey('U#1').limit(10).page();
const page2 = page1.cursor
  ? await db.query('User').partitionKey('U#1').cursor(page1.cursor).page()
  : { items: [] };
```

## Pattern: Batch + Transactions

```ts
await db.batchWrite('User', {
  puts: [
    { PK: 'U#1', SK: 'A' },
    { PK: 'U#1', SK: 'B' },
  ],
});

const got = await db.batchGet('User', [
  { PK: 'U#1', SK: 'A' },
  { PK: 'U#1', SK: 'B' },
]);

await db.transactWrite([
  {
    kind: 'put',
    model: 'User',
    item: { PK: 'U#1', SK: 'TX' },
    ifNotExists: true,
  },
  {
    kind: 'update',
    model: 'User',
    key: { PK: 'U#1', SK: 'TX' },
    updateExpression: 'SET #ws = if_not_exists(#ws, :ws) ADD #count :inc',
    conditionExpression: 'attribute_not_exists(#count) OR #count < :maxAllowed',
    expressionAttributeNames: { '#ws': 'WindowStart', '#count': 'Count' },
    expressionAttributeValues: {
      ':ws': { S: '2026-01-23T00:00:00Z' },
      ':inc': { N: '1' },
      ':maxAllowed': { N: '100' },
    },
  },
]);
```

## Pattern: Streams unmarshalling

```ts
import { unmarshalStreamRecord } from '@theory-cloud/tabletheory-ts';

const parsed = unmarshalStreamRecord(User, record);
```

## Pattern: Deterministic unit tests (testkit)

```ts
import assert from 'node:assert/strict';
import { PutItemCommand } from '@aws-sdk/client-dynamodb';

import { TheorydbClient } from '@theory-cloud/tabletheory-ts';
import {
  createMockDynamoDBClient,
  fixedNow,
} from '@theory-cloud/tabletheory-ts/testkit';

const mock = createMockDynamoDBClient();
mock.when(PutItemCommand, async () => ({}));

const db = new TheorydbClient(mock.client, {
  now: fixedNow('2026-01-16T00:00:00.000000000Z'),
}).register(User);
await db.create('User', { PK: 'U#1', SK: 'PROFILE' });

assert.equal(mock.calls.length, 1);
```
