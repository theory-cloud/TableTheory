---
title: TypeScript
description: TableTheory for TypeScript — installation, defineModel + TheorydbClient.
---

# TypeScript runtime

The TypeScript runtime lives under [`ts/`](https://github.com/theory-cloud/tabletheory/tree/main/ts) and is distributed as `@theory-cloud/tabletheory-ts`. It targets **Node.js 24** and the AWS SDK for JavaScript v3.

The TypeScript runtime is a **peer**, not a port: it implements the same P0 contract scenarios as Go and Python, and a behavior that passes the Go contract test but fails in TypeScript is a parity regression — never a "TypeScript-specific quirk."

## Install

This repo does **not** publish to npm. GitHub Releases are the source of truth. Install the release tarball directly:

```bash
# Stable release (replace X.Y.Z)
npm install --save-exact \
  https://github.com/theory-cloud/tabletheory/releases/download/vX.Y.Z/theory-cloud-tabletheory-ts-X.Y.Z.tgz

# Prerelease (replace X.Y.Z-rc.N)
npm install --save-exact \
  https://github.com/theory-cloud/tabletheory/releases/download/vX.Y.Z-rc.N/theory-cloud-tabletheory-ts-X.Y.Z-rc.N.tgz
```

The single distribution path is deliberate — it makes version drift between language registries impossible.

## defineModel + TheorydbClient

The TypeScript public surface is **explicit**: models are declared via `defineModel({ … })` (no decorators), and operations run through a `TheorydbClient` instance.

```typescript
import { DynamoDBClient } from '@aws-sdk/client-dynamodb';
import { TheorydbClient, defineModel } from '@theory-cloud/tabletheory-ts';

const Note = defineModel({
  name: 'Note',
  table: { name: 'notes_contract' },
  keys: {
    partition: { attribute: 'PK', type: 'S' },
    sort:      { attribute: 'SK', type: 'S' },
  },
  attributes: [
    { attribute: 'PK',        type: 'S', roles: ['pk'] },
    { attribute: 'SK',        type: 'S', roles: ['sk'] },
    { attribute: 'body',      type: 'S', optional: true, omit_empty: true },
    { attribute: 'createdAt', type: 'S', roles: ['created_at'] },
    { attribute: 'updatedAt', type: 'S', roles: ['updated_at'] },
    { attribute: 'version',   type: 'N', roles: ['version'] },
  ],
});

const ddb = new DynamoDBClient({ region: 'us-east-1' });
const db  = new TheorydbClient(ddb).register(Note);

await db.create('Note', {
  PK:   'USER#42',
  SK:   'NOTE#welcome',
  body: 'Hello, Theory Cloud.',
});

const item = await db.get('Note', { PK: 'USER#42', SK: 'NOTE#welcome' });
```

For a complete working program (model + GSI + create + query + update + delete + cursor pagination), see [`ts/examples/local.ts`](https://github.com/theory-cloud/tabletheory/blob/main/ts/examples/local.ts).

## Role vocabulary

Every `role` accepted by `defineModel` attributes maps one-to-one onto the canonical TableTheory contract:

| Go tag                       | TypeScript role / option                    |
|------------------------------|---------------------------------------------|
| `theorydb:"pk"`              | `roles: ['pk']`                             |
| `theorydb:"sk"`              | `roles: ['sk']`                             |
| `theorydb:"gsi1pk"`          | `roles: ['gsi1pk']` + `indexes:`            |
| `theorydb:"encrypted"`       | `encryption: { v: 1 }`                      |
| `theorydb:"version"`         | `roles: ['version']`                        |
| `theorydb:"created_at"`      | `roles: ['created_at']`                     |
| `theorydb:"updated_at"`      | `roles: ['updated_at']`                     |
| `theorydb:"ttl"`             | `roles: ['ttl']`                            |
| `theorydb:"omitempty"`       | `omit_empty: true`                          |

Naming strategy is implied by how you declare each attribute's name — the explicit shape of `defineModel` means no separate "naming strategy" decoration is needed.

## CRUD methods

`TheorydbClient` exposes the canonical CRUD surface:

```typescript
await db.create('Note', { … });
const item = await db.get('Note', key);
await db.update('Note', { … }, ['body']);
await db.delete('Note', key);

const page = await db.query('Note').partitionKey('USER#42').limit(20).page();
```

## Workflows

- `cd ts && npm run check` — lint, typecheck, and full TypeScript test suite
- `cd ts && npm test` — Jest unit tests
- Exercised against shared contract scenarios via [`contract-tests/runners/`](https://github.com/theory-cloud/tabletheory/tree/main/contract-tests/runners) on every commit

## Where to go next

- [Getting Started](https://theory-cloud.github.io/tabletheory/getting-started/) — full walkthrough
- [`ts/docs/getting-started.md`](https://github.com/theory-cloud/tabletheory/blob/main/ts/docs/getting-started.md) — TypeScript-specific runtime documentation
- [`ts/docs/api-reference.md`](https://github.com/theory-cloud/tabletheory/blob/main/ts/docs/api-reference.md) — full TypeScript API reference
- [Features → CRUD & Marshaling](../features/crud.md), [Optimistic Locking](../features/optimistic-locking.md), [Encryption](../features/encryption.md)

## Stability and support

The TypeScript runtime is **GA**. Versions are aligned with Go and Python per the multi-language version-sync invariant. Breaking changes ship coordinated with the other two runtimes — never in isolation.
