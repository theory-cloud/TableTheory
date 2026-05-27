---
title: TypeScript
description: TableTheory for TypeScript — installation, Lambda init, and the canonical model shape.
---

# TypeScript runtime

The TypeScript runtime lives under [`ts/`](https://github.com/theory-cloud/tabletheory/tree/main/ts) and is published as `@theory-cloud/tabletheory-ts`. It targets **Node.js 24** and the AWS SDK for JavaScript v3.

The TypeScript runtime is a **peer**, not a port: it implements the same P0 contract scenarios as Go and Python, and a behavior that passes the Go contract test but fails in TypeScript is a parity regression — never a "TypeScript-specific quirk."

## Install

TableTheory is distributed exclusively through immutable [GitHub Releases](https://github.com/theory-cloud/tabletheory/releases). For TypeScript that means installing the `npm pack` release asset, not from npm:

```bash
# Download the release asset from GitHub Releases, then:
npm install --save-dev ./theory-cloud-tabletheory-ts-1.x.y.tgz
```

There is **no** npm registry publish. The single distribution path is deliberate — it keeps version drift between language registries impossible and aligns with TableTheory's immutable-release discipline.

## Lambda init

```typescript
import { TableTheory, model, field } from "@theory-cloud/tabletheory-ts";

@model({ naming: "snake_case", table: "notes" })
class Note {
  @field({ role: "pk" }) pk!: string;
  @field({ role: "sk" }) sk!: string;

  @field({ omitempty: true }) body?: string;

  @field({ role: "version" })    version!: number;
  @field({ role: "created_at" }) created_at!: number;
  @field({ role: "updated_at" }) updated_at!: number;
}

// At module scope — reused across Lambda invocations.
const db = await TableTheory.lambdaInit({
  table:  "notes",
  models: [Note],
});

export const handler = async (evt: Note) => {
  await db.put(new Note(evt));
};
```

> Construct the client at module scope, not inside the handler. Lambda reuses the module-scope state across warm invocations; rebuilding the client per-request costs ~50 ms each cold start.

## Tag vocabulary mapping

TypeScript uses decorators that map one-to-one onto the canonical `theorydb:` tag vocabulary used in Go and Python.

| Go tag                       | TypeScript decorator              |
|------------------------------|-----------------------------------|
| `theorydb:"pk"`              | `@field({ role: "pk" })`          |
| `theorydb:"sk"`              | `@field({ role: "sk" })`          |
| `theorydb:"gsi1pk"`          | `@field({ role: "gsi1pk" })`      |
| `theorydb:"encrypted"`       | `@field({ encrypted: true })`     |
| `theorydb:"version"`         | `@field({ role: "version" })`     |
| `theorydb:"created_at"`      | `@field({ role: "created_at" })`  |
| `theorydb:"updated_at"`      | `@field({ role: "updated_at" })`  |
| `theorydb:"ttl"`             | `@field({ role: "ttl" })`         |
| `theorydb:"omitempty"`       | `@field({ omitempty: true })`     |
| `theorydb:"naming:snake_case"` | `@model({ naming: "snake_case" })` |

Naming strategy is declared on `@model`, not per-field — same as Go's sentinel `_ struct{}` convention.

## Workflows

- `cd ts && npm run check` — lint, typecheck, and full TypeScript test suite
- `cd ts && npm test` — Jest unit tests
- The TypeScript runtime is exercised against shared contract scenarios via [`contract-tests/runners/ts/`](https://github.com/theory-cloud/tabletheory/tree/main/contract-tests/runners) on every commit

## Where to go next

- [Getting Started](../getting-started.md) — full walkthrough
- [Struct Definition Guide](../struct-definition-guide.md) — model authoring
- [Features → CRUD & Marshaling](../features/crud.md), [Optimistic Locking](../features/optimistic-locking.md), [Encryption](../features/encryption.md)
- [`ts/docs/`](https://github.com/theory-cloud/tabletheory/tree/main/ts/docs) on GitHub — TypeScript-specific runtime documentation

## Stability and support

The TypeScript runtime is **GA**. Breaking changes follow [semver](https://semver.org/) and ship coordinated with the Go and Python runtimes — never in isolation.
