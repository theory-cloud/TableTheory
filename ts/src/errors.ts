export type ErrorCode =
  | 'ErrItemNotFound'
  | 'ErrConditionFailed'
  | 'ErrVersionConflict'
  | 'ErrLeaseHeld'
  | 'ErrLeaseNotOwned'
  | 'ErrInvalidModel'
  | 'ErrMissingPrimaryKey'
  | 'ErrInvalidOperator'
  | 'ErrTableNotFound'
  | 'ErrEncryptedFieldNotQueryable'
  | 'ErrEncryptionNotConfigured'
  | 'ErrInvalidEncryptedEnvelope'
  | 'ErrImmutableModelMutation'
  | 'ErrProtectedFieldMutation'
  | 'ErrRejectedDeployAuthorityEvidence';

export class TheorydbError extends Error {
  readonly code: ErrorCode;
  readonly codes: readonly ErrorCode[];

  constructor(
    code: ErrorCode,
    message: string,
    options?: { cause?: unknown; codes?: readonly ErrorCode[] },
  ) {
    super(message);
    this.code = code;
    this.codes = uniqueCodes([code, ...(options?.codes ?? [])]);
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

export function theorydbErrorCodes(value: unknown): readonly ErrorCode[] {
  if (!isTheorydbError(value)) return [];
  return value.codes.length > 0 ? value.codes : [value.code];
}

export function hasTheorydbErrorCode(value: unknown, code: ErrorCode): boolean {
  return theorydbErrorCodes(value).includes(code);
}

function uniqueCodes(codes: readonly ErrorCode[]): readonly ErrorCode[] {
  const seen = new Set<ErrorCode>();
  const out: ErrorCode[] = [];
  for (const code of codes) {
    if (seen.has(code)) continue;
    seen.add(code);
    out.push(code);
  }
  return out;
}
