from __future__ import annotations

from dataclasses import dataclass


@dataclass(frozen=True)
class CancellationReason:
    """One DynamoDB transaction cancellation reason, in its original position.

    Carries only the classification code and the service message: the DynamoDB
    item image is never copied here, so reasons are always safe to log. An entry
    whose code is missing keeps its position with an empty ``code``; DynamoDB's
    ``None`` placeholder is kept verbatim.
    """

    index: int
    code: str
    message: str


class TheorydbPyError(Exception):
    pass


class ConditionFailedError(TheorydbPyError):
    def __init__(
        self,
        message: str,
        *,
        reasons: tuple[CancellationReason, ...] = (),
    ) -> None:
        super().__init__(message)
        self.reasons = reasons


class VersionConflictError(ConditionFailedError):
    pass


class LeaseHeldError(ConditionFailedError):
    pass


class LeaseNotOwnedError(ConditionFailedError):
    pass


class NotFoundError(TheorydbPyError):
    pass


class ValidationError(TheorydbPyError):
    pass


class ImmutableModelMutationError(TheorydbPyError):
    pass


class ProtectedFieldMutationError(TheorydbPyError):
    pass


class RejectedDeployAuthorityEvidenceError(TheorydbPyError):
    pass


class BatchRetryExceededError(TheorydbPyError):
    def __init__(self, *, operation: str, unprocessed_count: int) -> None:
        super().__init__(f"{operation}: retry limit exceeded (unprocessed={unprocessed_count})")
        self.operation = operation
        self.unprocessed_count = unprocessed_count


class TransactionCanceledError(TheorydbPyError):
    def __init__(
        self,
        *,
        message: str,
        reason_codes: tuple[str, ...],
        reasons: tuple[CancellationReason, ...] = (),
    ) -> None:
        super().__init__(message)
        # reason_codes keeps every position (an entry with no code becomes "").
        self.reason_codes = reason_codes
        self.reasons = reasons


class TransactionConflictError(TransactionCanceledError):
    """A transaction was canceled by a DynamoDB transaction conflict.

    It remains a TransactionCanceledError so existing handlers keep matching.
    """


class ThrottledError(TransactionCanceledError):
    """A transaction was canceled because DynamoDB throttled it.

    It remains a TransactionCanceledError so existing handlers keep matching.
    """


class EncryptionNotConfiguredError(TheorydbPyError):
    pass


class MigrationEncryptionError(TheorydbPyError):
    pass


class AwsError(TheorydbPyError):
    def __init__(self, *, code: str, message: str) -> None:
        super().__init__(f"{code}: {message}")
        self.code = code
        self.message = message
