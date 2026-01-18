# Getting Started (TypeScript)

<!-- AI Training: This is the getting-started guide for the TableTheory TypeScript SDK -->

**Goal:** install `@theory-cloud/tabletheory-ts`, connect to DynamoDB (AWS or DynamoDB Local), and perform your first CRUD operations with a model definition that is compatible with cross-language contracts.

## Prerequisites

- Node.js **24** (required; matches AWS Lambda Node.js 24 runtime)
- AWS credentials (for AWS) or DynamoDB Local
- Basic DynamoDB concepts (PK/SK, GSIs, condition expressions)

## Installation

This repo does **not** publish to npm. GitHub Releases are the source of truth.

### Option A: Install from GitHub Release asset (recommended for consumers)

```bash
# Stable release example (replace X.Y.Z)
npm install --save-exact \
  https://github.com/theory-cloud/tabletheory/releases/download/vX.Y.Z/theory-cloud-tabletheory-ts-X.Y.Z.tgz

# Prerelease example (replace X.Y.Z-rc.N)
npm install --save-exact \
  https://github.com/theory-cloud/tabletheory/releases/download/vX.Y.Z-rc.N/theory-cloud-tabletheory-ts-X.Y.Z-rc.N.tgz
```

### Option B: Develop from source (this monorepo)

```bash
npm --prefix ts ci
npm --prefix ts run build
```

## Quickstart (DynamoDB Local)

Start DynamoDB Local from the repo root:

```bash
make docker-up
```

Run the local example:

```bash
npm --prefix ts run example:local
```

### Minimal CRUD example

```ts
import { DynamoDBClient } from '@aws-sdk/client-dynamodb';
import { TheorydbClient, defineModel } from '@theory-cloud/tabletheory-ts';

// ✅ CORRECT: define explicit attribute names (DMS-friendly)
const Note = defineModel({
  name: 'Note',
  table: { name: 'notes_contract' },
  keys: {
    partition: { attribute: 'PK', type: 'S' },
    sort: { attribute: 'SK', type: 'S' },
  },
  attributes: [
    { attribute: 'PK', type: 'S', roles: ['pk'] },
    { attribute: 'SK', type: 'S', roles: ['sk'] },
    { attribute: 'value', type: 'N' },
  ],
});

const ddb = new DynamoDBClient({
  region: process.env.AWS_REGION ?? 'us-east-1',
  endpoint: process.env.DYNAMODB_ENDPOINT ?? 'http://localhost:8000',
  credentials: { accessKeyId: 'dummy', secretAccessKey: 'dummy' },
});

const db = new TheorydbClient(ddb).register(Note);

await db.create(
  'Note',
  { PK: 'NOTE#1', SK: 'v1', value: 123 },
  { ifNotExists: true },
);
const item = await db.get('Note', { PK: 'NOTE#1', SK: 'v1' });
await db.delete('Note', { PK: 'NOTE#1', SK: 'v1' });
```

## Next Steps

- Read [Core Patterns](./core-patterns.md) for cursor pagination, batch, transactions, streams, and encryption.
- Use [Testing Guide](./testing-guide.md) for strict mocks and deterministic encryption tests.
- Use [API Reference](./api-reference.md) when you need signature-level detail.
