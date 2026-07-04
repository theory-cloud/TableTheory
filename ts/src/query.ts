import {
  DynamoDBClient,
  QueryCommand,
  ScanCommand,
  type AttributeValue,
} from '@aws-sdk/client-dynamodb';

import { sleep } from './batch.js';
import type { AggregateResult } from './aggregates.js';
import { GroupByQuery } from './aggregates.js';
import {
  decodeCursor,
  encodeCursor,
  type Cursor,
  type CursorSort,
} from './cursor.js';
import { buildConditionExpression } from './expression-builder.js';
import { TheorydbError } from './errors.js';
import {
  aggregateFromAll,
  averageFromAll,
  countDistinctFromAll,
  groupByFromAll,
  maxFromAll,
  minFromAll,
  sumFromAll,
} from './query-aggregation.js';
import {
  decryptItemAttributes,
  modelHasEncryptedAttributes,
  type EncryptionProvider,
} from './encryption.js';
import {
  marshalScalar,
  unmarshalItem,
  type UnmarshalOptions,
} from './marshal.js';
import type { AttributeSchema, IndexSchema, Model } from './model.js';
import type { BuilderShape } from './optimizer.js';
import { mapConcurrent } from './query-concurrency.js';
import {
  collectAllItems,
  itemIterator,
  pageIterator,
} from './query-iterators.js';
import { countAllPages } from './query-count.js';
import type { SendOptions } from './send-options.js';
import type { Page, QueryOperator, QueryRetryOptions } from './query-types.js';
export { unsafeOperator } from './query-types.js';
export type {
  KnownOperator,
  OperatorEscape,
  Page,
  QueryOperator,
  QueryRetryOptions,
} from './query-types.js';

export interface FilterGroupBuilder {
  filter(field: string, op: QueryOperator, ...values: unknown[]): this;
  orFilter(field: string, op: QueryOperator, ...values: unknown[]): this;
  filterGroup(fn: (b: FilterGroupBuilder) => void): this;
  orFilterGroup(fn: (b: FilterGroupBuilder) => void): this;
}

class FilterExpressionBuilder implements FilterGroupBuilder {
  private readonly conditions: string[] = [];
  private readonly operators: Array<'AND' | 'OR'> = [];
  private readonly state: {
    nameCounter: number;
    valueCounter: number;
    names: Record<string, string>;
    values: Record<string, AttributeValue>;
    namePlaceholders: Map<string, string>;
  };

  constructor(
    private readonly model: Model,
    state?: FilterExpressionBuilder['state'],
  ) {
    this.state =
      state ??
      ({
        nameCounter: 0,
        valueCounter: 0,
        names: {},
        values: {},
        namePlaceholders: new Map<string, string>(),
      } satisfies FilterExpressionBuilder['state']);
  }

  filter(field: string, op: QueryOperator, ...values: unknown[]): this {
    this.addCondition('AND', field, op, values);
    return this;
  }

  orFilter(field: string, op: QueryOperator, ...values: unknown[]): this {
    this.addCondition('OR', field, op, values);
    return this;
  }

  filterGroup(fn: (b: FilterGroupBuilder) => void): this {
    return this.addGroup('AND', fn);
  }

  orFilterGroup(fn: (b: FilterGroupBuilder) => void): this {
    return this.addGroup('OR', fn);
  }

  build(): {
    expression?: string;
    names: Record<string, string>;
    values: Record<string, AttributeValue>;
  } {
    const expression = this.buildExpression();
    return {
      ...(expression ? { expression } : {}),
      names: this.state.names,
      values: this.state.values,
    };
  }

  private addGroup(
    op: 'AND' | 'OR',
    fn: (b: FilterGroupBuilder) => void,
  ): this {
    const sub = new FilterExpressionBuilder(this.model, this.state);
    fn(sub);
    const expr = sub.buildExpression();
    if (!expr) return this;
    this.append(op, `(${expr})`);
    return this;
  }

  private addCondition(
    logicalOp: 'AND' | 'OR',
    field: string,
    op: QueryOperator,
    values: unknown[],
  ): void {
    const schema = this.model.attributes.get(field);
    if (!schema) {
      throw new TheorydbError(
        'ErrInvalidOperator',
        `Unknown filter field: ${field}`,
      );
    }
    if (schema.encryption) {
      throw new TheorydbError(
        'ErrEncryptedFieldNotQueryable',
        `Encrypted fields cannot be filtered: ${field}`,
      );
    }

    const nameRef = this.nameRef(field);
    const upper = op.toUpperCase();

    const expr = this.buildConditionExpr(nameRef, schema, upper, values);
    this.append(logicalOp, expr);
  }

  private buildConditionExpr(
    nameRef: string,
    schema: Readonly<AttributeSchema>,
    op: string,
    values: unknown[],
  ): string {
    return buildConditionExpression(
      nameRef,
      schema,
      op,
      values,
      (valueSchema, value) => this.valueRef(valueSchema, value),
      {
        existsOperators: ['EXISTS', 'ATTRIBUTE_EXISTS'],
        notExistsOperators: ['NOT_EXISTS', 'ATTRIBUTE_NOT_EXISTS'],
        existsValueError: 'EXISTS does not take a value',
        notExistsValueError: 'NOT_EXISTS does not take a value',
      },
    );
  }

  private append(op: 'AND' | 'OR', expr: string): void {
    if (this.conditions.length > 0) this.operators.push(op);
    this.conditions.push(expr);
  }

  private buildExpression(): string {
    if (this.conditions.length === 0) return '';
    let out = this.conditions[0] ?? '';
    for (let i = 1; i < this.conditions.length; i++) {
      out += ` ${this.operators[i - 1]} ${this.conditions[i]}`;
    }
    return out;
  }

  private nameRef(field: string): string {
    const existing = this.state.namePlaceholders.get(field);
    if (existing) return existing;
    this.state.nameCounter += 1;
    const placeholder = `#f${this.state.nameCounter}`;
    this.state.names[placeholder] = field;
    this.state.namePlaceholders.set(field, placeholder);
    return placeholder;
  }

  private valueRef(schema: Readonly<AttributeSchema>, value: unknown): string {
    this.state.valueCounter += 1;
    const placeholder = `:f${this.state.valueCounter}`;
    this.state.values[placeholder] = marshalScalar(schema, value);
    return placeholder;
  }
}

function buildKeyConditionExpression(args: {
  condition: SortKeyCondition | undefined;
  names: Record<string, string>;
  values: Record<string, AttributeValue>;
  sortKeyName: string | undefined;
  sortKeySchema: Readonly<AttributeSchema> | undefined;
}): string {
  let keyExpr = '#pk = :pk';
  if (!args.condition) return keyExpr;

  const { sortKeyName, sortKeySchema } = args;
  if (!sortKeyName || !sortKeySchema) {
    throw new TheorydbError(
      'ErrInvalidOperator',
      'sortKey() requires a sort key',
    );
  }
  args.names['#sk'] = sortKeyName;

  const { op, values: sortValues } = args.condition;
  if (op === 'begins_with') {
    if (sortValues.length !== 1) {
      throw new TheorydbError(
        'ErrInvalidOperator',
        'begins_with requires one value',
      );
    }
    args.values[':sk'] = marshalScalar(sortKeySchema, sortValues[0]);
    return `${keyExpr} AND begins_with(#sk, :sk)`;
  }

  if (op === 'between') {
    if (sortValues.length !== 2) {
      throw new TheorydbError(
        'ErrInvalidOperator',
        'between requires two values',
      );
    }
    args.values[':sk0'] = marshalScalar(sortKeySchema, sortValues[0]);
    args.values[':sk1'] = marshalScalar(sortKeySchema, sortValues[1]);
    return `${keyExpr} AND #sk BETWEEN :sk0 AND :sk1`;
  }

  if (sortValues.length !== 1) {
    throw new TheorydbError(
      'ErrInvalidOperator',
      'sort operator requires one value',
    );
  }
  args.values[':sk'] = marshalScalar(sortKeySchema, sortValues[0]);
  return `${keyExpr} AND #sk ${op} :sk`;
}

type SortKeyCondition = {
  op: '=' | '<' | '<=' | '>' | '>=' | 'between' | 'begins_with';
  values: unknown[];
};

export class QueryBuilder<
  TItem extends Record<string, unknown> = Record<string, unknown>,
> {
  private indexName?: string;
  private pkValue?: unknown;
  private skCondition?: SortKeyCondition;
  private limitCount?: number;
  private projectionFields?: string[];
  private consistentReadEnabled = false;
  private cursorToken: string | undefined;
  private sortDir: CursorSort = 'ASC';
  private readonly filters: FilterExpressionBuilder;

  constructor(
    private readonly ddb: DynamoDBClient,
    private readonly model: Model,
    private readonly encryption?: EncryptionProvider,
    private readonly sendOptions?: SendOptions,
    private readonly unmarshalOptions: UnmarshalOptions = {},
  ) {
    this.filters = new FilterExpressionBuilder(model);
  }

  usingIndex(name: string): this {
    this.indexName = name;
    return this;
  }

  sort(direction: CursorSort): this {
    this.sortDir = direction;
    return this;
  }

  consistentRead(enabled = true): this {
    this.consistentReadEnabled = enabled;
    return this;
  }

  limit(n: number): this {
    this.limitCount = n;
    return this;
  }

  projection(fields: string[]): this {
    this.projectionFields = fields.slice();
    return this;
  }

  filter(field: string, op: QueryOperator, ...values: unknown[]): this {
    this.filters.filter(field, op, ...values);
    return this;
  }

  orFilter(field: string, op: QueryOperator, ...values: unknown[]): this {
    this.filters.orFilter(field, op, ...values);
    return this;
  }

  filterGroup(fn: (b: FilterGroupBuilder) => void): this {
    this.filters.filterGroup(fn);
    return this;
  }

  orFilterGroup(fn: (b: FilterGroupBuilder) => void): this {
    this.filters.orFilterGroup(fn);
    return this;
  }

  cursor(encoded: string): this {
    this.cursorToken = encoded;
    return this;
  }

  partitionKey(value: unknown): this {
    this.pkValue = value;
    return this;
  }

  sortKey(
    op: '=' | '<' | '<=' | '>' | '>=' | 'between' | 'begins_with',
    ...values: unknown[]
  ): this {
    this.skCondition = { op, values };
    return this;
  }

  async page(): Promise<Page<TItem>> {
    const { pkName, pkSchema, skName, skSchema, index } =
      this.resolveKeySchema();
    if (this.pkValue === undefined)
      throw new TheorydbError(
        'ErrInvalidOperator',
        'partitionKey() is required',
      );

    if (index?.type === 'GSI' && this.consistentReadEnabled) {
      throw new TheorydbError(
        'ErrInvalidOperator',
        'Consistent reads are not supported on GSIs',
      );
    }
    if (modelHasEncryptedAttributes(this.model) && !this.encryption) {
      throw new TheorydbError(
        'ErrEncryptionNotConfigured',
        `Encryption is required for model: ${this.model.name}`,
      );
    }

    const names: Record<string, string> = { '#pk': pkName };
    const values: Record<string, AttributeValue> = {
      ':pk': marshalScalar(pkSchema, this.pkValue),
    };

    const keyExpr = buildKeyConditionExpression({
      condition: this.skCondition,
      names,
      values,
      sortKeyName: skName,
      sortKeySchema: skSchema,
    });

    let projectionExpr: string | undefined;
    if (this.projectionFields?.length) {
      const projParts: string[] = [];
      for (let i = 0; i < this.projectionFields.length; i++) {
        const field = this.projectionFields[i]!;
        const placeholder = `#p${i}`;
        names[placeholder] = field;
        projParts.push(placeholder);
      }
      projectionExpr = projParts.join(', ');
    }

    const filter = this.filters.build();
    if (filter.expression) {
      for (const [k, v] of Object.entries(filter.names)) {
        if (k in names) {
          throw new TheorydbError(
            'ErrInvalidOperator',
            `ExpressionAttributeNames collision: ${k}`,
          );
        }
        names[k] = v;
      }
      for (const [k, v] of Object.entries(filter.values)) {
        if (k in values) {
          throw new TheorydbError(
            'ErrInvalidOperator',
            `ExpressionAttributeValues collision: ${k}`,
          );
        }
        values[k] = v;
      }
    }

    let exclusiveStartKey: Record<string, AttributeValue> | undefined;
    if (this.cursorToken) {
      const c = decodeCursor(this.cursorToken);
      if (c.index && (this.indexName ?? undefined) !== c.index) {
        throw new TheorydbError(
          'ErrInvalidOperator',
          'Cursor index does not match query',
        );
      }
      if (c.sort && c.sort !== this.sortDir) {
        throw new TheorydbError(
          'ErrInvalidOperator',
          'Cursor sort does not match query',
        );
      }
      exclusiveStartKey = c.lastKey;
    }

    const resp = await this.ddb.send(
      new QueryCommand({
        TableName: this.model.tableName,
        IndexName: index?.name,
        KeyConditionExpression: keyExpr,
        FilterExpression: filter.expression,
        ExpressionAttributeNames: names,
        ExpressionAttributeValues: values,
        Limit: this.limitCount,
        ProjectionExpression: projectionExpr,
        ConsistentRead: this.consistentReadEnabled || undefined,
        ExclusiveStartKey: exclusiveStartKey,
        ScanIndexForward: this.sortDir === 'ASC',
      }),
      this.sendOptions,
    );

    const rawItems = resp.Items ?? [];
    const items = modelHasEncryptedAttributes(this.model)
      ? (
          await Promise.all(
            rawItems.map((it) =>
              decryptItemAttributes(this.model, it, this.encryption!),
            ),
          )
        ).map((it) => unmarshalItem(this.model, it, this.unmarshalOptions))
      : rawItems.map((it) =>
          unmarshalItem(this.model, it, this.unmarshalOptions),
        );
    let cursor: string | undefined;
    if (resp.LastEvaluatedKey) {
      const c: Cursor = { lastKey: resp.LastEvaluatedKey, sort: this.sortDir };
      if (index) c.index = index.name;
      cursor = encodeCursor(c);
    }

    const page: Page<TItem> = { items: items as TItem[] };
    if (cursor) page.cursor = cursor;
    return page;
  }

  async count(): Promise<number> {
    return countAllPages(
      this.cursorToken,
      (cursor) => (this.cursorToken = cursor),
      () => this.countPage(),
    );
  }

  private async countPage(): Promise<{ count: number; cursor?: string }> {
    const { pkName, pkSchema, skName, skSchema, index } =
      this.resolveKeySchema();
    if (this.pkValue === undefined)
      throw new TheorydbError(
        'ErrInvalidOperator',
        'partitionKey() is required',
      );

    if (index?.type === 'GSI' && this.consistentReadEnabled) {
      throw new TheorydbError(
        'ErrInvalidOperator',
        'Consistent reads are not supported on GSIs',
      );
    }
    if (modelHasEncryptedAttributes(this.model) && !this.encryption) {
      throw new TheorydbError(
        'ErrEncryptionNotConfigured',
        `Encryption is required for model: ${this.model.name}`,
      );
    }

    const names: Record<string, string> = { '#pk': pkName };
    const values: Record<string, AttributeValue> = {
      ':pk': marshalScalar(pkSchema, this.pkValue),
    };

    const keyExpr = buildKeyConditionExpression({
      condition: this.skCondition,
      names,
      values,
      sortKeyName: skName,
      sortKeySchema: skSchema,
    });

    const filter = this.filters.build();
    if (filter.expression) {
      for (const [k, v] of Object.entries(filter.names)) {
        if (k in names) {
          throw new TheorydbError(
            'ErrInvalidOperator',
            `ExpressionAttributeNames collision: ${k}`,
          );
        }
        names[k] = v;
      }
      for (const [k, v] of Object.entries(filter.values)) {
        if (k in values) {
          throw new TheorydbError(
            'ErrInvalidOperator',
            `ExpressionAttributeValues collision: ${k}`,
          );
        }
        values[k] = v;
      }
    }

    let exclusiveStartKey: Record<string, AttributeValue> | undefined;
    if (this.cursorToken) {
      const c = decodeCursor(this.cursorToken);
      if (c.index && (this.indexName ?? undefined) !== c.index) {
        throw new TheorydbError(
          'ErrInvalidOperator',
          'Cursor index does not match query',
        );
      }
      if (c.sort && c.sort !== this.sortDir) {
        throw new TheorydbError(
          'ErrInvalidOperator',
          'Cursor sort does not match query',
        );
      }
      exclusiveStartKey = c.lastKey;
    }

    const resp = await this.ddb.send(
      new QueryCommand({
        TableName: this.model.tableName,
        IndexName: index?.name,
        KeyConditionExpression: keyExpr,
        FilterExpression: filter.expression,
        ExpressionAttributeNames: names,
        ExpressionAttributeValues: values,
        ConsistentRead: this.consistentReadEnabled || undefined,
        ExclusiveStartKey: exclusiveStartKey,
        ScanIndexForward: this.sortDir === 'ASC',
        Select: 'COUNT',
      }),
      this.sendOptions,
    );

    const out: { count: number; cursor?: string } = { count: resp.Count ?? 0 };
    if (resp.LastEvaluatedKey) {
      const c: Cursor = { lastKey: resp.LastEvaluatedKey, sort: this.sortDir };
      if (index) c.index = index.name;
      out.cursor = encodeCursor(c);
    }
    return out;
  }

  async all(): Promise<TItem[]> {
    return collectAllItems(
      () => this.cursorToken,
      (cursor) => {
        this.cursorToken = cursor;
      },
      () => this.page(),
    );
  }

  pages(): AsyncGenerator<Page<TItem>> {
    return pageIterator(
      () => this.cursorToken,
      (cursor) => {
        this.cursorToken = cursor;
      },
      () => this.page(),
    );
  }

  items(): AsyncGenerator<TItem> {
    return itemIterator(this.pages());
  }

  /**
   * Client-side aggregation: calls `all()` and materializes every matching item before summing.
   * Use only for bounded result sets; use native `count()` for count-only reads.
   */
  async sum(field: string): Promise<number> {
    return sumFromAll(() => this.all(), field);
  }

  /**
   * Client-side aggregation: calls `all()` and materializes every matching item before averaging.
   * Use only for bounded result sets; use native `count()` for count-only reads.
   */
  async average(field: string): Promise<number> {
    return averageFromAll(() => this.all(), field);
  }

  /**
   * Client-side aggregation: calls `all()` and materializes every matching item before selecting the minimum.
   * Use only for bounded result sets; use native `count()` for count-only reads.
   */
  async min(field: string): Promise<unknown> {
    return minFromAll(() => this.all(), field);
  }

  /**
   * Client-side aggregation: calls `all()` and materializes every matching item before selecting the maximum.
   * Use only for bounded result sets; use native `count()` for count-only reads.
   */
  async max(field: string): Promise<unknown> {
    return maxFromAll(() => this.all(), field);
  }

  /**
   * Client-side aggregation: calls `all()` and materializes every matching item before computing the aggregate.
   * Use only for bounded result sets; use native `count()` for count-only reads.
   */
  async aggregate(...fields: string[]): Promise<AggregateResult> {
    return aggregateFromAll(() => this.all(), fields[0]);
  }

  /**
   * Client-side aggregation: calls `all()` and materializes every matching item before counting distinct values.
   * Use only for bounded result sets; use native `count()` for count-only reads.
   */
  async countDistinct(field: string): Promise<number> {
    return countDistinctFromAll(() => this.all(), field);
  }

  /**
   * Client-side aggregation: the returned group query calls `all()` during `execute()` and keeps groups in memory.
   * Use only for bounded result sets; use native `count()` for count-only reads.
   */
  groupBy(field: string): GroupByQuery<TItem> {
    return groupByFromAll(() => this.all(), field);
  }

  describe(): BuilderShape {
    const { skName, index } = this.resolveKeySchema();
    const filters = this.filters.build();
    return {
      kind: 'query',
      modelName: this.model.name,
      tableName: this.model.tableName,
      ...(index?.name ? { indexName: index.name } : {}),
      ...(index?.type ? { indexType: index.type } : {}),
      hasPartitionKey: this.pkValue !== undefined,
      hasSortKey: skName !== undefined,
      hasSortKeyCondition: this.skCondition !== undefined,
      hasFilters: Boolean(filters.expression),
      ...(this.projectionFields
        ? { projections: this.projectionFields.slice() }
        : {}),
      consistentRead: this.consistentReadEnabled,
      sort: this.sortDir,
    };
  }

  async pageWithRetry(
    opts: QueryRetryOptions<TItem> = {},
  ): Promise<Page<TItem>> {
    const maxAttempts = opts.maxAttempts ?? 5;
    if (!Number.isInteger(maxAttempts) || maxAttempts <= 0) {
      throw new TheorydbError(
        'ErrInvalidOperator',
        'maxAttempts must be a positive integer',
      );
    }
    const retryOnEmpty = opts.retryOnEmpty ?? true;
    const retryOnError = opts.retryOnError ?? true;
    const verify = opts.verify;

    let delay = opts.baseDelayMs ?? 100;
    const maxDelay = opts.maxDelayMs ?? 5_000;
    const backoff = opts.backoffFactor ?? 2;

    let lastPage: Page<TItem> | undefined;
    for (let attempt = 1; attempt <= maxAttempts; attempt++) {
      try {
        const page = await this.page();
        lastPage = page;

        if (verify) {
          if (verify(page)) return page;
        } else if (!retryOnEmpty || page.items.length > 0) {
          return page;
        }
      } catch (err) {
        if (!retryOnError || attempt === maxAttempts) throw err;
      }

      if (attempt < maxAttempts) {
        if (delay > 0) await sleep(delay);
        delay = Math.min(maxDelay, Math.max(0, delay) * backoff);
      }
    }

    return lastPage ?? { items: [] };
  }

  private resolveKeySchema(): {
    pkName: string;
    pkSchema: Readonly<AttributeSchema>;
    skName?: string;
    skSchema?: Readonly<AttributeSchema>;
    index?: IndexSchema;
  } {
    if (this.indexName) {
      const index = this.model.indexes.get(this.indexName);
      if (!index)
        throw new TheorydbError(
          'ErrInvalidOperator',
          `Unknown index: ${this.indexName}`,
        );

      const pkName = index.partition.attribute;
      const pkSchema = this.model.attributes.get(pkName);
      if (!pkSchema)
        throw new TheorydbError(
          'ErrInvalidModel',
          `Index pk attribute missing: ${pkName}`,
        );

      const skName = index.sort?.attribute;
      const out: {
        pkName: string;
        pkSchema: Readonly<AttributeSchema>;
        skName?: string;
        skSchema?: Readonly<AttributeSchema>;
        index: IndexSchema;
      } = { pkName, pkSchema, index };

      if (skName) {
        const skSchema = this.model.attributes.get(skName);
        if (!skSchema)
          throw new TheorydbError(
            'ErrInvalidModel',
            `Index sk attribute missing: ${skName}`,
          );
        out.skName = skName;
        out.skSchema = skSchema;
      }

      return out;
    }

    const pkName = this.model.roles.pk;
    const pkSchema = this.model.attributes.get(pkName);
    if (!pkSchema)
      throw new TheorydbError(
        'ErrInvalidModel',
        `Model pk attribute missing: ${pkName}`,
      );

    const out: {
      pkName: string;
      pkSchema: Readonly<AttributeSchema>;
      skName?: string;
      skSchema?: Readonly<AttributeSchema>;
    } = { pkName, pkSchema };

    const skName = this.model.roles.sk;
    if (skName) {
      const skSchema = this.model.attributes.get(skName);
      if (!skSchema)
        throw new TheorydbError(
          'ErrInvalidModel',
          `Model sk attribute missing: ${skName}`,
        );
      out.skName = skName;
      out.skSchema = skSchema;
    }

    return out;
  }
}

export class ScanBuilder<
  TItem extends Record<string, unknown> = Record<string, unknown>,
> {
  private indexName?: string;
  private limitCount?: number;
  private projectionFields?: string[];
  private consistentReadEnabled = false;
  private cursorToken: string | undefined;
  private readonly filters: FilterExpressionBuilder;
  private segment?: number;
  private totalSegments?: number;

  constructor(
    private readonly ddb: DynamoDBClient,
    private readonly model: Model,
    private readonly encryption?: EncryptionProvider,
    private readonly sendOptions?: SendOptions,
    private readonly unmarshalOptions: UnmarshalOptions = {},
  ) {
    this.filters = new FilterExpressionBuilder(model);
  }

  usingIndex(name: string): this {
    this.indexName = name;
    return this;
  }

  consistentRead(enabled = true): this {
    this.consistentReadEnabled = enabled;
    return this;
  }

  limit(n: number): this {
    this.limitCount = n;
    return this;
  }

  projection(fields: string[]): this {
    this.projectionFields = fields.slice();
    return this;
  }

  filter(field: string, op: QueryOperator, ...values: unknown[]): this {
    this.filters.filter(field, op, ...values);
    return this;
  }

  orFilter(field: string, op: QueryOperator, ...values: unknown[]): this {
    this.filters.orFilter(field, op, ...values);
    return this;
  }

  filterGroup(fn: (b: FilterGroupBuilder) => void): this {
    this.filters.filterGroup(fn);
    return this;
  }

  orFilterGroup(fn: (b: FilterGroupBuilder) => void): this {
    this.filters.orFilterGroup(fn);
    return this;
  }

  cursor(encoded: string): this {
    this.cursorToken = encoded;
    return this;
  }

  parallelScan(segment: number, totalSegments: number): this {
    if (!Number.isInteger(segment) || segment < 0) {
      throw new TheorydbError(
        'ErrInvalidOperator',
        'parallelScan segment must be a non-negative integer',
      );
    }
    if (!Number.isInteger(totalSegments) || totalSegments <= 0) {
      throw new TheorydbError(
        'ErrInvalidOperator',
        'parallelScan totalSegments must be a positive integer',
      );
    }
    if (segment >= totalSegments) {
      throw new TheorydbError(
        'ErrInvalidOperator',
        'parallelScan segment must be < totalSegments',
      );
    }
    this.segment = segment;
    this.totalSegments = totalSegments;
    return this;
  }

  async scanAllSegments(
    totalSegments: number,
    opts: { concurrency?: number } = {},
  ): Promise<TItem[]> {
    const index = this.indexName
      ? this.model.indexes.get(this.indexName)
      : undefined;
    if (index?.type === 'GSI' && this.consistentReadEnabled) {
      throw new TheorydbError(
        'ErrInvalidOperator',
        'Consistent reads are not supported on GSIs',
      );
    }
    if (modelHasEncryptedAttributes(this.model) && !this.encryption) {
      throw new TheorydbError(
        'ErrEncryptionNotConfigured',
        `Encryption is required for model: ${this.model.name}`,
      );
    }
    if (!Number.isInteger(totalSegments) || totalSegments <= 0) {
      throw new TheorydbError(
        'ErrInvalidOperator',
        'totalSegments must be a positive integer',
      );
    }
    const concurrency =
      opts.concurrency === undefined
        ? totalSegments
        : Math.max(1, Math.floor(opts.concurrency));

    const baseNames: Record<string, string> = {};
    let projectionExpr: string | undefined;
    if (this.projectionFields?.length) {
      const projParts: string[] = [];
      for (let i = 0; i < this.projectionFields.length; i++) {
        const field = this.projectionFields[i]!;
        const placeholder = `#p${i}`;
        baseNames[placeholder] = field;
        projParts.push(placeholder);
      }
      projectionExpr = projParts.join(', ');
    }

    const filter = this.filters.build();
    if (filter.expression) {
      for (const [k, v] of Object.entries(filter.names)) {
        if (k in baseNames) {
          throw new TheorydbError(
            'ErrInvalidOperator',
            `ExpressionAttributeNames collision: ${k}`,
          );
        }
        baseNames[k] = v;
      }
    }

    const baseInput = {
      TableName: this.model.tableName,
      IndexName: this.indexName,
      Limit: this.limitCount,
      ProjectionExpression: projectionExpr,
      FilterExpression: filter.expression,
      ExpressionAttributeNames:
        Object.keys(baseNames).length > 0 ? baseNames : undefined,
      ExpressionAttributeValues:
        Object.keys(filter.values).length > 0 ? filter.values : undefined,
      ConsistentRead: this.consistentReadEnabled || undefined,
    };

    const segments = Array.from({ length: totalSegments }, (_, i) => i);
    const results = await mapConcurrent(
      segments,
      concurrency,
      async (segment) => {
        let start: Record<string, AttributeValue> | undefined;
        const items: Array<Record<string, unknown>> = [];
        let more = true;
        while (more) {
          const resp = await this.ddb.send(
            new ScanCommand({
              ...baseInput,
              Segment: segment,
              TotalSegments: totalSegments,
              ExclusiveStartKey: start,
            }),
            this.sendOptions,
          );

          const rawItems = resp.Items ?? [];
          const chunk = modelHasEncryptedAttributes(this.model)
            ? (
                await Promise.all(
                  rawItems.map((it) =>
                    decryptItemAttributes(this.model, it, this.encryption!),
                  ),
                )
              ).map((it) =>
                unmarshalItem(this.model, it, this.unmarshalOptions),
              )
            : rawItems.map((it) =>
                unmarshalItem(this.model, it, this.unmarshalOptions),
              );
          items.push(...chunk);

          start = resp.LastEvaluatedKey;
          more = start !== undefined;
        }
        return items;
      },
    );

    const out: TItem[] = [];
    for (const r of results) out.push(...(r as TItem[]));
    return out;
  }

  async count(): Promise<number> {
    return countAllPages(
      this.cursorToken,
      (cursor) => (this.cursorToken = cursor),
      () => this.countPage(),
    );
  }

  private async countPage(): Promise<{ count: number; cursor?: string }> {
    const index = this.indexName
      ? this.model.indexes.get(this.indexName)
      : undefined;
    if (index?.type === 'GSI' && this.consistentReadEnabled) {
      throw new TheorydbError(
        'ErrInvalidOperator',
        'Consistent reads are not supported on GSIs',
      );
    }
    if (modelHasEncryptedAttributes(this.model) && !this.encryption) {
      throw new TheorydbError(
        'ErrEncryptionNotConfigured',
        `Encryption is required for model: ${this.model.name}`,
      );
    }

    const names: Record<string, string> = {};
    let exclusiveStartKey: Record<string, AttributeValue> | undefined;
    if (this.cursorToken) {
      const c = decodeCursor(this.cursorToken);
      if (c.index && (this.indexName ?? undefined) !== c.index) {
        throw new TheorydbError(
          'ErrInvalidOperator',
          'Cursor index does not match scan',
        );
      }
      exclusiveStartKey = c.lastKey;
    }

    const filter = this.filters.build();
    if (filter.expression) {
      for (const [k, v] of Object.entries(filter.names)) {
        if (k in names) {
          throw new TheorydbError(
            'ErrInvalidOperator',
            `ExpressionAttributeNames collision: ${k}`,
          );
        }
        names[k] = v;
      }
    }

    if ((this.segment === undefined) !== (this.totalSegments === undefined)) {
      throw new TheorydbError(
        'ErrInvalidOperator',
        'parallelScan requires both segment and totalSegments',
      );
    }
    if (
      this.segment !== undefined &&
      this.totalSegments !== undefined &&
      (this.segment < 0 || this.segment >= this.totalSegments)
    ) {
      throw new TheorydbError(
        'ErrInvalidOperator',
        'parallelScan segment must be < totalSegments',
      );
    }

    const resp = await this.ddb.send(
      new ScanCommand({
        TableName: this.model.tableName,
        IndexName: this.indexName,
        FilterExpression: filter.expression,
        ExpressionAttributeNames: Object.keys(names).length ? names : undefined,
        ExpressionAttributeValues:
          Object.keys(filter.values).length > 0 ? filter.values : undefined,
        ConsistentRead: this.consistentReadEnabled || undefined,
        ExclusiveStartKey: exclusiveStartKey,
        Segment: this.segment,
        TotalSegments: this.totalSegments,
        Select: 'COUNT',
      }),
      this.sendOptions,
    );

    const out: { count: number; cursor?: string } = { count: resp.Count ?? 0 };
    if (resp.LastEvaluatedKey) {
      const c: Cursor = { lastKey: resp.LastEvaluatedKey };
      if (this.indexName) c.index = this.indexName;
      out.cursor = encodeCursor(c);
    }
    return out;
  }

  async all(): Promise<TItem[]> {
    return collectAllItems(
      () => this.cursorToken,
      (cursor) => {
        this.cursorToken = cursor;
      },
      () => this.page(),
    );
  }

  pages(): AsyncGenerator<Page<TItem>> {
    return pageIterator(
      () => this.cursorToken,
      (cursor) => {
        this.cursorToken = cursor;
      },
      () => this.page(),
    );
  }

  items(): AsyncGenerator<TItem> {
    return itemIterator(this.pages());
  }

  /**
   * Client-side aggregation: calls `all()` and materializes every matching item before summing.
   * Use only for bounded result sets; use native `count()` for count-only reads.
   */
  async sum(field: string): Promise<number> {
    return sumFromAll(() => this.all(), field);
  }

  /**
   * Client-side aggregation: calls `all()` and materializes every matching item before averaging.
   * Use only for bounded result sets; use native `count()` for count-only reads.
   */
  async average(field: string): Promise<number> {
    return averageFromAll(() => this.all(), field);
  }

  /**
   * Client-side aggregation: calls `all()` and materializes every matching item before selecting the minimum.
   * Use only for bounded result sets; use native `count()` for count-only reads.
   */
  async min(field: string): Promise<unknown> {
    return minFromAll(() => this.all(), field);
  }

  /**
   * Client-side aggregation: calls `all()` and materializes every matching item before selecting the maximum.
   * Use only for bounded result sets; use native `count()` for count-only reads.
   */
  async max(field: string): Promise<unknown> {
    return maxFromAll(() => this.all(), field);
  }

  /**
   * Client-side aggregation: calls `all()` and materializes every matching item before computing the aggregate.
   * Use only for bounded result sets; use native `count()` for count-only reads.
   */
  async aggregate(...fields: string[]): Promise<AggregateResult> {
    return aggregateFromAll(() => this.all(), fields[0]);
  }

  /**
   * Client-side aggregation: calls `all()` and materializes every matching item before counting distinct values.
   * Use only for bounded result sets; use native `count()` for count-only reads.
   */
  async countDistinct(field: string): Promise<number> {
    return countDistinctFromAll(() => this.all(), field);
  }

  /**
   * Client-side aggregation: the returned group query calls `all()` during `execute()` and keeps groups in memory.
   * Use only for bounded result sets; use native `count()` for count-only reads.
   */
  groupBy(field: string): GroupByQuery<TItem> {
    return groupByFromAll(() => this.all(), field);
  }

  describe(): BuilderShape {
    const index = this.indexName
      ? this.model.indexes.get(this.indexName)
      : undefined;
    const filters = this.filters.build();
    return {
      kind: 'scan',
      modelName: this.model.name,
      tableName: this.model.tableName,
      ...(this.indexName ? { indexName: this.indexName } : {}),
      ...(index?.type ? { indexType: index.type } : {}),
      hasFilters: Boolean(filters.expression),
      ...(this.projectionFields
        ? { projections: this.projectionFields.slice() }
        : {}),
      consistentRead: this.consistentReadEnabled,
      parallelScanConfigured:
        this.segment !== undefined || this.totalSegments !== undefined,
      ...(this.totalSegments !== undefined
        ? { totalSegments: this.totalSegments }
        : {}),
    };
  }

  async page(): Promise<Page<TItem>> {
    const index = this.indexName
      ? this.model.indexes.get(this.indexName)
      : undefined;
    if (index?.type === 'GSI' && this.consistentReadEnabled) {
      throw new TheorydbError(
        'ErrInvalidOperator',
        'Consistent reads are not supported on GSIs',
      );
    }
    if (modelHasEncryptedAttributes(this.model) && !this.encryption) {
      throw new TheorydbError(
        'ErrEncryptionNotConfigured',
        `Encryption is required for model: ${this.model.name}`,
      );
    }

    const names: Record<string, string> = {};
    let projectionExpr: string | undefined;
    if (this.projectionFields?.length) {
      const projParts: string[] = [];
      for (let i = 0; i < this.projectionFields.length; i++) {
        const field = this.projectionFields[i]!;
        const placeholder = `#p${i}`;
        names[placeholder] = field;
        projParts.push(placeholder);
      }
      projectionExpr = projParts.join(', ');
    }

    let exclusiveStartKey: Record<string, AttributeValue> | undefined;
    if (this.cursorToken) {
      const c = decodeCursor(this.cursorToken);
      if (c.index && (this.indexName ?? undefined) !== c.index) {
        throw new TheorydbError(
          'ErrInvalidOperator',
          'Cursor index does not match scan',
        );
      }
      exclusiveStartKey = c.lastKey;
    }

    const filter = this.filters.build();
    if (filter.expression) {
      for (const [k, v] of Object.entries(filter.names)) {
        if (k in names) {
          throw new TheorydbError(
            'ErrInvalidOperator',
            `ExpressionAttributeNames collision: ${k}`,
          );
        }
        names[k] = v;
      }
    }

    if ((this.segment === undefined) !== (this.totalSegments === undefined)) {
      throw new TheorydbError(
        'ErrInvalidOperator',
        'parallelScan requires both segment and totalSegments',
      );
    }
    if (
      this.segment !== undefined &&
      this.totalSegments !== undefined &&
      (this.segment < 0 || this.segment >= this.totalSegments)
    ) {
      throw new TheorydbError(
        'ErrInvalidOperator',
        'parallelScan segment must be < totalSegments',
      );
    }

    const resp = await this.ddb.send(
      new ScanCommand({
        TableName: this.model.tableName,
        IndexName: this.indexName,
        Limit: this.limitCount,
        ProjectionExpression: projectionExpr,
        FilterExpression: filter.expression,
        ExpressionAttributeNames: Object.keys(names).length ? names : undefined,
        ExpressionAttributeValues:
          Object.keys(filter.values).length > 0 ? filter.values : undefined,
        ConsistentRead: this.consistentReadEnabled || undefined,
        ExclusiveStartKey: exclusiveStartKey,
        Segment: this.segment,
        TotalSegments: this.totalSegments,
      }),
      this.sendOptions,
    );

    const rawItems = resp.Items ?? [];
    const items = modelHasEncryptedAttributes(this.model)
      ? (
          await Promise.all(
            rawItems.map((it) =>
              decryptItemAttributes(this.model, it, this.encryption!),
            ),
          )
        ).map((it) => unmarshalItem(this.model, it, this.unmarshalOptions))
      : rawItems.map((it) =>
          unmarshalItem(this.model, it, this.unmarshalOptions),
        );
    let cursor: string | undefined;
    if (resp.LastEvaluatedKey) {
      const c: Cursor = { lastKey: resp.LastEvaluatedKey };
      if (this.indexName) c.index = this.indexName;
      cursor = encodeCursor(c);
    }

    const page: Page<TItem> = { items: items as TItem[] };
    if (cursor) page.cursor = cursor;
    return page;
  }

  async pageWithRetry(
    opts: QueryRetryOptions<TItem> = {},
  ): Promise<Page<TItem>> {
    const maxAttempts = opts.maxAttempts ?? 5;
    if (!Number.isInteger(maxAttempts) || maxAttempts <= 0) {
      throw new TheorydbError(
        'ErrInvalidOperator',
        'maxAttempts must be a positive integer',
      );
    }
    const retryOnEmpty = opts.retryOnEmpty ?? true;
    const retryOnError = opts.retryOnError ?? true;
    const verify = opts.verify;

    let delay = opts.baseDelayMs ?? 100;
    const maxDelay = opts.maxDelayMs ?? 5_000;
    const backoff = opts.backoffFactor ?? 2;

    let lastPage: Page<TItem> | undefined;
    for (let attempt = 1; attempt <= maxAttempts; attempt++) {
      try {
        const page = await this.page();
        lastPage = page;

        if (verify) {
          if (verify(page)) return page;
        } else if (!retryOnEmpty || page.items.length > 0) {
          return page;
        }
      } catch (err) {
        if (!retryOnError || attempt === maxAttempts) throw err;
      }

      if (attempt < maxAttempts) {
        if (delay > 0) await sleep(delay);
        delay = Math.min(maxDelay, Math.max(0, delay) * backoff);
      }
    }

    return lastPage ?? { items: [] };
  }
}
