import {
  ConditionalCheckFailedException,
  TransactionCanceledException,
} from '@aws-sdk/client-dynamodb';

import { TheorydbError } from './errors.js';

export interface DynamoErrorMappingOptions {
  readonly versionConflict?: boolean;
}

export function mapDynamoError(
  err: unknown,
  options: DynamoErrorMappingOptions = {},
): unknown {
  if (err instanceof TheorydbError) return err;

  if (err instanceof ConditionalCheckFailedException) {
    return conditionFailedError('Condition failed', err, options);
  }

  if (err instanceof TransactionCanceledException) {
    return new TheorydbError('ErrConditionFailed', 'Transaction canceled', {
      cause: err,
    });
  }

  if (typeof err === 'object' && err !== null && 'name' in err) {
    const name = (err as { name?: unknown }).name;
    if (name === 'ConditionalCheckFailedException') {
      return conditionFailedError('Condition failed', err, options);
    }
    if (name === 'TransactionCanceledException') {
      return new TheorydbError('ErrConditionFailed', 'Transaction canceled', {
        cause: err,
      });
    }
  }

  return err;
}

function conditionFailedError(
  message: string,
  cause: unknown,
  options: DynamoErrorMappingOptions,
): TheorydbError {
  return new TheorydbError('ErrConditionFailed', message, {
    cause,
    ...(options.versionConflict ? { codes: ['ErrVersionConflict'] } : {}),
  });
}
