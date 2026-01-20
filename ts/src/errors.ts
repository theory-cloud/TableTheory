export type ErrorCode =
  | 'ErrItemNotFound'
  | 'ErrConditionFailed'
  | 'ErrLeaseHeld'
  | 'ErrLeaseNotOwned'
  | 'ErrInvalidModel'
  | 'ErrMissingPrimaryKey'
  | 'ErrInvalidOperator'
  | 'ErrTableNotFound'
  | 'ErrEncryptedFieldNotQueryable'
  | 'ErrEncryptionNotConfigured'
  | 'ErrInvalidEncryptedEnvelope';

export class TheorydbError extends Error {
  readonly code: ErrorCode;

  constructor(code: ErrorCode, message: string, options?: { cause?: unknown }) {
    super(message);
    this.code = code;
    this.name = code;
    if (options?.cause !== undefined) {
      // Avoid depending on a specific TS libdom ErrorOptions typing.
      (this as { cause?: unknown }).cause = options.cause;
    }
  }
}

export function isTheorydbError(value: unknown): value is TheorydbError {
  return value instanceof TheorydbError;
}
