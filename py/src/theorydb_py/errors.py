from __future__ import annotations


class TheorydbPyError(Exception):
    pass


class ConditionFailedError(TheorydbPyError):
    pass


class NotFoundError(TheorydbPyError):
    pass


class ValidationError(TheorydbPyError):
    pass


class BatchRetryExceededError(TheorydbPyError):
    def __init__(self, *, operation: str, unprocessed_count: int) -> None:
        super().__init__(f"{operation}: retry limit exceeded (unprocessed={unprocessed_count})")
        self.operation = operation
        self.unprocessed_count = unprocessed_count


class TransactionCanceledError(TheorydbPyError):
    def __init__(self, *, message: str, reason_codes: tuple[str, ...]) -> None:
        super().__init__(message)
        self.reason_codes = reason_codes


class EncryptionNotConfiguredError(TheorydbPyError):
    pass


class AwsError(TheorydbPyError):
    def __init__(self, *, code: str, message: str) -> None:
        super().__init__(f"{code}: {message}")
        self.code = code
        self.message = message
