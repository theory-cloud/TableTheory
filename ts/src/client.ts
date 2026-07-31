import {
  type AttributeValue,
  BatchGetItemCommand,
  BatchWriteItemCommand,
  DeleteItemCommand,
  DynamoDBClient,
  GetItemCommand,
  PutItemCommand,
  TransactGetItemsCommand,
  TransactWriteItemsCommand,
  UpdateItemCommand,
  type ConditionCheck,
  type Delete,
  type Put,
  type TransactGetItem,
  type Update,
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
import { hasTheorydbErrorCode, TheorydbError } from './errors.js';
import type { Model, ModelItem } from './model.js';
import type { SendOptions } from './send-options.js';
import {
  isEmptyAttribute,
  marshalKey,
  marshalPutItem,
  marshalScalar,
  nowRfc3339Nano,
  unmarshalItem,
  type NumberUnmarshalMode,
  type UnmarshalOptions,
} from './marshal.js';
import { QueryBuilder, ScanBuilder } from './query.js';
import type {
  TransactAction,
  TransactGetAction,
  TransactModelUpdate,
} from './transaction.js';
import { UpdateBuilder } from './update-builder.js';
import {
  decryptItemAttributes,
  encryptAttributeValue,
  marshalPutItemEncrypted,
  modelHasEncryptedAttributes,
  type EncryptionProvider,
} from './encryption.js';
import {
  assertMutableWritePolicy,
  assertProtectedFieldsCanMutate,
  assertRawUpdateExpressionAllowed,
  isWriteOnceModel,
  type WritePolicyOptions,
} from './write-policy.js';

export interface TheorydbClientOptions {
  encryption?: EncryptionProvider;
  now?: () => string;
  sendOptions?: SendOptions;
  /**
   * Controls how DynamoDB N/NS values are unmarshaled.
   *
   * The default, 'string', returns canonical DynamoDB decimal strings so reads
   * are precision-safe for integers outside the safe range and high-precision
   * decimals. Use 'number' only when lossy JavaScript Number conversion is
   * acceptable for a specific client.
   */
  numberUnmarshalMode?: NumberUnmarshalMode;
}

type ModelKey<TItem extends Record<string, unknown>> = Partial<TItem>;
type ModelField<TItem extends Record<string, unknown>> = Extract<
  keyof TItem,
  string
>;

export class ModelRepository<
  TItem extends Record<string, unknown> = Record<string, unknown>,
> {
  constructor(
    private readonly client: TheorydbClient,
    private readonly modelDef: Model<TItem>,
  ) {}

  create(item: TItem, opts: { ifNotExists?: boolean } = {}): Promise<void> {
    return this.client.create(this.modelDef.name, item, opts);
  }

  save(item: TItem): Promise<void> {
    return this.client.save(this.modelDef.name, item);
  }

  async get(key: ModelKey<TItem>): Promise<TItem> {
    return (await this.client.get(this.modelDef.name, key)) as TItem;
  }

  async getOrNull(key: ModelKey<TItem>): Promise<TItem | null> {
    return (await this.client.getOrNull(
      this.modelDef.name,
      key,
    )) as TItem | null;
  }

  update(
    item: TItem,
    fields: readonly ModelField<TItem>[],
    opts: WritePolicyOptions = {},
  ): Promise<void> {
    return this.client.update(this.modelDef.name, item, [...fields], opts);
  }

  delete(key: ModelKey<TItem>): Promise<void> {
    return this.client.delete(this.modelDef.name, key);
  }

  async batchGet(
    keys: Array<ModelKey<TItem>>,
    opts: RetryOptions & { consistentRead?: boolean } = {},
  ): Promise<BatchGetResult<TItem>> {
    const result = await this.client.batchGet(this.modelDef.name, keys, opts);
    return result as BatchGetResult<TItem>;
  }

  batchWrite(
    req: {
      puts?: TItem[];
      deletes?: Array<ModelKey<TItem>>;
    },
    opts: RetryOptions = {},
  ): Promise<BatchWriteResult> {
    return this.client.batchWrite(this.modelDef.name, req, opts);
  }

  query(): QueryBuilder<TItem> {
    return this.client.query(this.modelDef.name) as QueryBuilder<TItem>;
  }

  scan(): ScanBuilder<TItem> {
    return this.client.scan(this.modelDef.name) as ScanBuilder<TItem>;
  }

  updateBuilder(key: ModelKey<TItem>): UpdateBuilder {
    return this.client.updateBuilder(this.modelDef.name, key);
  }
}

export class TheorydbClient {
  private readonly models = new Map<string, Model>();
  private encryption: EncryptionProvider | undefined;
  private readonly now: () => string;
  private readonly sendOptions: SendOptions | undefined;
  private readonly unmarshalOptions: UnmarshalOptions;

  constructor(
    private readonly ddb: DynamoDBClient,
    opts: TheorydbClientOptions = {},
  ) {
    this.encryption = opts.encryption;
    this.now = opts.now ?? (() => nowRfc3339Nano());
    this.sendOptions = opts.sendOptions;
    this.unmarshalOptions = {
      numberMode: opts.numberUnmarshalMode ?? 'string',
    };
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
      numberUnmarshalMode: this.unmarshalOptions.numberMode ?? 'string',
    });
    next.register(...this.models.values());
    return next;
  }

  withDynamoDBClient(ddb: DynamoDBClient): TheorydbClient {
    const next = new TheorydbClient(ddb, {
      now: this.now,
      ...(this.encryption ? { encryption: this.encryption } : {}),
      ...(this.sendOptions ? { sendOptions: this.sendOptions } : {}),
      numberUnmarshalMode: this.unmarshalOptions.numberMode ?? 'string',
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

  model<M extends Model>(model: M): ModelRepository<ModelItem<M>>;
  model(modelName: string): ModelRepository<Record<string, unknown>>;
  model(modelOrName: string | Model): ModelRepository<Record<string, unknown>> {
    const model =
      typeof modelOrName === 'string'
        ? this.requireModel(modelOrName)
        : this.register(modelOrName).requireModel(modelOrName.name);
    return new ModelRepository(this, model);
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

    const requireIfNotExists =
      opts.ifNotExists || requiresCreateNotExistsCondition(model);
    const cmd = new PutItemCommand({
      TableName: model.tableName,
      Item: putItem,
      ...(requireIfNotExists
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

  async save(modelName: string, item: Record<string, unknown>): Promise<void> {
    const model = this.requireModel(modelName);
    assertMutableWritePolicy(model, 'save');
    assertNoProtectedOverwrite(model, 'save');

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
      return unmarshalItem(model, item, this.unmarshalOptions);
    } catch (err) {
      throw mapDynamoError(err);
    }
  }

  async getOrNull(
    modelName: string,
    key: Record<string, unknown>,
  ): Promise<Record<string, unknown> | null> {
    try {
      return await this.get(modelName, key);
    } catch (err) {
      if (hasTheorydbErrorCode(err, 'ErrItemNotFound')) return null;
      throw err;
    }
  }

  async update(
    modelName: string,
    item: Record<string, unknown>,
    fields: string[],
    opts: WritePolicyOptions = {},
  ): Promise<void> {
    const model = this.requireModel(modelName);
    assertMutableWritePolicy(model, 'update');
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

    assertProtectedFieldsCanMutate(
      model,
      policyFieldsForUpdate(model, fields),
      opts.protectedAttributes ?? [],
    );

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

      if (schema.omit_empty && isEmptyAttribute(schema, value)) {
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
      throw mapDynamoError(err, { versionConflict: true });
    }
  }

  async delete(modelName: string, key: Record<string, unknown>): Promise<void> {
    const model = this.requireModel(modelName);
    assertMutableWritePolicy(model, 'delete');
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
          allItems.push(
            ...decrypted.map((it) =>
              unmarshalItem(model, it, this.unmarshalOptions),
            ),
          );
        } else {
          allItems.push(
            ...got.map((it) => unmarshalItem(model, it, this.unmarshalOptions)),
          );
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
    if ((req.puts?.length ?? 0) > 0) {
      assertMutableWritePolicy(model, 'batch put');
      assertNoProtectedOverwrite(model, 'batch put');
    }
    if ((req.deletes?.length ?? 0) > 0) {
      assertMutableWritePolicy(model, 'batch delete');
    }
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

  async transactGet(
    actions: TransactGetAction[],
  ): Promise<Array<Record<string, unknown> | undefined>> {
    if (actions.length === 0) {
      throw new TheorydbError(
        'ErrInvalidOperator',
        'TransactGetItems requires at least one item',
      );
    }
    if (actions.length > 100) {
      throw new TheorydbError(
        'ErrInvalidOperator',
        'DynamoDB TransactGetItems supports up to 100 items',
      );
    }

    const models = actions.map((action) => this.requireModel(action.model));
    const transactItems: TransactGetItem[] = actions.map((action, index) => {
      const model = models[index]!;
      const get: NonNullable<TransactGetItem['Get']> = {
        TableName: model.tableName,
        Key: marshalKey(model, action.key),
      };
      if (action.projection && action.projection.length > 0) {
        const projection = buildProjectionExpression(model, action.projection);
        get.ProjectionExpression = projection.expression;
        get.ExpressionAttributeNames = projection.names;
      }
      return { Get: get };
    });

    let resp;
    try {
      resp = await this.ddb.send(
        new TransactGetItemsCommand({ TransactItems: transactItems }),
        this.sendOptions,
      );
    } catch (err) {
      throw mapDynamoError(err);
    }

    const responses = resp.Responses ?? [];
    return Promise.all(
      actions.map(async (_, index) => {
        const item = responses[index]?.Item;
        if (!item || Object.keys(item).length === 0) return undefined;
        const model = models[index]!;
        const provider = modelHasEncryptedAttributes(model)
          ? this.requireEncryption(model)
          : undefined;
        const plain = provider
          ? await decryptItemAttributes(model, item, provider)
          : item;
        return unmarshalItem(model, plain, this.unmarshalOptions);
      }),
    );
  }

  async transactWrite(actions: TransactAction[]): Promise<void> {
    if (actions.length > 100) {
      throw new TheorydbError(
        'ErrInvalidOperator',
        'DynamoDB TransactWriteItems supports up to 100 actions',
      );
    }

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
          if (a.ifNotExists || requiresCreateNotExistsCondition(model)) {
            put.ConditionExpression = 'attribute_not_exists(#pk)';
            put.ExpressionAttributeNames = { '#pk': model.roles.pk };
          }
          transactItems.push({ Put: put });
          break;
        }
        case 'update': {
          assertMutableWritePolicy(model, 'transaction update');
          let update: Update;
          if (isTransactModelUpdate(a)) {
            update = await buildTransactModelUpdate(
              this.ddb,
              model,
              a,
              provider,
              this.sendOptions,
              this.now(),
            );
          } else if (typeof a.updateFn === 'function') {
            const key = marshalKey(model, a.key);
            const builder = new UpdateBuilder(
              this.ddb,
              model,
              a.key,
              provider,
              this.sendOptions,
            );
            await a.updateFn(builder);
            const built = await builder.build();
            update = {
              TableName: model.tableName,
              Key: key,
              UpdateExpression: built.updateExpression,
              ConditionExpression: built.conditionExpression,
              ExpressionAttributeNames: built.expressionAttributeNames,
              ExpressionAttributeValues: built.expressionAttributeValues,
            };
          } else {
            const updateExpression = a.updateExpression;
            if (!updateExpression) {
              throw new TheorydbError(
                'ErrInvalidOperator',
                'Transaction update requires an updateExpression',
              );
            }
            assertRawUpdateExpressionAllowed(
              model,
              updateExpression,
              a.expressionAttributeNames,
            );
            if (provider) {
              throw new TheorydbError(
                'ErrInvalidModel',
                `Encrypted transaction updates for model ${model.name} must use updateFn or the model-based item + fields action`,
              );
            }
            update = {
              TableName: model.tableName,
              Key: marshalKey(model, a.key),
              UpdateExpression: updateExpression,
              ConditionExpression: a.conditionExpression,
              ExpressionAttributeNames: a.expressionAttributeNames,
              ExpressionAttributeValues: a.expressionAttributeValues,
            };
          }

          transactItems.push({ Update: update });
          break;
        }
        case 'delete':
          assertMutableWritePolicy(model, 'transaction delete');
          transactItems.push({
            Delete: {
              TableName: model.tableName,
              Key: marshalKey(model, a.key),
              ConditionExpression: a.conditionExpression,
              ExpressionAttributeNames: a.expressionAttributeNames,
              ExpressionAttributeValues: a.expressionAttributeValues,
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
    return new QueryBuilder(
      this.ddb,
      model,
      this.encryption,
      this.sendOptions,
      this.unmarshalOptions,
    );
  }

  scan(modelName: string): ScanBuilder {
    const model = this.requireModel(modelName);
    return new ScanBuilder(
      this.ddb,
      model,
      this.encryption,
      this.sendOptions,
      this.unmarshalOptions,
    );
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
      this.unmarshalOptions,
    );
  }
}

async function buildTransactModelUpdate(
  ddb: DynamoDBClient,
  model: Model,
  action: TransactModelUpdate,
  provider: EncryptionProvider | undefined,
  sendOptions: SendOptions | undefined,
  now: string,
): Promise<Update> {
  const builder = new UpdateBuilder(
    ddb,
    model,
    action.item,
    provider,
    sendOptions,
  );

  if (model.roles.updatedAt) {
    builder.set(model.roles.updatedAt, now);
  }

  for (const field of action.fields) {
    if (field === model.roles.createdAt || field === model.roles.updatedAt) {
      throw new TheorydbError(
        'ErrInvalidModel',
        `Cannot update lifecycle timestamp field: ${field}`,
      );
    }
    if (field === model.roles.version) {
      throw new TheorydbError(
        'ErrInvalidModel',
        `Do not include version in update fields: ${field}`,
      );
    }
    const schema = model.attributes.get(field);
    if (!schema) {
      throw new TheorydbError(
        'ErrInvalidModel',
        `Unknown field for model ${model.name}: ${field}`,
      );
    }

    const value = action.item[field];
    if (value === undefined) {
      throw new TheorydbError(
        'ErrInvalidModel',
        `Missing update value for field: ${field}`,
      );
    }

    if (schema.omit_empty && isEmptyAttribute(schema, value)) {
      builder.remove(field);
    } else {
      builder.set(field, value);
    }
  }

  const built = await builder.build();
  const names = mergeExpressionAttributes(
    'ExpressionAttributeNames',
    built.expressionAttributeNames,
    action.expressionAttributeNames,
  );
  const values = mergeExpressionAttributes(
    'ExpressionAttributeValues',
    built.expressionAttributeValues,
    action.expressionAttributeValues,
  );

  return {
    TableName: model.tableName,
    Key: marshalKey(model, action.item),
    UpdateExpression: built.updateExpression,
    ConditionExpression: action.conditionExpression,
    ExpressionAttributeNames: names,
    ExpressionAttributeValues: values,
  };
}

function mergeExpressionAttributes<T>(
  label: string,
  generated: Record<string, T> | undefined,
  supplied: Record<string, T> | undefined,
): Record<string, T> | undefined {
  if (!generated && !supplied) return undefined;

  const merged = { ...(generated ?? {}) };
  for (const [key, value] of Object.entries(supplied ?? {})) {
    if (key in merged) {
      throw new TheorydbError(
        'ErrInvalidOperator',
        `${label} collision: ${key}`,
      );
    }
    merged[key] = value;
  }
  return merged;
}

function isTransactModelUpdate(
  action: Extract<TransactAction, { kind: 'update' }>,
): action is TransactModelUpdate {
  return 'item' in action && Array.isArray(action.fields);
}

function policyFieldsForUpdate(
  model: Model,
  fields: readonly string[],
): string[] {
  const out = [...fields];
  if (model.roles.updatedAt) out.push(model.roles.updatedAt);
  if (model.roles.version) out.push(model.roles.version);
  return out;
}

function buildProjectionExpression(
  model: Model,
  fields: readonly string[],
): { expression: string; names: Record<string, string> } {
  const names: Record<string, string> = {};
  const parts: string[] = [];
  fields.forEach((field, index) => {
    const attr = model.attributes.get(field)?.attribute ?? field;
    const placeholder = `#p${index}`;
    names[placeholder] = attr;
    parts.push(placeholder);
  });
  return { expression: parts.join(', '), names };
}

function hasProtectedAttributes(model: Model): boolean {
  return model.writePolicy.protectedAttributes.length > 0;
}

function requiresCreateNotExistsCondition(model: Model): boolean {
  return isWriteOnceModel(model) || hasProtectedAttributes(model);
}

function assertNoProtectedOverwrite(model: Model, operation: string): void {
  if (!hasProtectedAttributes(model)) return;
  throw new TheorydbError('ErrProtectedFieldMutation', operation);
}
