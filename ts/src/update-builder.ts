import {
  UpdateItemCommand,
  type AttributeValue,
  type DynamoDBClient,
} from '@aws-sdk/client-dynamodb';

import { mapDynamoError } from './dynamo-error.js';
import { TheorydbError } from './errors.js';
import {
  decryptItemAttributes,
  encryptAttributeValue,
  modelHasEncryptedAttributes,
  type EncryptionProvider,
} from './encryption.js';
import {
  marshalDocumentValue,
  marshalKey,
  marshalScalar,
  unmarshalItem,
} from './marshal.js';
import type { AttributeSchema, Model } from './model.js';
import type { SendOptions } from './send-options.js';

export type ReturnValuesOption =
  | 'NONE'
  | 'ALL_OLD'
  | 'UPDATED_OLD'
  | 'ALL_NEW'
  | 'UPDATED_NEW';

type UpdateOp =
  | { kind: 'set'; field: string; value: unknown }
  | { kind: 'setIfNotExists'; field: string; defaultValue: unknown }
  | { kind: 'add'; field: string; value: unknown }
  | { kind: 'remove'; field: string }
  | { kind: 'delete'; field: string; value: unknown }
  | { kind: 'appendToList'; field: string; values: unknown[] }
  | { kind: 'prependToList'; field: string; values: unknown[] }
  | { kind: 'removeFromListAt'; field: string; index: number }
  | { kind: 'setListElement'; field: string; index: number; value: unknown };

type ConditionOp = {
  logicOp: 'AND' | 'OR';
  field: string;
  operator: string;
  value?: unknown;
};

class ConditionExpressionBuilder {
  private readonly conditions: string[] = [];
  private readonly operators: Array<'AND' | 'OR'> = [];
  private readonly state: {
    nameCounter: number;
    valueCounter: number;
    names: Record<string, string>;
    values: Record<string, AttributeValue>;
    namePlaceholders: Map<string, string>;
  } = {
    nameCounter: 0,
    valueCounter: 0,
    names: {},
    values: {},
    namePlaceholders: new Map<string, string>(),
  };

  constructor(private readonly model: Model) {}

  and(field: string, op: string, value?: unknown): void {
    this.addCondition('AND', field, op, value);
  }

  or(field: string, op: string, value?: unknown): void {
    this.addCondition('OR', field, op, value);
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

  private addCondition(
    logicalOp: 'AND' | 'OR',
    field: string,
    op: string,
    value?: unknown,
  ): void {
    const schema = this.model.attributes.get(field);
    if (!schema) {
      throw new TheorydbError(
        'ErrInvalidOperator',
        `Unknown condition field: ${field}`,
      );
    }
    if (schema.encryption) {
      throw new TheorydbError(
        'ErrEncryptedFieldNotQueryable',
        `Encrypted fields cannot be used in conditions: ${field}`,
      );
    }

    const nameRef = this.nameRef(field);
    const upper = op.toUpperCase();

    let values: unknown[] = [];
    if (upper === 'BETWEEN') {
      if (!Array.isArray(value) || value.length !== 2) {
        throw new TheorydbError(
          'ErrInvalidOperator',
          'BETWEEN requires a 2-element array value',
        );
      }
      values = value;
    } else if (upper === 'IN') {
      values = [value];
    } else if (
      upper === 'ATTRIBUTE_EXISTS' ||
      upper === 'ATTRIBUTE_NOT_EXISTS'
    ) {
      values = [];
    } else {
      values = value === undefined ? [] : [value];
    }

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
      case 'ATTRIBUTE_EXISTS': {
        if (values.length !== 0) {
          throw new TheorydbError(
            'ErrInvalidOperator',
            'attribute_exists does not take a value',
          );
        }
        return `attribute_exists(${nameRef})`;
      }
      case 'ATTRIBUTE_NOT_EXISTS': {
        if (values.length !== 0) {
          throw new TheorydbError(
            'ErrInvalidOperator',
            'attribute_not_exists does not take a value',
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
    const placeholder = `#c${this.state.nameCounter}`;
    this.state.names[placeholder] = field;
    this.state.namePlaceholders.set(field, placeholder);
    return placeholder;
  }

  private valueRef(schema: Readonly<AttributeSchema>, value: unknown): string {
    this.state.valueCounter += 1;
    const placeholder = `:c${this.state.valueCounter}`;
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

export class UpdateBuilder {
  private readonly updateOps: UpdateOp[] = [];
  private readonly conditionOps: ConditionOp[] = [];
  private returnValuesOpt: ReturnValuesOption = 'NONE';

  constructor(
    private readonly ddb: DynamoDBClient,
    private readonly model: Model,
    private readonly key: Record<string, unknown>,
    private readonly encryption?: EncryptionProvider,
    private readonly sendOptions?: SendOptions,
  ) {}

  set(field: string, value: unknown): this {
    this.updateOps.push({ kind: 'set', field, value });
    return this;
  }

  setIfNotExists(field: string, _value: unknown, defaultValue: unknown): this {
    this.updateOps.push({ kind: 'setIfNotExists', field, defaultValue });
    return this;
  }

  add(field: string, value: unknown): this {
    this.updateOps.push({ kind: 'add', field, value });
    return this;
  }

  increment(field: string): this {
    return this.add(field, 1);
  }

  decrement(field: string): this {
    return this.add(field, -1);
  }

  remove(field: string): this {
    this.updateOps.push({ kind: 'remove', field });
    return this;
  }

  delete(field: string, value: unknown): this {
    this.updateOps.push({ kind: 'delete', field, value });
    return this;
  }

  appendToList(field: string, values: unknown[]): this {
    this.updateOps.push({ kind: 'appendToList', field, values });
    return this;
  }

  prependToList(field: string, values: unknown[]): this {
    this.updateOps.push({ kind: 'prependToList', field, values });
    return this;
  }

  removeFromListAt(field: string, index: number): this {
    this.updateOps.push({ kind: 'removeFromListAt', field, index });
    return this;
  }

  setListElement(field: string, index: number, value: unknown): this {
    this.updateOps.push({ kind: 'setListElement', field, index, value });
    return this;
  }

  condition(field: string, operator: string, value?: unknown): this {
    this.conditionOps.push({ logicOp: 'AND', field, operator, value });
    return this;
  }

  orCondition(field: string, operator: string, value?: unknown): this {
    this.conditionOps.push({ logicOp: 'OR', field, operator, value });
    return this;
  }

  conditionExists(field: string): this {
    return this.condition(field, 'attribute_exists');
  }

  conditionNotExists(field: string): this {
    return this.condition(field, 'attribute_not_exists');
  }

  conditionVersion(currentVersion: number): this {
    const versionAttr = this.model.roles.version;
    if (!versionAttr) {
      throw new TheorydbError(
        'ErrInvalidModel',
        `Model ${this.model.name} does not define a version field`,
      );
    }
    return this.condition(versionAttr, '=', currentVersion);
  }

  returnValues(option: ReturnValuesOption): this {
    this.returnValuesOpt = option;
    return this;
  }

  async execute(): Promise<Record<string, unknown> | undefined> {
    if (this.updateOps.length === 0) {
      throw new TheorydbError('ErrInvalidOperator', 'No updates provided');
    }
    if (modelHasEncryptedAttributes(this.model) && !this.encryption) {
      throw new TheorydbError(
        'ErrEncryptionNotConfigured',
        `Encryption is required for model: ${this.model.name}`,
      );
    }

    const { updateExpression, names, values } =
      await this.buildUpdateExpression();

    const cond = this.buildConditionExpression();
    for (const [k, v] of Object.entries(cond.names)) {
      if (k in names) {
        throw new TheorydbError(
          'ErrInvalidOperator',
          `ExpressionAttributeNames collision: ${k}`,
        );
      }
      names[k] = v;
    }
    for (const [k, v] of Object.entries(cond.values)) {
      if (k in values) {
        throw new TheorydbError(
          'ErrInvalidOperator',
          `ExpressionAttributeValues collision: ${k}`,
        );
      }
      values[k] = v;
    }

    const cmd = new UpdateItemCommand({
      TableName: this.model.tableName,
      Key: marshalKey(this.model, this.key),
      UpdateExpression: updateExpression,
      ...(cond.expression ? { ConditionExpression: cond.expression } : {}),
      ExpressionAttributeNames: Object.keys(names).length ? names : undefined,
      ExpressionAttributeValues: Object.keys(values).length
        ? values
        : undefined,
      ReturnValues: this.returnValuesOpt,
    });

    try {
      const resp = await this.ddb.send(cmd, this.sendOptions);
      if (!resp.Attributes) return undefined;
      const provider = modelHasEncryptedAttributes(this.model)
        ? this.encryption!
        : undefined;
      const attrs = provider
        ? await decryptItemAttributes(this.model, resp.Attributes, provider)
        : resp.Attributes;
      return unmarshalItem(this.model, attrs);
    } catch (err) {
      throw mapDynamoError(err);
    }
  }

  private buildConditionExpression(): {
    expression?: string;
    names: Record<string, string>;
    values: Record<string, AttributeValue>;
  } {
    const builder = new ConditionExpressionBuilder(this.model);
    for (const c of this.conditionOps) {
      if (c.logicOp === 'AND') builder.and(c.field, c.operator, c.value);
      else builder.or(c.field, c.operator, c.value);
    }
    return builder.build();
  }

  private async buildUpdateExpression(): Promise<{
    updateExpression: string;
    names: Record<string, string>;
    values: Record<string, AttributeValue>;
  }> {
    const names: Record<string, string> = {};
    const values: Record<string, AttributeValue> = {};
    const namePlaceholders = new Map<string, string>();
    let nameCounter = 0;
    let valueCounter = 0;

    const setParts: string[] = [];
    const removeParts: string[] = [];
    const addParts: string[] = [];
    const deleteParts: string[] = [];

    const nameRef = (
      field: string,
    ): { ref: string; schema: AttributeSchema } => {
      this.assertUpdatableField(field);
      const schema = this.model.attributes.get(field);
      if (!schema) {
        throw new TheorydbError(
          'ErrInvalidOperator',
          `Unknown update field: ${field}`,
        );
      }

      const existing = namePlaceholders.get(field);
      if (existing) return { ref: existing, schema };

      nameCounter += 1;
      const placeholder = `#u${nameCounter}`;
      names[placeholder] = field;
      namePlaceholders.set(field, placeholder);
      return { ref: placeholder, schema };
    };

    const valueRef = async (
      field: string,
      schema: Readonly<AttributeSchema>,
      value: unknown,
    ): Promise<string> => {
      valueCounter += 1;
      const placeholder = `:u${valueCounter}`;
      values[placeholder] =
        schema.encryption !== undefined
          ? await encryptAttributeValue(schema, value, this.encryption!, {
              model: this.model.name,
              attribute: field,
            })
          : marshalScalar(schema, value);
      return placeholder;
    };

    const documentValueRef = (value: unknown): string => {
      valueCounter += 1;
      const placeholder = `:u${valueCounter}`;
      values[placeholder] = marshalDocumentValue(value);
      return placeholder;
    };

    for (const op of this.updateOps) {
      switch (op.kind) {
        case 'set': {
          const { ref, schema } = nameRef(op.field);
          const v = await valueRef(op.field, schema, op.value);
          setParts.push(`${ref} = ${v}`);
          break;
        }
        case 'setIfNotExists': {
          const { ref, schema } = nameRef(op.field);
          const d = await valueRef(op.field, schema, op.defaultValue);
          setParts.push(`${ref} = if_not_exists(${ref}, ${d})`);
          break;
        }
        case 'add': {
          const { ref, schema } = nameRef(op.field);
          if (schema.encryption) {
            throw new TheorydbError(
              'ErrInvalidOperator',
              `Encrypted fields cannot be used in ADD: ${op.field}`,
            );
          }
          if (schema.type === 'N') {
            const v = await valueRef(op.field, schema, op.value);
            addParts.push(`${ref} ${v}`);
            break;
          }
          if (
            schema.type === 'SS' ||
            schema.type === 'NS' ||
            schema.type === 'BS'
          ) {
            const v = await valueRef(
              op.field,
              schema,
              normalizeSetValue(schema, op.value),
            );
            addParts.push(`${ref} ${v}`);
            break;
          }
          throw new TheorydbError(
            'ErrInvalidOperator',
            `ADD is only supported for N/SS/NS/BS: ${op.field}`,
          );
        }
        case 'remove': {
          const { ref } = nameRef(op.field);
          removeParts.push(ref);
          break;
        }
        case 'delete': {
          const { ref, schema } = nameRef(op.field);
          if (schema.encryption) {
            throw new TheorydbError(
              'ErrInvalidOperator',
              `Encrypted fields cannot be used in DELETE: ${op.field}`,
            );
          }
          if (
            schema.type !== 'SS' &&
            schema.type !== 'NS' &&
            schema.type !== 'BS'
          ) {
            throw new TheorydbError(
              'ErrInvalidOperator',
              `DELETE is only supported for SS/NS/BS: ${op.field}`,
            );
          }
          const v = await valueRef(
            op.field,
            schema,
            normalizeSetValue(schema, op.value),
          );
          deleteParts.push(`${ref} ${v}`);
          break;
        }
        case 'appendToList': {
          const { ref, schema } = nameRef(op.field);
          if (schema.encryption) {
            throw new TheorydbError(
              'ErrInvalidOperator',
              `Encrypted fields cannot be used in list operations: ${op.field}`,
            );
          }
          if (schema.type !== 'L') {
            throw new TheorydbError(
              'ErrInvalidOperator',
              `appendToList requires L type: ${op.field}`,
            );
          }
          const v = await valueRef(op.field, schema, op.values);
          setParts.push(`${ref} = list_append(${ref}, ${v})`);
          break;
        }
        case 'prependToList': {
          const { ref, schema } = nameRef(op.field);
          if (schema.encryption) {
            throw new TheorydbError(
              'ErrInvalidOperator',
              `Encrypted fields cannot be used in list operations: ${op.field}`,
            );
          }
          if (schema.type !== 'L') {
            throw new TheorydbError(
              'ErrInvalidOperator',
              `prependToList requires L type: ${op.field}`,
            );
          }
          const v = await valueRef(op.field, schema, op.values);
          setParts.push(`${ref} = list_append(${v}, ${ref})`);
          break;
        }
        case 'removeFromListAt': {
          const { ref, schema } = nameRef(op.field);
          if (schema.encryption) {
            throw new TheorydbError(
              'ErrInvalidOperator',
              `Encrypted fields cannot be used in list operations: ${op.field}`,
            );
          }
          if (schema.type !== 'L') {
            throw new TheorydbError(
              'ErrInvalidOperator',
              `removeFromListAt requires L type: ${op.field}`,
            );
          }
          if (!Number.isInteger(op.index) || op.index < 0) {
            throw new TheorydbError(
              'ErrInvalidOperator',
              'list index must be a non-negative integer',
            );
          }
          removeParts.push(`${ref}[${op.index}]`);
          break;
        }
        case 'setListElement': {
          const { ref, schema } = nameRef(op.field);
          if (schema.encryption) {
            throw new TheorydbError(
              'ErrInvalidOperator',
              `Encrypted fields cannot be used in list operations: ${op.field}`,
            );
          }
          if (schema.type !== 'L') {
            throw new TheorydbError(
              'ErrInvalidOperator',
              `setListElement requires L type: ${op.field}`,
            );
          }
          if (!Number.isInteger(op.index) || op.index < 0) {
            throw new TheorydbError(
              'ErrInvalidOperator',
              'list index must be a non-negative integer',
            );
          }
          const v = documentValueRef(op.value);
          setParts.push(`${ref}[${op.index}] = ${v}`);
          break;
        }
        default: {
          const _exhaustive: never = op;
          throw new TheorydbError(
            'ErrInvalidOperator',
            `Unsupported update op: ${String(_exhaustive)}`,
          );
        }
      }
    }

    const updateParts: string[] = [];
    if (setParts.length) updateParts.push(`SET ${setParts.join(', ')}`);
    if (removeParts.length)
      updateParts.push(`REMOVE ${removeParts.join(', ')}`);
    if (addParts.length) updateParts.push(`ADD ${addParts.join(', ')}`);
    if (deleteParts.length)
      updateParts.push(`DELETE ${deleteParts.join(', ')}`);

    const updateExpression = updateParts.join(' ');
    if (!updateExpression) {
      throw new TheorydbError('ErrInvalidOperator', 'No updates provided');
    }

    return { updateExpression, names, values };
  }

  private assertUpdatableField(field: string): void {
    if (field === this.model.roles.pk || field === this.model.roles.sk) {
      throw new TheorydbError(
        'ErrInvalidOperator',
        `Cannot update key field: ${field}`,
      );
    }
  }
}

function normalizeSetValue(
  schema: Readonly<AttributeSchema>,
  value: unknown,
): unknown {
  if (schema.type === 'SS') {
    if (typeof value === 'string') return [value];
    if (Array.isArray(value)) return value;
    throw new TheorydbError(
      'ErrInvalidOperator',
      'SS set requires string or string[]',
    );
  }
  if (schema.type === 'NS') {
    if (
      typeof value === 'number' ||
      typeof value === 'bigint' ||
      typeof value === 'string'
    )
      return [value];
    if (Array.isArray(value)) return value;
    throw new TheorydbError(
      'ErrInvalidOperator',
      'NS set requires number or number[]',
    );
  }
  if (schema.type === 'BS') {
    if (value instanceof Uint8Array) return [value];
    if (Array.isArray(value)) return value;
    throw new TheorydbError(
      'ErrInvalidOperator',
      'BS set requires Uint8Array or Uint8Array[]',
    );
  }
  return value;
}
