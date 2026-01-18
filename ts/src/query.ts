import {
  DynamoDBClient,
  QueryCommand,
  ScanCommand,
  type AttributeValue,
} from '@aws-sdk/client-dynamodb';

import { sleep } from './batch.js';
import type { AggregateResult } from './aggregates.js';
import {
  aggregateField,
  averageField,
  countDistinct,
  GroupByQuery,
  maxField,
  minField,
  sumField,
} from './aggregates.js';
import {
  decodeCursor,
  encodeCursor,
  type Cursor,
  type CursorSort,
} from './cursor.js';
import { TheorydbError } from './errors.js';
import {
  decryptItemAttributes,
  modelHasEncryptedAttributes,
  type EncryptionProvider,
} from './encryption.js';
import { marshalScalar, unmarshalItem } from './marshal.js';
import type { AttributeSchema, IndexSchema, Model } from './model.js';
import type { BuilderShape } from './optimizer.js';
import type { SendOptions } from './send-options.js';

export interface Page<T = Record<string, unknown>> {
  items: T[];
  cursor?: string;
}

export interface QueryRetryOptions {
  maxAttempts?: number;
  baseDelayMs?: number;
  maxDelayMs?: number;
  backoffFactor?: number;
  retryOnEmpty?: boolean;
  retryOnError?: boolean;
  verify?: (page: Page) => boolean;
}

export interface FilterGroupBuilder {
  filter(field: string, op: string, ...values: unknown[]): this;
  orFilter(field: string, op: string, ...values: unknown[]): this;
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

  filter(field: string, op: string, ...values: unknown[]): this {
    this.addCondition('AND', field, op, values);
    return this;
  }

  orFilter(field: string, op: string, ...values: unknown[]): this {
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
    op: string,
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
    switch (op) {
      case '=':
      case 'EQ': {
        const valueRef = this.valueRef(schema, singleValue(values, op));
        return `${nameRef} = ${valueRef}`;
      }
      case '!=':
      case '<>':
      case 'NE': {
        const valueRef = this.valueRef(schema, singleValue(values, op));
        return `${nameRef} <> ${valueRef}`;
      }
      case '<':
      case 'LT': {
        const valueRef = this.valueRef(schema, singleValue(values, op));
        return `${nameRef} < ${valueRef}`;
      }
      case '<=':
      case 'LE': {
        const valueRef = this.valueRef(schema, singleValue(values, op));
        return `${nameRef} <= ${valueRef}`;
      }
      case '>':
      case 'GT': {
        const valueRef = this.valueRef(schema, singleValue(values, op));
        return `${nameRef} > ${valueRef}`;
      }
      case '>=':
      case 'GE': {
        const valueRef = this.valueRef(schema, singleValue(values, op));
        return `${nameRef} >= ${valueRef}`;
      }
      case 'BETWEEN': {
        if (values.length !== 2) {
          throw new TheorydbError(
            'ErrInvalidOperator',
            'BETWEEN requires two values',
          );
        }
        const left = this.valueRef(schema, values[0]);
        const right = this.valueRef(schema, values[1]);
        return `${nameRef} BETWEEN ${left} AND ${right}`;
      }
      case 'IN': {
        if (values.length !== 1 || !Array.isArray(values[0])) {
          throw new TheorydbError(
            'ErrInvalidOperator',
            'IN requires a single array value',
          );
        }
        const list = values[0];
        if (list.length > 100) {
          throw new TheorydbError(
            'ErrInvalidOperator',
            'IN supports maximum 100 values',
          );
        }
        const refs = list.map((v) => this.valueRef(schema, v));
        return `${nameRef} IN (${refs.join(', ')})`;
      }
      case 'BEGINS_WITH': {
        const valueRef = this.valueRef(schema, singleValue(values, op));
        return `begins_with(${nameRef}, ${valueRef})`;
      }
      case 'CONTAINS': {
        const valueRef = this.valueRef(schema, singleValue(values, op));
        return `contains(${nameRef}, ${valueRef})`;
      }
      case 'EXISTS':
      case 'ATTRIBUTE_EXISTS': {
        if (values.length !== 0) {
          throw new TheorydbError(
            'ErrInvalidOperator',
            'EXISTS does not take a value',
          );
        }
        return `attribute_exists(${nameRef})`;
      }
      case 'NOT_EXISTS':
      case 'ATTRIBUTE_NOT_EXISTS': {
        if (values.length !== 0) {
          throw new TheorydbError(
            'ErrInvalidOperator',
            'NOT_EXISTS does not take a value',
          );
        }
        return `attribute_not_exists(${nameRef})`;
      }
      default:
        throw new TheorydbError(
          'ErrInvalidOperator',
          `Unsupported operator: ${op}`,
        );
    }
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

function singleValue(values: unknown[], op: string): unknown {
  if (values.length !== 1) {
    throw new TheorydbError('ErrInvalidOperator', `${op} requires one value`);
  }
  return values[0];
}

export class QueryBuilder {
  private indexName?: string;
  private pkValue?: unknown;
  private skCondition?: {
    op: '=' | '<' | '<=' | '>' | '>=' | 'between' | 'begins_with';
    values: unknown[];
  };
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

  filter(field: string, op: string, ...values: unknown[]): this {
    this.filters.filter(field, op, ...values);
    return this;
  }

  orFilter(field: string, op: string, ...values: unknown[]): this {
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

  async page(): Promise<Page> {
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

    let keyExpr = '#pk = :pk';
    if (this.skCondition) {
      if (!skName || !skSchema)
        throw new TheorydbError(
          'ErrInvalidOperator',
          'sortKey() requires a sort key',
        );
      names['#sk'] = skName;

      const { op, values: skValues } = this.skCondition;
      switch (op) {
        case 'begins_with': {
          if (skValues.length !== 1)
            throw new TheorydbError(
              'ErrInvalidOperator',
              'begins_with requires one value',
            );
          values[':sk'] = marshalScalar(skSchema, skValues[0]);
          keyExpr += ' AND begins_with(#sk, :sk)';
          break;
        }
        case 'between': {
          if (skValues.length !== 2)
            throw new TheorydbError(
              'ErrInvalidOperator',
              'between requires two values',
            );
          values[':sk0'] = marshalScalar(skSchema, skValues[0]);
          values[':sk1'] = marshalScalar(skSchema, skValues[1]);
          keyExpr += ' AND #sk BETWEEN :sk0 AND :sk1';
          break;
        }
        default: {
          if (skValues.length !== 1)
            throw new TheorydbError(
              'ErrInvalidOperator',
              'sort operator requires one value',
            );
          values[':sk'] = marshalScalar(skSchema, skValues[0]);
          keyExpr += ` AND #sk ${op} :sk`;
          break;
        }
      }
    }

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
        ).map((it) => unmarshalItem(this.model, it))
      : rawItems.map((it) => unmarshalItem(this.model, it));
    let cursor: string | undefined;
    if (resp.LastEvaluatedKey) {
      const c: Cursor = { lastKey: resp.LastEvaluatedKey, sort: this.sortDir };
      if (index) c.index = index.name;
      cursor = encodeCursor(c);
    }

    const page: Page = { items };
    if (cursor) page.cursor = cursor;
    return page;
  }

  async all(): Promise<Array<Record<string, unknown>>> {
    const original = this.cursorToken;
    try {
      const out: Array<Record<string, unknown>> = [];
      let cursor = original;

      for (;;) {
        this.cursorToken = cursor;
        const page = await this.page();
        out.push(...page.items);
        if (!page.cursor) break;
        cursor = page.cursor;
      }

      return out;
    } finally {
      this.cursorToken = original;
    }
  }

  async sum(field: string): Promise<number> {
    return sumField(await this.all(), field);
  }

  async average(field: string): Promise<number> {
    return averageField(await this.all(), field);
  }

  async min(field: string): Promise<unknown> {
    return minField(await this.all(), field);
  }

  async max(field: string): Promise<unknown> {
    return maxField(await this.all(), field);
  }

  async aggregate(...fields: string[]): Promise<AggregateResult> {
    return aggregateField(await this.all(), fields[0]);
  }

  async countDistinct(field: string): Promise<number> {
    return countDistinct(await this.all(), field);
  }

  groupBy(field: string): GroupByQuery<Record<string, unknown>> {
    return new GroupByQuery(() => this.all(), field);
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

  async pageWithRetry(opts: QueryRetryOptions = {}): Promise<Page> {
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

    let lastPage: Page | undefined;
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

export class ScanBuilder {
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

  filter(field: string, op: string, ...values: unknown[]): this {
    this.filters.filter(field, op, ...values);
    return this;
  }

  orFilter(field: string, op: string, ...values: unknown[]): this {
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
  ): Promise<Array<Record<string, unknown>>> {
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
              ).map((it) => unmarshalItem(this.model, it))
            : rawItems.map((it) => unmarshalItem(this.model, it));
          items.push(...chunk);

          start = resp.LastEvaluatedKey;
          more = start !== undefined;
        }
        return items;
      },
    );

    const out: Array<Record<string, unknown>> = [];
    for (const r of results) out.push(...r);
    return out;
  }

  async all(): Promise<Array<Record<string, unknown>>> {
    const original = this.cursorToken;
    try {
      const out: Array<Record<string, unknown>> = [];
      let cursor = original;

      for (;;) {
        this.cursorToken = cursor;
        const page = await this.page();
        out.push(...page.items);
        if (!page.cursor) break;
        cursor = page.cursor;
      }

      return out;
    } finally {
      this.cursorToken = original;
    }
  }

  async sum(field: string): Promise<number> {
    return sumField(await this.all(), field);
  }

  async average(field: string): Promise<number> {
    return averageField(await this.all(), field);
  }

  async min(field: string): Promise<unknown> {
    return minField(await this.all(), field);
  }

  async max(field: string): Promise<unknown> {
    return maxField(await this.all(), field);
  }

  async aggregate(...fields: string[]): Promise<AggregateResult> {
    return aggregateField(await this.all(), fields[0]);
  }

  async countDistinct(field: string): Promise<number> {
    return countDistinct(await this.all(), field);
  }

  groupBy(field: string): GroupByQuery<Record<string, unknown>> {
    return new GroupByQuery(() => this.all(), field);
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

  async page(): Promise<Page> {
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
        ).map((it) => unmarshalItem(this.model, it))
      : rawItems.map((it) => unmarshalItem(this.model, it));
    let cursor: string | undefined;
    if (resp.LastEvaluatedKey) {
      const c: Cursor = { lastKey: resp.LastEvaluatedKey };
      if (this.indexName) c.index = this.indexName;
      cursor = encodeCursor(c);
    }

    const page: Page = { items };
    if (cursor) page.cursor = cursor;
    return page;
  }

  async pageWithRetry(opts: QueryRetryOptions = {}): Promise<Page> {
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

    let lastPage: Page | undefined;
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

async function mapConcurrent<T, R>(
  items: T[],
  concurrency: number,
  fn: (item: T) => Promise<R>,
): Promise<R[]> {
  if (!Number.isFinite(concurrency) || concurrency <= 0) {
    throw new TheorydbError(
      'ErrInvalidOperator',
      'concurrency must be a positive number',
    );
  }
  if (items.length === 0) return [];

  const limit = Math.min(items.length, Math.floor(concurrency));
  const out: R[] = new Array<R>(items.length);
  let next = 0;

  const workers = Array.from({ length: limit }, async () => {
    let done = false;
    while (!done) {
      const idx = next;
      next += 1;
      if (idx >= items.length) {
        done = true;
        continue;
      }
      out[idx] = await fn(items[idx]!);
    }
  });
  await Promise.all(workers);
  return out;
}
