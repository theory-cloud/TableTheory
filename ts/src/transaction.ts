import type { AttributeValue } from '@aws-sdk/client-dynamodb';

import type { UpdateBuilder } from './update-builder.js';

/**
 * Raw transaction updates bypass UpdateBuilder encryption/validation and are
 * therefore only valid for models without encrypted attributes. Encrypted
 * models must use `updateFn` or `TransactModelUpdate`.
 */
type TransactUpdateRaw = {
  kind: 'update';
  model: string;
  key: Record<string, unknown>;
  updateExpression: string;
  conditionExpression?: string;
  expressionAttributeNames?: Record<string, string>;
  expressionAttributeValues?: Record<string, AttributeValue>;
  updateFn?: never;
  item?: never;
  fields?: never;
};

type TransactUpdateWithBuilder = {
  kind: 'update';
  model: string;
  key: Record<string, unknown>;
  updateFn: (builder: UpdateBuilder) => void | Promise<void>;
  updateExpression?: never;
  conditionExpression?: never;
  expressionAttributeNames?: never;
  expressionAttributeValues?: never;
  item?: never;
  fields?: never;
};

/**
 * A model-based transactional update with an explicit field selection.
 *
 * Selected empty `omit_empty` attributes are removed according to DMS v0.2;
 * every other selected attribute is set from `item`. Fields not listed in
 * `fields` are not mutated, except that the runtime advances `updatedAt` when
 * the model declares that lifecycle role. `createdAt` and version cannot be
 * selected because those attributes are library-owned.
 */
export type TransactModelUpdate = {
  kind: 'update';
  model: string;
  item: Record<string, unknown>;
  fields: readonly string[];
  conditionExpression?: string;
  expressionAttributeNames?: Record<string, string>;
  expressionAttributeValues?: Record<string, AttributeValue>;
  key?: never;
  updateExpression?: never;
  updateFn?: never;
};

export type TransactAction =
  | {
      kind: 'put';
      model: string;
      item: Record<string, unknown>;
      ifNotExists?: boolean;
    }
  | TransactUpdateRaw
  | TransactUpdateWithBuilder
  | TransactModelUpdate
  | {
      kind: 'delete';
      model: string;
      key: Record<string, unknown>;
      conditionExpression?: string;
      expressionAttributeNames?: Record<string, string>;
      expressionAttributeValues?: Record<string, AttributeValue>;
    }
  | {
      kind: 'condition';
      model: string;
      key: Record<string, unknown>;
      conditionExpression: string;
      expressionAttributeNames?: Record<string, string>;
      expressionAttributeValues?: Record<string, AttributeValue>;
    };

export type TransactGetAction = {
  model: string;
  key: Record<string, unknown>;
  projection?: string[];
};
