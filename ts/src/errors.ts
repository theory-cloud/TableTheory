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
  | 'ErrNumberPrecisionLoss'
  | 'ErrMigrationEncryptionRequired'
  | 'ErrImmutableModelMutation'
  | 'ErrProtectedFieldMutation'
  | 'ErrRejectedDeployAuthorityEvidence'
  | 'ErrTransactionFailed'
  | 'ErrTransactionConflict'
  | 'ErrThrottled';

/**
 * One entry of a DynamoDB transaction cancellation reason list, in its original
 * position. It carries only the classification code and the service message: the
 * DynamoDB item image is never copied here, so cancellation reasons are always
 * safe to log. Entries whose code is missing keep their position with an empty
 * `code`; DynamoDB's "None" placeholder is kept verbatim.
 */
export interface CancellationReason {
  readonly index: number;
  readonly code: string;
  readonly message: string;
}

export class TheorydbError extends Error {
  readonly code: ErrorCode;
  readonly codes: readonly ErrorCode[];
  readonly cancellationReasons: readonly CancellationReason[];

  constructor(
    code: ErrorCode,
    message: string,
    options?: {
      cause?: unknown;
      codes?: readonly ErrorCode[];
      cancellationReasons?: readonly CancellationReason[];
    },
  ) {
    super(message);
    this.code = code;
    this.codes = uniqueCodes([code, ...(options?.codes ?? [])]);
    this.cancellationReasons = options?.cancellationReasons ?? [];
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

/**
 * The typed, item-free surface of a `TheorydbError`.
 */
export interface SafeErrorFields {
  readonly code: ErrorCode;
  readonly codes: readonly ErrorCode[];
  readonly message: string;
  readonly cancellationReasons: readonly CancellationReason[];
}

/**
 * Projects a `TheorydbError` onto its typed surface only.
 *
 * The result never contains `cause`, so it cannot carry the underlying SDK
 * exception — and therefore never carries `CancellationReasons[].Item`, an
 * `Item` payload, a key, or a request/response body. Returns `undefined` for a
 * value that is not a `TheorydbError`: there is no typed surface to project.
 *
 * A whole-error dump (`JSON.stringify(err)`, `console.log(err)`, `util.inspect`)
 * is not safe: `TheorydbError.cause` is an own enumerable property, so it walks
 * the raw SDK exception. Use this helper to log the typed surface, and the
 * client's `redactItemImages` option to strip item images from the retained
 * cause itself.
 */
export function safeErrorFields(err: unknown): SafeErrorFields | undefined {
  if (!isTheorydbError(err)) return undefined;
  return {
    code: err.code,
    codes: err.codes,
    message: err.message,
    cancellationReasons: err.cancellationReasons.map((reason) => ({
      index: reason.index,
      code: reason.code,
      message: reason.message,
    })),
  };
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
