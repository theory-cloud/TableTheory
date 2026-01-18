import {
  ConditionalCheckFailedException,
  TransactionCanceledException,
} from '@aws-sdk/client-dynamodb';

import { TheorydbError } from './errors.js';

export function mapDynamoError(err: unknown): unknown {
  if (err instanceof TheorydbError) return err;

  if (err instanceof ConditionalCheckFailedException) {
    return new TheorydbError('ErrConditionFailed', 'Condition failed', {
      cause: err,
    });
  }

  if (err instanceof TransactionCanceledException) {
    return new TheorydbError('ErrConditionFailed', 'Transaction canceled', {
      cause: err,
    });
  }

  if (typeof err === 'object' && err !== null && 'name' in err) {
    const name = (err as { name?: unknown }).name;
    if (name === 'ConditionalCheckFailedException') {
      return new TheorydbError('ErrConditionFailed', 'Condition failed', {
        cause: err,
      });
    }
    if (name === 'TransactionCanceledException') {
      return new TheorydbError('ErrConditionFailed', 'Transaction canceled', {
        cause: err,
      });
    }
  }

  return err;
}
