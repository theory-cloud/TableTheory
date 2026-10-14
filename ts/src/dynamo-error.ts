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
  /**
   * When true, item images are stripped from the cause retained on the mapped
   * error: `CancellationReasons[].Item` and the `Item` a
   * `ConditionalCheckFailedException` carries. Off by default, so the retained
   * cause is exactly what it was before. The original SDK error object is never
   * mutated — a clone keeps its prototype, so `instanceof` still holds.
   */
  readonly redactItemImages?: boolean;
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
    return transactionCanceledError(err, options);
  }

  if (typeof err === 'object' && err !== null && 'name' in err) {
    const name = (err as { name?: unknown }).name;
    if (name === 'ConditionalCheckFailedException') {
      return conditionFailedError('Condition failed', err, options);
    }
    if (name === 'TransactionCanceledException') {
      return transactionCanceledError(err, options);
    }
  }

  return err;
}

function retainedCause(
  cause: unknown,
  options: DynamoErrorMappingOptions,
): unknown {
  return options.redactItemImages ? stripItemImages(cause) : cause;
}

function conditionFailedError(
  message: string,
  cause: unknown,
  options: DynamoErrorMappingOptions,
): TheorydbError {
  return new TheorydbError('ErrConditionFailed', message, {
    cause: retainedCause(cause, options),
    ...(options.versionConflict ? { codes: ['ErrVersionConflict'] } : {}),
  });
}

function transactionCanceledError(
  err: unknown,
  options: DynamoErrorMappingOptions,
): TheorydbError {
  const reasons = extractCancellationReasons(err);
  const firstReal = reasons.find(
    (reason) => reason.code !== '' && reason.code !== 'None',
  );
  const code: ErrorCode = firstReal
    ? (CANCELLATION_CLASSIFICATION[firstReal.code] ?? 'ErrTransactionFailed')
    : 'ErrTransactionFailed';

  return new TheorydbError(code, 'Transaction canceled', {
    cause: retainedCause(err, options),
    cancellationReasons: reasons,
  });
}

/**
 * Returns a copy of an SDK exception with item images removed:
 * `CancellationReasons[].Item` and a top-level `Item`. The prototype and every
 * own property descriptor are preserved, so `instanceof` and `message`/`stack`
 * still behave. The original error object is left untouched, and `err` itself
 * is returned when there is nothing to strip.
 */
function stripItemImages(err: unknown): unknown {
  if (typeof err !== 'object' || err === null) return err;

  const source = err as Record<string, unknown>;
  const reasons = source.CancellationReasons;
  const hasReasons = Array.isArray(reasons);
  const hasItem = Object.prototype.hasOwnProperty.call(source, 'Item');
  if (!hasReasons && !hasItem) return err;

  const clone = Object.create(
    Object.getPrototypeOf(err) as object | null,
  ) as Record<string, unknown>;
  Object.defineProperties(clone, Object.getOwnPropertyDescriptors(source));

  if (hasReasons) {
    const redacted = (reasons as unknown[]).map((entry) => {
      if (typeof entry !== 'object' || entry === null) return entry;
      const copy: Record<string, unknown> = {
        ...(entry as Record<string, unknown>),
      };
      delete copy.Item;
      return copy;
    });
    Object.defineProperty(clone, 'CancellationReasons', {
      value: redacted,
      writable: true,
      enumerable: true,
      configurable: true,
    });
  }

  if (hasItem) {
    delete clone.Item;
  }

  return clone;
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
