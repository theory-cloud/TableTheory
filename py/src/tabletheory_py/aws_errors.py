from __future__ import annotations

from typing import Any

from botocore.exceptions import ClientError

from .errors import (
    AwsError,
    CancellationReason,
    ConditionFailedError,
    NotFoundError,
    ThrottledError,
    TransactionCanceledError,
    TransactionConflictError,
    ValidationError,
    VersionConflictError,
)

# The cancellation taxonomy shared with the Go and TypeScript runtimes. The FIRST
# genuine failure in the reason list decides the classification; an unclassified
# code is a generic transaction cancellation and never a conditional conflict.
_CANCELLATION_CLASSIFICATION: dict[str, str] = {
    "ConditionalCheckFailed": "condition_failed",
    "ConditionalCheckFailedException": "condition_failed",
    "TransactionConflict": "transaction_conflict",
    "TransactionConflictException": "transaction_conflict",
    "ProvisionedThroughputExceeded": "throttled",
    "ProvisionedThroughputExceededException": "throttled",
    "ThrottlingException": "throttled",
    "ThrottlingError": "throttled",
    "RequestLimitExceeded": "throttled",
    "InternalServerError": "throttled",
    "ServiceUnavailable": "throttled",
}


def map_client_error(err: ClientError, *, version_conflict: bool = False) -> Exception:
    code = str(err.response.get("Error", {}).get("Code", ""))
    message = str(err.response.get("Error", {}).get("Message", ""))

    if code == "ConditionalCheckFailedException":
        if version_conflict:
            return VersionConflictError(message)
        return ConditionFailedError(message)
    if code == "ValidationException":
        return ValidationError(message)
    if code == "ResourceNotFoundException":
        return NotFoundError(message)

    return AwsError(code=code or "UnknownError", message=message or str(err))


def _cancellation_reasons(raw: Any) -> tuple[CancellationReason, ...]:
    """Read DynamoDB cancellation reasons, preserving position.

    Only the code and message survive: a reason's item image is never copied.
    A missing code becomes "" so positions stay aligned with the operations.
    """

    if not isinstance(raw, list):
        return ()
    reasons: list[CancellationReason] = []
    for index, entry in enumerate(raw):
        reason = entry if isinstance(entry, dict) else {}
        code = reason.get("Code")
        message = reason.get("Message")
        reasons.append(
            CancellationReason(
                index=index,
                code=str(code) if code is not None else "",
                message=str(message) if message is not None else "",
            )
        )
    return tuple(reasons)


def map_transaction_error(err: ClientError) -> Exception:
    code = str(err.response.get("Error", {}).get("Code", ""))
    message = str(err.response.get("Error", {}).get("Message", ""))

    if code == "TransactionCanceledException":
        reasons = _cancellation_reasons(err.response.get("CancellationReasons"))
        reason_codes = tuple(reason.code for reason in reasons)
        first_real = next(
            (reason for reason in reasons if reason.code not in ("", "None")),
            None,
        )

        if first_real is not None:
            classification = _CANCELLATION_CLASSIFICATION.get(first_real.code)
            if classification == "condition_failed":
                return ConditionFailedError(
                    message or "transaction canceled: ConditionalCheckFailed",
                    reasons=reasons,
                )
            if classification == "transaction_conflict":
                return TransactionConflictError(
                    message=message or "transaction canceled: TransactionConflict",
                    reason_codes=reason_codes,
                    reasons=reasons,
                )
            if classification == "throttled":
                return ThrottledError(
                    message=message or "transaction canceled: throttled",
                    reason_codes=reason_codes,
                    reasons=reasons,
                )

        return TransactionCanceledError(
            message=message or "transaction canceled",
            reason_codes=reason_codes,
            reasons=reasons,
        )

    return map_client_error(err)
