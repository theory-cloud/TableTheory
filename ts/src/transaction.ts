import type { AttributeValue } from '@aws-sdk/client-dynamodb';

import type { UpdateBuilder } from './update-builder.js';

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
    }
  | {
      kind: 'condition';
      model: string;
      key: Record<string, unknown>;
      conditionExpression: string;
      expressionAttributeNames?: Record<string, string>;
      expressionAttributeValues?: Record<string, AttributeValue>;
    };
