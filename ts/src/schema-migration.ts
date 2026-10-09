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
import { encodeEncryptedPayload } from './encryption-avjson.js';
import type { EncryptionProvider } from './encryption.js';
import { TheorydbError } from './errors.js';
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
  /** Provider used to encrypt plaintext attributes the target declares encrypted. */
  encryption?: EncryptionProvider;
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

  const targetEnc = encryptedNames(targetModel);
  const srcEnc = encryptedNames(sourceModel);

  // A migration that copies no data cannot write plaintext into the target, so
  // the encryption preflight applies only when a copy will actually happen.
  if (
    opts.dataCopy === true &&
    sourceModel.tableName !== targetModel.tableName
  ) {
    const needsEncryption = new Set(
      [...targetEnc].filter((name) => !srcEnc.has(name)),
    );
    if (needsEncryption.size > 0 && !opts.encryption) {
      throw new TheorydbError(
        'ErrMigrationEncryptionRequired',
        'migration target model "' +
          targetModel.name +
          '" requires encryption for attribute(s) ' +
          [...needsEncryption].sort().join(', ') +
          '; supply AutoMigrateOptions.encryption',
      );
    }
  }

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
      {
        srcEnc,
        targetEnc,
        provider: opts.encryption,
        targetModelName: targetModel.name,
      },
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

interface MigrationEncryptionContext {
  srcEnc: ReadonlySet<string>;
  targetEnc: ReadonlySet<string>;
  provider: EncryptionProvider | undefined;
  targetModelName: string;
}

async function copyData(
  ddb: DynamoDBClient,
  sourceTable: string,
  targetTable: string,
  transform: MigrationTransform | undefined,
  batchSize: number,
  encryption?: MigrationEncryptionContext,
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
      const requests: WriteRequest[] = [];
      for (const sourceItem of items) {
        let item: MigrationItem = transform
          ? transform(sourceItem)
          : sourceItem;
        if (encryption) {
          item = await guardMigrationItem(
            item,
            encryption.srcEnc,
            encryption.targetEnc,
            encryption.provider,
            encryption.targetModelName,
          );
        }
        requests.push({ PutRequest: { Item: item } });
      }
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

function encryptedNames(model: Model): ReadonlySet<string> {
  const names = new Set<string>();
  for (const attr of model.schema.attributes) {
    if (attr.encryption !== undefined && attr.encryption !== null) {
      names.add(attr.attribute);
    }
  }
  return names;
}

async function guardMigrationItem(
  item: MigrationItem,
  srcEnc: ReadonlySet<string>,
  targetEnc: ReadonlySet<string>,
  provider: EncryptionProvider | undefined,
  targetModelName: string,
): Promise<MigrationItem> {
  for (const name of [...srcEnc].sort()) {
    const value = item[name];
    if (value === undefined) continue;
    if (!isEncryptedEnvelope(value)) {
      throw new TheorydbError(
        'ErrMigrationEncryptionRequired',
        'migration attribute "' +
          name +
          '" is declared encrypted in the source model but the copied value is not an encrypted envelope',
      );
    }
  }

  for (const name of [...srcEnc].filter((n) => !targetEnc.has(n)).sort()) {
    const value = item[name];
    if (value !== undefined && isEncryptedEnvelope(value)) {
      throw new TheorydbError(
        'ErrMigrationEncryptionRequired',
        'migration cannot copy encrypted attribute "' +
          name +
          '" into a target attribute that is not encrypted',
      );
    }
  }

  for (const name of [...targetEnc].filter((n) => !srcEnc.has(n)).sort()) {
    const value = item[name];
    if (value === undefined) continue;
    if (isEncryptedEnvelope(value)) {
      throw new TheorydbError(
        'ErrMigrationEncryptionRequired',
        'migration cannot re-encrypt attribute "' +
          name +
          '": the value is already an encrypted envelope under a different attribute name',
      );
    }
    item[name] = await encryptRawAttributeValue(
      value,
      name,
      provider,
      targetModelName,
    );
  }

  return item;
}

async function encryptRawAttributeValue(
  av: AttributeValue,
  attrName: string,
  provider: EncryptionProvider | undefined,
  modelName: string,
): Promise<AttributeValue> {
  if (!provider) {
    throw new TheorydbError(
      'ErrMigrationEncryptionRequired',
      'migration target model "' +
        modelName +
        '" requires encryption for attribute(s) ' +
        attrName +
        '; supply AutoMigrateOptions.encryption',
    );
  }

  const bytes = encodeEncryptedPayload(av);
  const env = await provider.encrypt(bytes, {
    model: modelName,
    attribute: attrName,
  });
  if (env.v !== 1) {
    throw new TheorydbError(
      'ErrInvalidEncryptedEnvelope',
      'Unsupported envelope version',
    );
  }

  return {
    M: {
      v: { N: '1' },
      edk: { B: env.edk },
      nonce: { B: env.nonce },
      ct: { B: env.ct },
    },
  };
}

function isEncryptedEnvelope(av: unknown): boolean {
  if (typeof av !== 'object' || av === null) return false;
  const map = (av as { M?: unknown }).M;
  if (typeof map !== 'object' || map === null) return false;
  const envelope = map as Record<string, unknown>;

  const version = envelope.v;
  if (typeof version !== 'object' || version === null) return false;
  if ((version as { N?: unknown }).N !== '1') return false;

  for (const key of ['edk', 'nonce', 'ct']) {
    const field = envelope[key];
    if (typeof field !== 'object' || field === null) return false;
    const bytes = (field as { B?: unknown }).B;
    if (!(bytes instanceof Uint8Array) || bytes.length === 0) return false;
  }
  return true;
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
