import type { AttributeValue } from '@aws-sdk/client-dynamodb';

import type { UpdateBuilder } from './update-builder.js';

/**
 * Raw transaction updates bypass UpdateBuilder encryption/validation and are
 * therefore only valid for models without encrypted attributes. Encrypted
 * models must use `updateFn`.
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
