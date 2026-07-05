---
title: Examples
---

# Examples

Use these examples when you want a runnable proof before adapting a model to your application.

## Local onboarding

- [Go local quickstart](./go-local-quickstart.md) — two commands, DynamoDB Local, verified CRUD write.
- [TypeScript local example](https://github.com/theory-cloud/TableTheory/blob/main/ts/examples/local.ts) — package-local `npm run example:local` flow.
- [Python getting started](../runtimes/python/getting-started.md) — runtime-native first CRUD flow.

## Cross-language and infrastructure examples

- [CDK multi-language example](./cdk-multilang.md) — Go, TypeScript, and Python sharing one generated table contract.
- [Release-state example](./release-state.md) — write policy and protected-transition patterns.
- [All checked-in examples](https://github.com/theory-cloud/TableTheory/tree/main/examples) — source tree for every runnable example.

Examples never bypass the TableTheory contract: they use the canonical tag/role vocabulary, DynamoDB Local for local
validation, and immutable GitHub Release installation paths when they model a consumer app.
