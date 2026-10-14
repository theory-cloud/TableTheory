from __future__ import annotations

from typing import Any, cast

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


def strip_item_images(err: BaseException) -> BaseException:
    """Return a copy of ``err`` with DynamoDB item images removed from its response.

    Only a ``botocore.exceptions.ClientError`` whose ``response`` is a dict
    carrying ``CancellationReasons`` and/or a top-level ``Item`` is copied. Each
    dict entry of ``CancellationReasons`` has its ``Item`` key removed and the
    top-level ``Item`` key is removed. The original error and its ``response``
    are never mutated. Anything else, or a response with nothing to strip, is
    returned unchanged.
    """

    if not isinstance(err, ClientError):
        return err
    response: Any = err.response
    if not isinstance(response, dict):
        return err

    reasons = response.get("CancellationReasons")
    has_reason_item = isinstance(reasons, list) and any(
        isinstance(entry, dict) and "Item" in entry for entry in reasons
    )
    if not has_reason_item and "Item" not in response:
        return err

    redacted: dict[str, Any] = dict(response)
    if isinstance(reasons, list):
        redacted["CancellationReasons"] = [
            {key: value for key, value in entry.items() if key != "Item"}
            if isinstance(entry, dict)
            else entry
            for entry in reasons
        ]
    redacted.pop("Item", None)

    operation_name = cast(str, getattr(err, "operation_name", None))
    return ClientError(cast(Any, redacted), operation_name)


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
