from __future__ import annotations

from dataclasses import dataclass
from typing import Any

import pytest
from botocore.exceptions import ClientError

from tabletheory_py import (
    CancellationReason,
    ConditionFailedError,
    ModelDefinition,
    StatefulDynamoDBClient,
    Table,
    ThrottledError,
    TransactConditionCheck,
    TransactionCanceledError,
    TransactionConflictError,
    TransactPut,
    theorydb_field,
)
from tabletheory_py.aws_errors import map_transaction_error as _map_transaction_error


def _cancel(
    reasons: list[dict[str, Any]] | None = None,
    message: str = "transaction canceled",
) -> ClientError:
    response: dict[str, Any] = {"Error": {"Code": "TransactionCanceledException", "Message": message}}
    if reasons is not None:
        response["CancellationReasons"] = reasons
    return ClientError(response, "TransactWriteItems")


def test_first_genuine_failure_decides_classification() -> None:
    mapped = _map_transaction_error(
        _cancel(
            [
                {"Code": "None"},
                {"Code": "TransactionConflict", "Message": "raced"},
                {"Code": "ConditionalCheckFailed", "Message": "loser"},
            ]
        )
    )

    # The first genuine failure wins; a later condition failure must not override.
    assert isinstance(mapped, TransactionConflictError)
    assert isinstance(mapped, TransactionCanceledError)
    assert not isinstance(mapped, ConditionFailedError)
    assert mapped.reasons == (
        CancellationReason(index=0, code="None", message=""),
        CancellationReason(index=1, code="TransactionConflict", message="raced"),
        CancellationReason(index=2, code="ConditionalCheckFailed", message="loser"),
    )
    assert mapped.reason_codes == (
        "None",
        "TransactionConflict",
        "ConditionalCheckFailed",
    )


def test_condition_failed_keeps_every_reason_in_position() -> None:
    mapped = _map_transaction_error(
        _cancel(
            [
                {"Code": "None"},
                {"Code": "ConditionalCheckFailed", "Message": "survivor failed"},
            ]
        )
    )

    assert isinstance(mapped, ConditionFailedError)
    assert mapped.reasons == (
        CancellationReason(index=0, code="None", message=""),
        CancellationReason(index=1, code="ConditionalCheckFailed", message="survivor failed"),
    )


def test_throttled_unknown_and_empty_classification() -> None:
    assert isinstance(_map_transaction_error(_cancel([{"Code": "ThrottlingError"}])), ThrottledError)
    assert isinstance(
        _map_transaction_error(_cancel([{"Code": "ProvisionedThroughputExceeded"}])),
        ThrottledError,
    )

    unknown = _map_transaction_error(_cancel([{"Code": "SomeUnmappedReason"}]))
    assert isinstance(unknown, TransactionCanceledError)
    assert not isinstance(unknown, ConditionFailedError)

    empty = _map_transaction_error(_cancel([]))
    assert isinstance(empty, TransactionCanceledError)
    assert not isinstance(empty, ConditionFailedError)
    assert empty.reasons == ()
    assert empty.reason_codes == ()


def test_missing_code_keeps_position() -> None:
    mapped = _map_transaction_error(_cancel([{"Message": "no code"}, {"Code": "ConditionalCheckFailed"}]))

    assert isinstance(mapped, ConditionFailedError)
    assert [reason.index for reason in mapped.reasons] == [0, 1]
    assert [reason.code for reason in mapped.reasons] == ["", "ConditionalCheckFailed"]


def test_item_images_never_surface() -> None:
    mapped = _map_transaction_error(
        _cancel(
            [
                {
                    "Code": "ConditionalCheckFailed",
                    "Message": "boom",
                    "Item": {"secret": {"S": "top-secret-value"}},
                }
            ]
        )
    )

    assert "top-secret-value" not in repr(mapped.reasons)
    assert "top-secret-value" not in str(mapped)


@dataclass(frozen=True)
class CancelNote:
    pk: str = theorydb_field(name="PK", roles=["pk"])
    sk: str = theorydb_field(name="SK", roles=["sk"])
    name: str = theorydb_field(name="name")


def _table(client: StatefulDynamoDBClient) -> Table[CancelNote]:
    model = ModelDefinition.from_dataclass(CancelNote, table_name="cancel_notes")
    return Table(model, client=client, now=lambda: "2026-07-04T00:00:00.000000000Z")


def test_fakedb_emits_positioned_reasons_and_chains_the_original_error() -> None:
    client = StatefulDynamoDBClient()
    table = _table(client)
    table.put(CancelNote(pk="USER#0", sk="PROFILE", name="exists"))

    with pytest.raises(ConditionFailedError) as info:
        table.transact_write(
            [
                TransactPut(CancelNote(pk="USER#1", sk="PROFILE", name="new")),
                TransactConditionCheck(
                    pk="USER#0",
                    sk="PROFILE",
                    condition_expression="attribute_not_exists(PK)",
                ),
            ]
        )

    mapped = info.value
    assert isinstance(mapped.__cause__, ClientError)
    assert [reason.index for reason in mapped.reasons] == [0, 1]
    assert [reason.code for reason in mapped.reasons] == [
        "None",
        "ConditionalCheckFailed",
    ]


def test_fakedb_injection_hook_classifies_synthetic_reasons() -> None:
    client = StatefulDynamoDBClient()
    table = _table(client)
    client.queue_transaction_cancellation(
        [{"Code": "None"}, {"Code": "TransactionConflict", "Message": "raced"}]
    )

    with pytest.raises(TransactionConflictError) as info:
        table.transact_write([TransactPut(CancelNote(pk="USER#1", sk="PROFILE", name="new"))])

    assert [reason.code for reason in info.value.reasons] == [
        "None",
        "TransactionConflict",
    ]
