import {
  ConditionalCheckFailedException,
  TransactionCanceledException,
} from '@aws-sdk/client-dynamodb';

import {
  TheorydbError,
  type CancellationReason,
  type ErrorCode,
} from './errors.js';

export interface DynamoErrorMappingOptions {
  readonly versionConflict?: boolean;
}

// The cancellation taxonomy shared with the Go and Python runtimes. The FIRST
// genuine failure in the reason list decides the primary code; an unclassified
// code is a generic transaction failure and never a conditional conflict.
const CANCELLATION_CLASSIFICATION: Readonly<Record<string, ErrorCode>> = {
  ConditionalCheckFailed: 'ErrConditionFailed',
  ConditionalCheckFailedException: 'ErrConditionFailed',
  TransactionConflict: 'ErrTransactionConflict',
  TransactionConflictException: 'ErrTransactionConflict',
  ProvisionedThroughputExceeded: 'ErrThrottled',
  ProvisionedThroughputExceededException: 'ErrThrottled',
  ThrottlingException: 'ErrThrottled',
  ThrottlingError: 'ErrThrottled',
  RequestLimitExceeded: 'ErrThrottled',
  InternalServerError: 'ErrThrottled',
  ServiceUnavailable: 'ErrThrottled',
};

export function mapDynamoError(
  err: unknown,
  options: DynamoErrorMappingOptions = {},
): unknown {
  if (err instanceof TheorydbError) return err;

  if (err instanceof ConditionalCheckFailedException) {
    return conditionFailedError('Condition failed', err, options);
  }

  if (err instanceof TransactionCanceledException) {
    return transactionCanceledError(err);
  }

  if (typeof err === 'object' && err !== null && 'name' in err) {
    const name = (err as { name?: unknown }).name;
    if (name === 'ConditionalCheckFailedException') {
      return conditionFailedError('Condition failed', err, options);
    }
    if (name === 'TransactionCanceledException') {
      return transactionCanceledError(err);
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

function transactionCanceledError(err: unknown): TheorydbError {
  const reasons = extractCancellationReasons(err);
  const firstReal = reasons.find(
    (reason) => reason.code !== '' && reason.code !== 'None',
  );
  const code: ErrorCode = firstReal
    ? (CANCELLATION_CLASSIFICATION[firstReal.code] ?? 'ErrTransactionFailed')
    : 'ErrTransactionFailed';

  return new TheorydbError(code, 'Transaction canceled', {
    cause: err,
    cancellationReasons: reasons,
  });
}

/**
 * Reads the ordered `CancellationReasons` off a transaction cancellation,
 * preserving position and treating a missing code as an empty string. Item
 * images are dropped: only the code and message survive.
 */
function extractCancellationReasons(err: unknown): CancellationReason[] {
  const raw = (err as { CancellationReasons?: unknown }).CancellationReasons;
  if (!Array.isArray(raw)) return [];
  return raw.map((entry, index) => {
    const reason = (entry ?? {}) as { Code?: unknown; Message?: unknown };
    return {
      index,
      code: typeof reason.Code === 'string' ? reason.Code : '',
      message: typeof reason.Message === 'string' ? reason.Message : '',
    };
  });
}
