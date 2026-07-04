---
title: Generating CDK Table Constructs from DMS
permalink: /cdk/generated-constructs/
---

# Generating CDK Table Constructs from DMS

The `tabletheory` CLI can emit AWS CDK (`aws-cdk-lib`) DynamoDB `Table`
constructs directly from a [DMS](https://github.com/theory-cloud/TableTheory/blob/main/docs/development/planning/theorydb-spec-dms-v0.1.md)
document. Because the generated table, index, and TTL shape is derived from the
same DMS that the runtimes validate against, your infrastructure definition
cannot drift from your runtime model contract.

This is the productized form of the DMS→`CreateTable` translation that the
contract-test runners implement internally; instead of hand-writing a `Table`
and *asserting* it matches the model, you generate it.

## Usage

```bash
tabletheory gen --cdk path/to/models.yml            # write to stdout
tabletheory gen --cdk --out lib/generated-tables.ts path/to/models.yml
tabletheory gen --cdk --model User path/to/models.yml   # a single model
```

`--cdk` cannot be combined with `--lang`; it always emits TypeScript CDK code.

## What is generated

For each model the generator emits one exported factory function
`create<ModelName>Table(scope, id, options)` that returns a
`dynamodb.Table`. The mapping is:

| DMS                                   | CDK output                                                   |
| ------------------------------------- | ------------------------------------------------------------ |
| `table.name`                          | `tableName`                                                  |
| `keys.partition` / `keys.sort`        | `partitionKey` / `sortKey` (`S`→STRING, `N`→NUMBER, `B`→BINARY) |
| attribute with role `ttl`             | `timeToLiveAttribute`                                        |
| `indexes[]` of `type: GSI`            | `table.addGlobalSecondaryIndex({ ... })`                     |
| `indexes[]` of `type: LSI`            | `table.addLocalSecondaryIndex({ ... })`                      |
| `projection.type` / `projection.fields` | `projectionType` / `nonKeyAttributes`                      |

Tables default to `BillingMode.PAY_PER_REQUEST` (serverless-first). The factory
accepts an `options: Partial<dynamodb.TableProps>` argument that is spread over
the generated props, so a caller can add point-in-time recovery, streams,
removal policy, or override the billing mode without editing generated code:

```typescript
import { createUserTable } from './generated-tables';

const userTable = createUserTable(this, 'UserTable', {
  pointInTimeRecovery: true,
  stream: dynamodb.StreamViewType.NEW_AND_OLD_IMAGES,
});
```

Generation is deterministic (models keep document order; index projections are
sorted), so committed output can be drift-gated in CI like the other
generators.

## `cdk synth` smoke

The `examples/cdk-multilang` demo consumes generated constructs and provides a
`cdk synth` smoke that requires no AWS account. See
[`examples/cdk-multilang/README.md`](https://github.com/theory-cloud/TableTheory/blob/main/examples/cdk-multilang/README.md)
for the local walkthrough.
