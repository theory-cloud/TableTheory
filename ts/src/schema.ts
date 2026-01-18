import {
  CreateTableCommand,
  DeleteTableCommand,
  DescribeTableCommand,
  type AttributeDefinition,
  type CreateTableCommandInput,
  type DynamoDBClient,
  type GlobalSecondaryIndex,
  type KeySchemaElement,
  type LocalSecondaryIndex,
  ResourceInUseException,
  ResourceNotFoundException,
  type TableDescription,
} from '@aws-sdk/client-dynamodb';

import { sleep } from './batch.js';
import { TheorydbError } from './errors.js';
import type { KeyType, Model } from './model.js';

export type BillingMode = 'PAY_PER_REQUEST' | 'PROVISIONED';

export interface ProvisionedThroughput {
  readCapacityUnits: number;
  writeCapacityUnits: number;
}

export interface CreateTableOptions {
  tableName?: string;
  billingMode?: BillingMode;
  provisionedThroughput?: ProvisionedThroughput;
  waitForActive?: boolean;
  waitTimeoutSeconds?: number;
  pollIntervalMs?: number;
}

export interface DeleteTableOptions {
  tableName?: string;
  waitForDelete?: boolean;
  waitTimeoutSeconds?: number;
  pollIntervalMs?: number;
  ignoreMissing?: boolean;
}

export interface DescribeTableOptions {
  tableName?: string;
}

export async function createTable(
  ddb: DynamoDBClient,
  model: Model,
  opts: CreateTableOptions = {},
): Promise<void> {
  const tableName = opts.tableName ?? model.tableName;
  const input = buildCreateTableInput(model, {
    tableName,
    ...(opts.billingMode ? { billingMode: opts.billingMode } : {}),
    ...(opts.provisionedThroughput
      ? { provisionedThroughput: opts.provisionedThroughput }
      : {}),
  });

  try {
    await ddb.send(new CreateTableCommand(input));
  } catch (err) {
    if (!isResourceInUse(err)) throw err;
  }

  if (opts.waitForActive ?? true) {
    const waitOpts: { timeoutSeconds?: number; pollIntervalMs?: number } = {};
    if (opts.waitTimeoutSeconds !== undefined)
      waitOpts.timeoutSeconds = opts.waitTimeoutSeconds;
    if (opts.pollIntervalMs !== undefined)
      waitOpts.pollIntervalMs = opts.pollIntervalMs;
    await waitForTableActive(ddb, tableName, waitOpts);
  }
}

export async function ensureTable(
  ddb: DynamoDBClient,
  model: Model,
  opts: CreateTableOptions = {},
): Promise<void> {
  const tableName = opts.tableName ?? model.tableName;
  try {
    await describeTable(ddb, model, { tableName });
  } catch (err) {
    if (!isResourceNotFound(err) && !isTableNotFound(err)) throw err;
    await createTable(ddb, model, opts);
    return;
  }

  if (opts.waitForActive ?? true) {
    const waitOpts: { timeoutSeconds?: number; pollIntervalMs?: number } = {};
    if (opts.waitTimeoutSeconds !== undefined)
      waitOpts.timeoutSeconds = opts.waitTimeoutSeconds;
    if (opts.pollIntervalMs !== undefined)
      waitOpts.pollIntervalMs = opts.pollIntervalMs;
    await waitForTableActive(ddb, tableName, waitOpts);
  }
}

export async function deleteTable(
  ddb: DynamoDBClient,
  model: Model,
  opts: DeleteTableOptions = {},
): Promise<void> {
  const tableName = opts.tableName ?? model.tableName;
  try {
    await ddb.send(new DeleteTableCommand({ TableName: tableName }));
  } catch (err) {
    if (opts.ignoreMissing && isResourceNotFound(err)) return;
    throw err;
  }

  if (opts.waitForDelete ?? true) {
    const waitOpts: { timeoutSeconds?: number; pollIntervalMs?: number } = {};
    if (opts.waitTimeoutSeconds !== undefined)
      waitOpts.timeoutSeconds = opts.waitTimeoutSeconds;
    if (opts.pollIntervalMs !== undefined)
      waitOpts.pollIntervalMs = opts.pollIntervalMs;
    await waitForTableDeleted(ddb, tableName, waitOpts);
  }
}

export async function describeTable(
  ddb: DynamoDBClient,
  model: Model,
  opts: DescribeTableOptions = {},
): Promise<TableDescription> {
  const tableName = opts.tableName ?? model.tableName;
  try {
    const resp = await ddb.send(
      new DescribeTableCommand({ TableName: tableName }),
    );
    if (!resp.Table) {
      throw new TheorydbError(
        'ErrTableNotFound',
        `Table not found: ${tableName}`,
      );
    }
    return resp.Table;
  } catch (err) {
    if (isResourceNotFound(err)) {
      throw new TheorydbError(
        'ErrTableNotFound',
        `Table not found: ${tableName}`,
        {
          cause: err,
        },
      );
    }
    throw err;
  }
}

function buildCreateTableInput(
  model: Model,
  opts: {
    tableName: string;
    billingMode?: BillingMode;
    provisionedThroughput?: ProvisionedThroughput;
  },
): CreateTableCommandInput {
  const billingMode = opts.billingMode ?? 'PAY_PER_REQUEST';
  if (billingMode === 'PROVISIONED' && !opts.provisionedThroughput) {
    throw new TheorydbError(
      'ErrInvalidOperator',
      'provisionedThroughput is required when billingMode=PROVISIONED',
    );
  }

  const keySchema: KeySchemaElement[] = [
    {
      AttributeName: model.schema.keys.partition.attribute,
      KeyType: 'HASH',
    },
  ];
  if (model.schema.keys.sort) {
    keySchema.push({
      AttributeName: model.schema.keys.sort.attribute,
      KeyType: 'RANGE',
    });
  }

  const attributeTypes = new Map<string, KeyType>();
  attributeTypes.set(
    model.schema.keys.partition.attribute,
    model.schema.keys.partition.type,
  );
  if (model.schema.keys.sort) {
    attributeTypes.set(
      model.schema.keys.sort.attribute,
      model.schema.keys.sort.type,
    );
  }
  for (const idx of model.schema.indexes ?? []) {
    if (
      idx.type === 'LSI' &&
      idx.partition.attribute !== model.schema.keys.partition.attribute
    ) {
      throw new TheorydbError(
        'ErrInvalidModel',
        `LSI partition key must match table partition key: ${idx.name}`,
      );
    }
    attributeTypes.set(idx.partition.attribute, idx.partition.type);
    if (idx.sort) attributeTypes.set(idx.sort.attribute, idx.sort.type);
  }

  const attributeDefinitions: AttributeDefinition[] = [];
  for (const [name, type] of attributeTypes) {
    attributeDefinitions.push({
      AttributeName: name,
      AttributeType: type,
    });
  }
  attributeDefinitions.sort((a, b) => {
    const aa = a.AttributeName ?? '';
    const bb = b.AttributeName ?? '';
    if (aa < bb) return -1;
    if (aa > bb) return 1;
    return 0;
  });

  const gsis: GlobalSecondaryIndex[] = [];
  const lsis: LocalSecondaryIndex[] = [];
  for (const idx of model.schema.indexes ?? []) {
    const projectionType = idx.projection?.type ?? 'ALL';
    const nonKeyAttributes =
      projectionType === 'INCLUDE' ? (idx.projection?.fields ?? []) : undefined;

    const schema: KeySchemaElement[] = [
      { AttributeName: idx.partition.attribute, KeyType: 'HASH' },
    ];
    if (idx.sort)
      schema.push({ AttributeName: idx.sort.attribute, KeyType: 'RANGE' });

    if (idx.type === 'GSI') {
      gsis.push({
        IndexName: idx.name,
        KeySchema: schema,
        Projection: {
          ProjectionType: projectionType,
          NonKeyAttributes: nonKeyAttributes,
        },
        ...(billingMode === 'PROVISIONED'
          ? {
              ProvisionedThroughput: {
                ReadCapacityUnits:
                  opts.provisionedThroughput!.readCapacityUnits,
                WriteCapacityUnits:
                  opts.provisionedThroughput!.writeCapacityUnits,
              },
            }
          : {}),
      });
    } else {
      lsis.push({
        IndexName: idx.name,
        KeySchema: schema,
        Projection: {
          ProjectionType: projectionType,
          NonKeyAttributes: nonKeyAttributes,
        },
      });
    }
  }

  return {
    TableName: opts.tableName,
    BillingMode: billingMode,
    KeySchema: keySchema,
    AttributeDefinitions: attributeDefinitions,
    ...(gsis.length ? { GlobalSecondaryIndexes: gsis } : {}),
    ...(lsis.length ? { LocalSecondaryIndexes: lsis } : {}),
    ...(billingMode === 'PROVISIONED'
      ? {
          ProvisionedThroughput: {
            ReadCapacityUnits: opts.provisionedThroughput!.readCapacityUnits,
            WriteCapacityUnits: opts.provisionedThroughput!.writeCapacityUnits,
          },
        }
      : {}),
  };
}

async function waitForTableActive(
  ddb: DynamoDBClient,
  tableName: string,
  opts: { timeoutSeconds?: number; pollIntervalMs?: number } = {},
): Promise<void> {
  const timeoutSeconds = opts.timeoutSeconds ?? 300;
  const pollIntervalMs = opts.pollIntervalMs ?? 250;
  const deadline = Date.now() + timeoutSeconds * 1000;

  while (Date.now() < deadline) {
    try {
      const resp = await ddb.send(
        new DescribeTableCommand({ TableName: tableName }),
      );
      if (resp.Table?.TableStatus === 'ACTIVE') return;
    } catch (err) {
      if (!isResourceNotFound(err)) throw err;
    }
    await sleep(pollIntervalMs);
  }

  throw new TheorydbError(
    'ErrInvalidOperator',
    `Timed out waiting for table ACTIVE: ${tableName}`,
  );
}

async function waitForTableDeleted(
  ddb: DynamoDBClient,
  tableName: string,
  opts: { timeoutSeconds?: number; pollIntervalMs?: number } = {},
): Promise<void> {
  const timeoutSeconds = opts.timeoutSeconds ?? 300;
  const pollIntervalMs = opts.pollIntervalMs ?? 250;
  const deadline = Date.now() + timeoutSeconds * 1000;

  while (Date.now() < deadline) {
    try {
      await ddb.send(new DescribeTableCommand({ TableName: tableName }));
    } catch (err) {
      if (isResourceNotFound(err)) return;
      throw err;
    }
    await sleep(pollIntervalMs);
  }

  throw new TheorydbError(
    'ErrInvalidOperator',
    `Timed out waiting for table deletion: ${tableName}`,
  );
}

function isResourceNotFound(err: unknown): boolean {
  if (err instanceof ResourceNotFoundException) return true;
  if (typeof err === 'object' && err !== null && 'name' in err) {
    return (err as { name?: unknown }).name === 'ResourceNotFoundException';
  }
  return false;
}

function isResourceInUse(err: unknown): boolean {
  if (err instanceof ResourceInUseException) return true;
  if (typeof err === 'object' && err !== null && 'name' in err) {
    return (err as { name?: unknown }).name === 'ResourceInUseException';
  }
  return false;
}

function isTableNotFound(err: unknown): boolean {
  if (err instanceof TheorydbError) return err.code === 'ErrTableNotFound';
  return false;
}
