import {
  type AttributeValue,
  BatchGetItemCommand,
  BatchWriteItemCommand,
  DeleteItemCommand,
  DynamoDBClient,
  GetItemCommand,
  PutItemCommand,
  TransactWriteItemsCommand,
  UpdateItemCommand,
  type ConditionCheck,
  type Delete,
  type Put,
  type TransactWriteItem,
  type WriteRequest,
} from '@aws-sdk/client-dynamodb';

import {
  chunk,
  sleep,
  type BatchGetResult,
  type BatchWriteResult,
  type RetryOptions,
} from './batch.js';
import { mapDynamoError } from './dynamo-error.js';
import { TheorydbError } from './errors.js';
import type { Model } from './model.js';
import type { SendOptions } from './send-options.js';
import {
  isEmpty,
  marshalKey,
  marshalPutItem,
  marshalScalar,
  nowRfc3339Nano,
  unmarshalItem,
} from './marshal.js';
import { QueryBuilder, ScanBuilder } from './query.js';
import type { TransactAction } from './transaction.js';
import { UpdateBuilder } from './update-builder.js';
import {
  decryptItemAttributes,
  encryptAttributeValue,
  marshalPutItemEncrypted,
  modelHasEncryptedAttributes,
  type EncryptionProvider,
} from './encryption.js';

export class TheorydbClient {
  private readonly models = new Map<string, Model>();
  private encryption: EncryptionProvider | undefined;
  private readonly now: () => string;
  private readonly sendOptions: SendOptions | undefined;

  constructor(
    private readonly ddb: DynamoDBClient,
    opts: {
      encryption?: EncryptionProvider;
      now?: () => string;
      sendOptions?: SendOptions;
    } = {},
  ) {
    this.encryption = opts.encryption;
    this.now = opts.now ?? (() => nowRfc3339Nano());
    this.sendOptions = opts.sendOptions;
  }

  withEncryption(provider: EncryptionProvider): this {
    this.encryption = provider;
    return this;
  }

  withSendOptions(sendOptions?: SendOptions): TheorydbClient {
    const next = new TheorydbClient(this.ddb, {
      now: this.now,
      ...(this.encryption ? { encryption: this.encryption } : {}),
      ...(sendOptions ? { sendOptions } : {}),
    });
    next.register(...this.models.values());
    return next;
  }

  withDynamoDBClient(ddb: DynamoDBClient): TheorydbClient {
    const next = new TheorydbClient(ddb, {
      now: this.now,
      ...(this.encryption ? { encryption: this.encryption } : {}),
      ...(this.sendOptions ? { sendOptions: this.sendOptions } : {}),
    });
    next.register(...this.models.values());
    return next;
  }

  register(...models: Model[]): this {
    for (const model of models) {
      this.models.set(model.name, model);
    }
    return this;
  }

  private requireModel(name: string): Model {
    const model = this.models.get(name);
    if (!model)
      throw new TheorydbError('ErrInvalidModel', `Unknown model: ${name}`);
    return model;
  }

  private requireEncryption(model: Model): EncryptionProvider {
    const provider = this.encryption;
    if (!provider) {
      throw new TheorydbError(
        'ErrEncryptionNotConfigured',
        `Encryption is required for model: ${model.name}`,
      );
    }
    return provider;
  }

  async create(
    modelName: string,
    item: Record<string, unknown>,
    opts: { ifNotExists?: boolean } = {},
  ): Promise<void> {
    const model = this.requireModel(modelName);

    const now = this.now();
    const putItem = modelHasEncryptedAttributes(model)
      ? await marshalPutItemEncrypted(
          model,
          item,
          this.requireEncryption(model),
          {
            now,
          },
        )
      : marshalPutItem(model, item, { now });

    const cmd = new PutItemCommand({
      TableName: model.tableName,
      Item: putItem,
      ...(opts.ifNotExists
        ? {
            ConditionExpression: 'attribute_not_exists(#pk)',
            ExpressionAttributeNames: { '#pk': model.roles.pk },
          }
        : {}),
    });

    try {
      await this.ddb.send(cmd, this.sendOptions);
    } catch (err) {
      throw mapDynamoError(err);
    }
  }

  async get(
    modelName: string,
    key: Record<string, unknown>,
  ): Promise<Record<string, unknown>> {
    const model = this.requireModel(modelName);
    const provider = modelHasEncryptedAttributes(model)
      ? this.requireEncryption(model)
      : undefined;
    const cmd = new GetItemCommand({
      TableName: model.tableName,
      Key: marshalKey(model, key),
      ConsistentRead: true,
    });

    try {
      const resp = await this.ddb.send(cmd, this.sendOptions);
      if (!resp.Item)
        throw new TheorydbError('ErrItemNotFound', 'Item not found');
      const item = provider
        ? await decryptItemAttributes(model, resp.Item, provider)
        : resp.Item;
      return unmarshalItem(model, item);
    } catch (err) {
      throw mapDynamoError(err);
    }
  }

  async update(
    modelName: string,
    item: Record<string, unknown>,
    fields: string[],
  ): Promise<void> {
    const model = this.requireModel(modelName);
    const provider = modelHasEncryptedAttributes(model)
      ? this.requireEncryption(model)
      : undefined;
    const key = marshalKey(model, item);

    const versionAttr = model.roles.version;
    if (!versionAttr)
      throw new TheorydbError(
        'ErrInvalidModel',
        `Model ${model.name} does not define a version field`,
      );
    const currentVersion = item[versionAttr];
    if (
      currentVersion === undefined ||
      currentVersion === null ||
      currentVersion === ''
    ) {
      throw new TheorydbError(
        'ErrInvalidModel',
        `Update requires current version in field: ${versionAttr}`,
      );
    }

    const now = this.now();
    const names: Record<string, string> = {
      '#ver': versionAttr,
    };
    const values: Record<string, AttributeValue> = {
      ':expected': { N: String(currentVersion) },
      ':inc': { N: '1' },
    };

    const setParts: string[] = [];
    const removeParts: string[] = [];

    if (model.roles.updatedAt) {
      names['#updatedAt'] = model.roles.updatedAt;
      values[':now'] = { S: now };
      setParts.push('#updatedAt = :now');
    }

    for (const field of fields) {
      const fieldIndex = setParts.length + removeParts.length;
      if (field === model.roles.pk || field === model.roles.sk) {
        throw new TheorydbError(
          'ErrInvalidModel',
          `Cannot update primary key field: ${field}`,
        );
      }
      if (field === model.roles.createdAt) {
        throw new TheorydbError(
          'ErrInvalidModel',
          `Cannot update createdAt field: ${field}`,
        );
      }
      if (field === versionAttr) {
        throw new TheorydbError(
          'ErrInvalidModel',
          `Do not include version in update fields: ${field}`,
        );
      }

      const schema = model.attributes.get(field);
      if (!schema)
        throw new TheorydbError(
          'ErrInvalidModel',
          `Unknown field for model ${model.name}: ${field}`,
        );

      const value = item[field];
      if (value === undefined) {
        throw new TheorydbError(
          'ErrInvalidModel',
          `Missing update value for field: ${field}`,
        );
      }

      const placeholder = `#f${fieldIndex}`;
      names[placeholder] = field;

      if (schema.omit_empty && isEmpty(value)) {
        removeParts.push(placeholder);
        continue;
      }

      const valueKey = `:v${fieldIndex}`;
      values[valueKey] =
        schema.encryption !== undefined
          ? await encryptAttributeValue(schema, value, provider!, {
              model: model.name,
              attribute: field,
            })
          : marshalScalar(schema, value);
      setParts.push(`${placeholder} = ${valueKey}`);
    }

    const updateParts: string[] = [];
    if (setParts.length) updateParts.push(`SET ${setParts.join(', ')}`);
    if (removeParts.length)
      updateParts.push(`REMOVE ${removeParts.join(', ')}`);
    updateParts.push(`ADD #ver :inc`);

    const cmd = new UpdateItemCommand({
      TableName: model.tableName,
      Key: key,
      ConditionExpression: '#ver = :expected',
      UpdateExpression: updateParts.join(' '),
      ExpressionAttributeNames: names,
      ExpressionAttributeValues: values,
    });

    try {
      await this.ddb.send(cmd, this.sendOptions);
    } catch (err) {
      throw mapDynamoError(err);
    }
  }

  async delete(modelName: string, key: Record<string, unknown>): Promise<void> {
    const model = this.requireModel(modelName);
    if (modelHasEncryptedAttributes(model)) this.requireEncryption(model);
    const cmd = new DeleteItemCommand({
      TableName: model.tableName,
      Key: marshalKey(model, key),
    });

    try {
      await this.ddb.send(cmd, this.sendOptions);
    } catch (err) {
      throw mapDynamoError(err);
    }
  }

  async batchGet(
    modelName: string,
    keys: Array<Record<string, unknown>>,
    opts: RetryOptions & { consistentRead?: boolean } = {},
  ): Promise<BatchGetResult> {
    const model = this.requireModel(modelName);
    const provider = modelHasEncryptedAttributes(model)
      ? this.requireEncryption(model)
      : undefined;

    const maxAttempts = opts.maxAttempts ?? 5;
    const baseDelayMs = opts.baseDelayMs ?? 25;
    const consistentRead = opts.consistentRead ?? true;

    const allItems: Array<Record<string, unknown>> = [];
    const unprocessedKeys: Array<Record<string, AttributeValue>> = [];

    for (const keyChunk of chunk(keys, 100)) {
      let pending = keyChunk.map((k) => marshalKey(model, k));

      for (let attempt = 1; attempt <= maxAttempts; attempt++) {
        const resp = await this.ddb.send(
          new BatchGetItemCommand({
            RequestItems: {
              [model.tableName]: {
                Keys: pending,
                ConsistentRead: consistentRead,
              },
            },
          }),
          this.sendOptions,
        );

        const got = resp.Responses?.[model.tableName] ?? [];
        if (provider) {
          const decrypted = await Promise.all(
            got.map((it) => decryptItemAttributes(model, it, provider)),
          );
          allItems.push(...decrypted.map((it) => unmarshalItem(model, it)));
        } else {
          allItems.push(...got.map((it) => unmarshalItem(model, it)));
        }

        const next = resp.UnprocessedKeys?.[model.tableName]?.Keys ?? [];
        if (next.length === 0) {
          pending = [];
          break;
        }

        pending = next;
        if (attempt < maxAttempts) {
          await sleep(baseDelayMs * attempt);
        }
      }

      unprocessedKeys.push(...pending);
    }

    return { items: allItems, unprocessedKeys };
  }

  async batchWrite(
    modelName: string,
    req: {
      puts?: Array<Record<string, unknown>>;
      deletes?: Array<Record<string, unknown>>;
    },
    opts: RetryOptions = {},
  ): Promise<BatchWriteResult> {
    const model = this.requireModel(modelName);
    const provider = modelHasEncryptedAttributes(model)
      ? this.requireEncryption(model)
      : undefined;

    const maxAttempts = opts.maxAttempts ?? 5;
    const baseDelayMs = opts.baseDelayMs ?? 25;

    const now = this.now();
    const writeRequests: WriteRequest[] = [];

    for (const item of req.puts ?? []) {
      const marshaledItem = provider
        ? await marshalPutItemEncrypted(model, item, provider, { now })
        : marshalPutItem(model, item, { now });
      writeRequests.push({
        PutRequest: {
          Item: marshaledItem,
        },
      });
    }

    for (const key of req.deletes ?? []) {
      writeRequests.push({
        DeleteRequest: {
          Key: marshalKey(model, key),
        },
      });
    }

    const unprocessed: WriteRequest[] = [];

    for (const requestChunk of chunk(writeRequests, 25)) {
      let pending = requestChunk;

      for (let attempt = 1; attempt <= maxAttempts; attempt++) {
        const resp = await this.ddb.send(
          new BatchWriteItemCommand({
            RequestItems: {
              [model.tableName]: pending,
            },
          }),
          this.sendOptions,
        );

        const next = resp.UnprocessedItems?.[model.tableName] ?? [];
        if (next.length === 0) {
          pending = [];
          break;
        }

        pending = next;
        if (attempt < maxAttempts) {
          await sleep(baseDelayMs * attempt);
        }
      }

      unprocessed.push(...pending);
    }

    return { unprocessed };
  }

  async transactWrite(actions: TransactAction[]): Promise<void> {
    const transactItems: TransactWriteItem[] = [];

    for (const a of actions) {
      const model = this.requireModel(a.model);
      const provider = modelHasEncryptedAttributes(model)
        ? this.requireEncryption(model)
        : undefined;

      switch (a.kind) {
        case 'put': {
          const item = provider
            ? await marshalPutItemEncrypted(model, a.item, provider)
            : marshalPutItem(model, a.item);
          const put: Put = {
            TableName: model.tableName,
            Item: item,
          };
          if (a.ifNotExists) {
            put.ConditionExpression = 'attribute_not_exists(#pk)';
            put.ExpressionAttributeNames = { '#pk': model.roles.pk };
          }
          transactItems.push({ Put: put });
          break;
        }
        case 'delete':
          transactItems.push({
            Delete: {
              TableName: model.tableName,
              Key: marshalKey(model, a.key),
            } satisfies Delete,
          });
          break;
        case 'condition':
          transactItems.push({
            ConditionCheck: {
              TableName: model.tableName,
              Key: marshalKey(model, a.key),
              ConditionExpression: a.conditionExpression,
              ExpressionAttributeNames: a.expressionAttributeNames,
              ExpressionAttributeValues: a.expressionAttributeValues,
            } satisfies ConditionCheck,
          });
          break;
        default:
          throw new TheorydbError(
            'ErrInvalidOperator',
            'Unknown transaction action',
          );
      }
    }

    try {
      await this.ddb.send(
        new TransactWriteItemsCommand({ TransactItems: transactItems }),
        this.sendOptions,
      );
    } catch (err) {
      throw mapDynamoError(err);
    }
  }

  query(modelName: string): QueryBuilder {
    const model = this.requireModel(modelName);
    return new QueryBuilder(this.ddb, model, this.encryption, this.sendOptions);
  }

  scan(modelName: string): ScanBuilder {
    const model = this.requireModel(modelName);
    return new ScanBuilder(this.ddb, model, this.encryption, this.sendOptions);
  }

  updateBuilder(
    modelName: string,
    key: Record<string, unknown>,
  ): UpdateBuilder {
    const model = this.requireModel(modelName);
    return new UpdateBuilder(
      this.ddb,
      model,
      key,
      this.encryption,
      this.sendOptions,
    );
  }
}
