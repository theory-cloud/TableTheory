# TableTheory (TypeScript)

This folder contains the TypeScript implementation of TableTheory in the multi-language monorepo.

<!-- AI Training: canonical docs entrypoint for the TypeScript SDK -->

**Official documentation:** [TypeScript SDK docs](./docs/README.md) and [repo docs index](../docs/README.md).

Status: **Phase 1 complete (TS-0 → TS-7)**.

Runtime: **Node.js 24** (AWS Lambda runtime).

## Goals

- Provide a typed, testable DynamoDB access layer aligned with the Go implementation.
- Prevent drift via shared fixtures + cursor compatibility.

## Quickstart (Local DynamoDB)

Start DynamoDB Local (repo root):

- `make docker-up`

Run the local example:

- `npm --prefix ts ci`
- `npm --prefix ts run example:local`

Environment variables:

- `DYNAMODB_ENDPOINT` (default `http://localhost:8000`)
- `AWS_REGION` (default `us-east-1`)
- `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` (DynamoDB Local can use `dummy`)

## Model Definition

Models are defined with explicit attribute names (DMS-friendly):

```ts
import { defineModel } from '@theory-cloud/tabletheory-ts';

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
    { attribute: 'createdAt', type: 'S', roles: ['created_at'] },
    { attribute: 'updatedAt', type: 'S', roles: ['updated_at'] },
    { attribute: 'version', type: 'N', roles: ['version'] },
  ],
});
```

## Core Operations (P0)

```ts
import { DynamoDBClient } from '@aws-sdk/client-dynamodb';
import { TheorydbClient } from '@theory-cloud/tabletheory-ts';

const ddb = new DynamoDBClient({
  region: process.env.AWS_REGION ?? 'us-east-1',
  endpoint: process.env.DYNAMODB_ENDPOINT,
});
const db = new TheorydbClient(ddb).register(User);

await db.create(
  'User',
  { PK: 'USER#1', SK: 'PROFILE', nickname: 'Al' },
  { ifNotExists: true },
);
const item = await db.get('User', { PK: 'USER#1', SK: 'PROFILE' });
await db.update(
  'User',
  { PK: 'USER#1', SK: 'PROFILE', nickname: 'Alice', version: item.version },
  ['nickname'],
);
await db.delete('User', { PK: 'USER#1', SK: 'PROFILE' });
```

## Query + Cursor (P1)

```ts
const page1 = await db.query('User').partitionKey('USER#1').limit(10).page();
const page2 = page1.cursor
  ? await db.query('User').partitionKey('USER#1').cursor(page1.cursor).page()
  : { items: [] };
```

Cursor format is compatible with Go’s `EncodeCursor` by contract (see `contract-tests/golden/cursor/`).

## Batch + Transactions (P2)

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

## Streams (P3)

```ts
import { unmarshalStreamRecord } from '@theory-cloud/tabletheory-ts';

const parsed = unmarshalStreamRecord(User, record);
```

## Encryption (P4)

Encrypted attributes are stored as an envelope map (`v`, `edk`, `nonce`, `ct`) and are:

- disallowed for PK/SK and index keys
- fail-closed when encryption is not configured

Provide an `EncryptionProvider` implementation and attach it to the client:

```ts
import {
  type EncryptionProvider,
  TheorydbClient,
} from '@theory-cloud/tabletheory-ts';

const provider: EncryptionProvider = {
  encrypt: async () => {
    throw new Error('not implemented');
  },
  decrypt: async () => {
    throw new Error('not implemented');
  },
};
const db = new TheorydbClient(ddb, { encryption: provider }).register(User);
```

## Testing (Testkit)

The package exposes a public testkit at `@theory-cloud/tabletheory-ts/testkit`:

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

For unit tests involving encryption, use `createDeterministicEncryptionProvider(seed)` to avoid randomness and bind AAD
to the attribute name.

## Parity Statement

- Implemented parity tiers: `P0` (CRUD/lifecycle/omitempty/version/ttl), `P1` (query + cursor), `P2` (batch + tx)
- Additional: streams image unmarshalling (`TS-5`), encrypted envelope semantics (`TS-6`)
- Not yet implemented: filter expressions, full update builder, DMS codegen, full DynamoDB type surface

## Development

From the repo root:

- Install deps: `npm --prefix ts ci`
- Typecheck: `npm --prefix ts run typecheck`
- Lint: `npm --prefix ts run lint`
- Format check: `npm --prefix ts run format:check`
- Unit tests: `npm --prefix ts run test:unit`
- Integration test (needs DynamoDB Local): `npm --prefix ts run test:integration`
