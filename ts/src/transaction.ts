import type { AttributeValue } from '@aws-sdk/client-dynamodb';

export type TransactAction =
  | {
      kind: 'put';
      model: string;
      item: Record<string, unknown>;
      ifNotExists?: boolean;
    }
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
