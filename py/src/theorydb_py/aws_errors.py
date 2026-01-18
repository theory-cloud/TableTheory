from __future__ import annotations

from botocore.exceptions import ClientError

from .errors import (
    AwsError,
    ConditionFailedError,
    NotFoundError,
    TransactionCanceledError,
    ValidationError,
)


def map_client_error(err: ClientError) -> Exception:
    code = str(err.response.get("Error", {}).get("Code", ""))
    message = str(err.response.get("Error", {}).get("Message", ""))

    if code == "ConditionalCheckFailedException":
        return ConditionFailedError(message)
    if code == "ValidationException":
        return ValidationError(message)
    if code == "ResourceNotFoundException":
        return NotFoundError(message)

    return AwsError(code=code or "UnknownError", message=message or str(err))


def map_transaction_error(err: ClientError) -> Exception:
    code = str(err.response.get("Error", {}).get("Code", ""))
    message = str(err.response.get("Error", {}).get("Message", ""))

    if code == "TransactionCanceledException":
        reasons_raw = err.response.get("CancellationReasons") or []
        reason_codes = tuple(
            str(reason.get("Code", "Unknown"))
            for reason in reasons_raw
            if isinstance(reason, dict) and reason.get("Code")
        )

        if any(rc == "ConditionalCheckFailed" for rc in reason_codes) or "ConditionalCheckFailed" in message:
            return ConditionFailedError(message or "transaction canceled: ConditionalCheckFailed")

        return TransactionCanceledError(
            message=message or "transaction canceled",
            reason_codes=reason_codes,
        )

    return map_client_error(err)
