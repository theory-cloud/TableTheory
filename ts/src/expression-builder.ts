import type { AttributeValue } from '@aws-sdk/client-dynamodb';

import { TheorydbError } from './errors.js';
import type { AttributeSchema } from './model.js';

type ValueRef = (schema: Readonly<AttributeSchema>, value: unknown) => string;

export interface ConditionExpressionOptions {
  existsOperators: readonly string[];
  notExistsOperators: readonly string[];
  existsValueError: string;
  notExistsValueError: string;
}

export interface BuiltExpressionState {
  expression?: string;
  names: Record<string, string>;
  values: Record<string, AttributeValue>;
}

export function buildConditionExpression(
  nameRef: string,
  schema: Readonly<AttributeSchema>,
  op: string,
  values: unknown[],
  valueRef: ValueRef,
  options: ConditionExpressionOptions,
): string {
  switch (op) {
    case '=':
    case 'EQ': {
      const ref = valueRef(schema, singleConditionValue(values, op));
      return `${nameRef} = ${ref}`;
    }
    case '!=':
    case '<>':
    case 'NE': {
      const ref = valueRef(schema, singleConditionValue(values, op));
      return `${nameRef} <> ${ref}`;
    }
    case '<':
    case 'LT': {
      const ref = valueRef(schema, singleConditionValue(values, op));
      return `${nameRef} < ${ref}`;
    }
    case '<=':
    case 'LE': {
      const ref = valueRef(schema, singleConditionValue(values, op));
      return `${nameRef} <= ${ref}`;
    }
    case '>':
    case 'GT': {
      const ref = valueRef(schema, singleConditionValue(values, op));
      return `${nameRef} > ${ref}`;
    }
    case '>=':
    case 'GE': {
      const ref = valueRef(schema, singleConditionValue(values, op));
      return `${nameRef} >= ${ref}`;
    }
    case 'BETWEEN': {
      if (values.length !== 2) {
        throw new TheorydbError(
          'ErrInvalidOperator',
          'BETWEEN requires two values',
        );
      }
      const left = valueRef(schema, values[0]);
      const right = valueRef(schema, values[1]);
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
      const refs = list.map((v) => valueRef(schema, v));
      return `${nameRef} IN (${refs.join(', ')})`;
    }
    case 'BEGINS_WITH': {
      const ref = valueRef(schema, singleConditionValue(values, op));
      return `begins_with(${nameRef}, ${ref})`;
    }
    case 'CONTAINS': {
      const ref = valueRef(schema, singleConditionValue(values, op));
      return `contains(${nameRef}, ${ref})`;
    }
    default: {
      if (options.existsOperators.includes(op)) {
        if (values.length !== 0) {
          throw new TheorydbError(
            'ErrInvalidOperator',
            options.existsValueError,
          );
        }
        return `attribute_exists(${nameRef})`;
      }
      if (options.notExistsOperators.includes(op)) {
        if (values.length !== 0) {
          throw new TheorydbError(
            'ErrInvalidOperator',
            options.notExistsValueError,
          );
        }
        return `attribute_not_exists(${nameRef})`;
      }
      throw new TheorydbError(
        'ErrInvalidOperator',
        `Unsupported operator: ${op}`,
      );
    }
  }
}

function singleConditionValue(values: unknown[], op: string): unknown {
  if (values.length !== 1) {
    throw new TheorydbError('ErrInvalidOperator', `${op} requires one value`);
  }
  return values[0];
}
