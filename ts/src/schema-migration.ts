import {
  BatchWriteItemCommand,
  PutItemCommand,
  ScanCommand,
  type AttributeValue,
  type DynamoDBClient,
  type ScanCommandInput,
  type WriteRequest,
} from '@aws-sdk/client-dynamodb';

import { chunk, sleep } from './batch.js';
import type { Model } from './model.js';
import { createTable, describeTable, ensureTable } from './schema.js';

/** A raw DynamoDB item (attribute-name to AttributeValue). */
export type MigrationItem = Record<string, AttributeValue>;

/**
 * A transform applied to each item during a data-copying migration. It mirrors
 * the Go `schema.TransformFunc` AttributeValue-level transforms.
 */
export type MigrationTransform = (item: MigrationItem) => MigrationItem;

export interface AutoMigrateOptions {
  /** Migrate into a different model/table than the source. Defaults to the source. */
  targetModel?: Model;
  /** Applied to each item when copying data. */
  transform?: MigrationTransform;
  /** When set, copy the source table into this backup table before migrating. */
  backupTable?: string;
  /** Scan/batch page size. Defaults to 25 (the DynamoDB BatchWriteItem limit). */
  batchSize?: number;
  /** Copy data from the source table into the target table. */
  dataCopy?: boolean;
}

const MAX_BATCH_SIZE = 25;
const MAX_RETRIES = 5;

/**
 * autoMigrate ensures the target table exists and, when requested, copies data
 * from the source table into it (optionally through a transform), mirroring the
 * Go `Manager.AutoMigrateWithOptions` story.
 */
export async function autoMigrate(
  ddb: DynamoDBClient,
  sourceModel: Model,
  opts: AutoMigrateOptions = {},
): Promise<void> {
  const targetModel = opts.targetModel ?? sourceModel;
  const batchSize =
    opts.batchSize && opts.batchSize > 0 ? opts.batchSize : MAX_BATCH_SIZE;

  if (opts.backupTable) {
    await backupSourceTable(ddb, sourceModel, opts.backupTable, batchSize);
  }

  await ensureTable(ddb, targetModel);

  if (opts.dataCopy && sourceModel.tableName !== targetModel.tableName) {
    await copyData(
      ddb,
      sourceModel.tableName,
      targetModel.tableName,
      opts.transform,
      batchSize,
    );
  }
}

async function backupSourceTable(
  ddb: DynamoDBClient,
  sourceModel: Model,
  backupTable: string,
  batchSize: number,
): Promise<void> {
  // Fail closed if the source table does not exist, matching the Go behavior.
  await describeTable(ddb, sourceModel);
  await createTable(ddb, sourceModel, { tableName: backupTable });
  await copyData(ddb, sourceModel.tableName, backupTable, undefined, batchSize);
}

async function copyData(
  ddb: DynamoDBClient,
  sourceTable: string,
  targetTable: string,
  transform: MigrationTransform | undefined,
  batchSize: number,
): Promise<void> {
  let lastKey: MigrationItem | undefined;
  let done = false;
  while (!done) {
    const input: ScanCommandInput = {
      TableName: sourceTable,
      Limit: batchSize,
    };
    if (lastKey) input.ExclusiveStartKey = lastKey;

    const resp = await ddb.send(new ScanCommand(input));
    const items = resp.Items ?? [];
    if (items.length > 0) {
      const requests: WriteRequest[] = items.map((item) => ({
        PutRequest: { Item: transform ? transform(item) : item },
      }));
      await batchWriteAll(ddb, targetTable, requests);
    }

    lastKey = resp.LastEvaluatedKey;
    done = !lastKey;
  }
}

async function batchWriteAll(
  ddb: DynamoDBClient,
  tableName: string,
  requests: WriteRequest[],
): Promise<void> {
  for (const batch of chunk(requests, MAX_BATCH_SIZE)) {
    let pending = batch;
    for (
      let attempt = 1;
      attempt <= MAX_RETRIES && pending.length > 0;
      attempt++
    ) {
      const resp = await ddb.send(
        new BatchWriteItemCommand({ RequestItems: { [tableName]: pending } }),
      );
      pending = resp.UnprocessedItems?.[tableName] ?? [];
      if (pending.length > 0 && attempt < MAX_RETRIES) {
        await sleep(attempt * attempt * 100);
      }
    }
    // Fall back to individual puts, mirroring the Go batched writer.
    for (const req of pending) {
      if (req.PutRequest?.Item) {
        await ddb.send(
          new PutItemCommand({
            TableName: tableName,
            Item: req.PutRequest.Item,
          }),
        );
      }
    }
  }
}

/** copyAllFields returns a transform that passes every attribute through unchanged. */
export function copyAllFields(): MigrationTransform {
  return (item) => ({ ...item });
}

/** renameField returns a transform that renames one attribute. */
export function renameField(
  oldName: string,
  newName: string,
): MigrationTransform {
  return (item) => {
    const out: MigrationItem = {};
    for (const [key, value] of Object.entries(item)) {
      out[key === oldName ? newName : key] = value;
    }
    return out;
  };
}

/** addField returns a transform that adds an attribute (overwriting if present). */
export function addField(
  name: string,
  value: AttributeValue,
): MigrationTransform {
  return (item) => ({ ...item, [name]: value });
}

/** removeField returns a transform that drops an attribute. */
export function removeField(name: string): MigrationTransform {
  return (item) => {
    const out: MigrationItem = {};
    for (const [key, value] of Object.entries(item)) {
      if (key !== name) out[key] = value;
    }
    return out;
  };
}

/** chainTransforms composes transforms left to right. */
export function chainTransforms(
  ...transforms: MigrationTransform[]
): MigrationTransform {
  return (item) => transforms.reduce((acc, transform) => transform(acc), item);
}
