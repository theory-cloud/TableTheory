# Getting Started (TypeScript)

<!-- AI Training: This is the getting-started guide for the TableTheory TypeScript SDK -->

**Goal:** install `@theory-cloud/tabletheory-ts`, connect to DynamoDB (AWS or DynamoDB Local), and perform your first CRUD operations with a model definition that is compatible with cross-language contracts.

## Prerequisites

- Node.js **20+** (Node 20 LTS and Node 24 are both exercised in CI)
- AWS credentials (for AWS) or DynamoDB Local
- Basic DynamoDB concepts (PK/SK, GSIs, condition expressions)

## Installation

This repo does **not** publish to npm. GitHub Releases are the source of truth.

Find the current version before replacing `X.Y.Z`:

```bash
gh release view --repo theory-cloud/TableTheory --json tagName,publishedAt,url
gh release list --repo theory-cloud/TableTheory --exclude-drafts --limit 10
```

The release tag includes the leading `v` (`vX.Y.Z`), while the TypeScript tarball omits it
(`theory-cloud-tabletheory-ts-X.Y.Z.tgz`).

### Option A: Install from GitHub Release asset (recommended for consumers)

```bash
# Stable release example (replace X.Y.Z)
npm install --save-exact \
  https://github.com/theory-cloud/tabletheory/releases/download/vX.Y.Z/theory-cloud-tabletheory-ts-X.Y.Z.tgz

# Prerelease example (replace X.Y.Z-rc.N)
npm install --save-exact \
  https://github.com/theory-cloud/tabletheory/releases/download/vX.Y.Z-rc.N/theory-cloud-tabletheory-ts-X.Y.Z-rc.N.tgz
```

`@aws-sdk/client-dynamodb`, `@aws-sdk/client-kms`, and `@aws-sdk/client-sts` are peer dependencies. npm 7+ installs peers automatically for ordinary installs; if your package manager disables peer auto-install (for example `--legacy-peer-deps`, some pnpm/yarn configurations, or an offline mirror), install them explicitly in your application with the same tested range:

```bash
npm install \
  @aws-sdk/client-dynamodb@^3.1053.0 \
  @aws-sdk/client-kms@^3.1053.0 \
  @aws-sdk/client-sts@^3.1053.0
```

This repository's `overrides` are development and release-build safeguards only. npm applies overrides from the consuming application's root `package.json`, so consumers that need audit overrides must declare their own root-level `overrides`/`resolutions`.

To keep pinned GitHub Release asset URLs current, copy this Renovate regex manager into the consuming repository's
`renovate.json`:

```json
{
  "customManagers": [
    {
      "customType": "regex",
      "description": "Update TableTheory TypeScript GitHub Release asset URLs",
      "managerFilePatterns": [
        "/(^|/)package\\.json$/",
        "/(^|/)package-lock\\.json$/",
        "/(^|/)README\\.md$/",
        "/(^|/)docs/.+\\.md$/"
      ],
      "matchStrings": [
        "https://github\\.com/theory-cloud/[Tt]able[Tt]heory/releases/download/v(?<currentValue>\\d+\\.\\d+\\.\\d+)/theory-cloud-tabletheory-ts-(?<assetVersion>\\d+\\.\\d+\\.\\d+)\\.tgz"
      ],
      "datasourceTemplate": "github-releases",
      "depNameTemplate": "theory-cloud/TableTheory",
      "versioningTemplate": "semver",
      "extractVersionTemplate": "^v(?<version>\\d+\\.\\d+\\.\\d+)$",
      "autoReplaceStringTemplate": "https://github.com/theory-cloud/TableTheory/releases/download/v{{{newValue}}}/theory-cloud-tabletheory-ts-{{{newValue}}}.tgz"
    }
  ]
}
```

For combined TypeScript + Python automation, see the published
[Consumer update automation](https://tabletheory.theorycloud.ai/guides/consumer-updates/) guide.

The package exposes both ESM and CommonJS entry points. ESM consumers can keep using `import`, and CommonJS consumers can use `require`:

```js
const { TheorydbClient, defineModel } = require('@theory-cloud/tabletheory-ts');
```

### Domain subpath imports

For domain-specific helpers, prefer the package subpaths so new code does not pull the whole root barrel:

```ts
import { FaceTheoryIsrMetaStore } from '@theory-cloud/tabletheory-ts/facetheory';
import { transitionReleaseState } from '@theory-cloud/tabletheory-ts/release-state';
import { LeaseManager } from '@theory-cloud/tabletheory-ts/lease';
```

The root package no longer re-exports these domain helpers in v2. FaceTheory ISR, release-state,
and lease integrations must use the subpaths above; keep the root package for generic ORM APIs.

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
console.log(item.value);
await db.delete('Note', { PK: 'NOTE#1', SK: 'v1' });
```

## Next Steps

- Use [API Reference](./api-reference.md) for exact signatures; for example, `db.update` requires a `version` role and the current version value, while `db.get` has no options parameter.
- Read [Core Patterns](./core-patterns.md) for cursor pagination, batch, transactions, streams, and encryption.
- Use [Testing Guide](./testing-guide.md) for strict mocks and deterministic encryption tests.
