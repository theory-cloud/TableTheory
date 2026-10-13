from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path
from typing import Any, cast

import pytest
import yaml
from botocore.exceptions import ClientError

from tabletheory_py import (
    ConditionFailedError,
    ModelDefinition,
    StatefulDynamoDBClient,
    Table,
    ThrottledError,
    TransactionCanceledError,
    TransactionConflictError,
    TransactPut,
    theorydb_field,
)

_FIXTURE_PATH = (
    Path(__file__).resolve().parents[2] / "scenarios" / "cancellation" / "01-transaction-cancellation.yml"
)


@dataclass(frozen=True)
class _CancelNote:
    pk: str = theorydb_field(name="PK", roles=["pk"])
    sk: str = theorydb_field(name="SK", roles=["sk"])
    name: str = theorydb_field(name="name")


def _table(client: StatefulDynamoDBClient) -> Table[_CancelNote]:
    model = ModelDefinition.from_dataclass(_CancelNote, table_name="cancel_notes")
    return Table(model, client=client, now=lambda: "2026-07-04T00:00:00.000000000Z")


def _load_cases() -> list[dict[str, Any]]:
    document = yaml.safe_load(_FIXTURE_PATH.read_text(encoding="utf-8"))
    assert isinstance(document, dict)
    cases = cast(list[dict[str, Any]], document.get("cases", []))
    assert cases, "fixture must declare cancellation cases"
    return cases


_CASES = _load_cases()


def _fixture_reason(entry: dict[str, Any]) -> dict[str, Any]:
    reason: dict[str, Any] = {}
    if entry.get("code") is not None:
        reason["Code"] = entry["code"]
    if entry.get("message") is not None:
        reason["Message"] = entry["message"]
    return reason


def _map_code(exc: Exception) -> str:
    if isinstance(exc, TransactionConflictError):
        return "ErrTransactionConflict"
    if isinstance(exc, ThrottledError):
        return "ErrThrottled"
    if isinstance(exc, ConditionFailedError):
        return "ErrConditionFailed"
    if isinstance(exc, TransactionCanceledError):
        return "ErrTransactionFailed"
    return ""


def _reason_codes(exc: Exception) -> list[str]:
    reasons = getattr(exc, "reasons", None)
    if not reasons:
        return []
    return [str(reason.code) for reason in reasons]


def _cause_recoverable(exc: Exception) -> bool:
    cause = getattr(exc, "__cause__", None)
    if not isinstance(cause, ClientError):
        return False
    return cause.response.get("Error", {}).get("Code") == "TransactionCanceledException"


@pytest.mark.parametrize("case", _CASES, ids=lambda case: str(case["name"]))
def test_transaction_cancellation_contract(case: dict[str, Any]) -> None:
    client = StatefulDynamoDBClient()
    table = _table(client)

    kind = case["kind"]
    if kind == "cancellation":
        client.queue_transaction_cancellation([_fixture_reason(entry) for entry in case.get("reasons", [])])
    elif kind == "transport":
        client.queue_transaction_error(
            ClientError(
                {"Error": {"Code": "SomeTransportError", "Message": case["error_message"]}},
                "TransactWriteItems",
            )
        )
    else:
        raise AssertionError(f"unknown fixture kind {kind!r}")

    with pytest.raises(Exception) as info:
        table.transact_write([TransactPut(_CancelNote(pk="USER#1", sk="PROFILE", name="new"))])

    exc = info.value
    expect = case["expect"]
    assert _map_code(exc) == expect["code"], f"{case['name']}: mapped error code"
    assert _reason_codes(exc) == list(expect["reason_codes"]), f"{case['name']}: ordered reason codes"
    assert _cause_recoverable(exc) == expect["cause_recoverable"], (
        f"{case['name']}: original cancellation recoverable"
    )
