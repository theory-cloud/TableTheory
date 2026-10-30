from __future__ import annotations

import json
import threading
from dataclasses import dataclass

import pytest
from botocore.exceptions import ClientError

from tabletheory_py import (
    AwsError,
    BatchRetryExceededError,
    CancellationReason,
    ConditionFailedError,
    ModelDefinition,
    StatefulDynamoDBClient,
    Table,
    TransactionCanceledError,
    TransactPut,
    theorydb_field,
)
from tabletheory_py.aws_errors import map_transaction_error as _map_transaction_error
from tabletheory_py.aws_errors import redacting_client, strip_item_images

_SECRET = "top-secret-item-image"


@dataclass(frozen=True)
class SafeNote:
    pk: str = theorydb_field(name="PK", roles=["pk"])
    sk: str = theorydb_field(name="SK", roles=["sk"])
    name: str = theorydb_field(name="name")


def _table(client: StatefulDynamoDBClient, *, redact_item_images: bool = False) -> Table[SafeNote]:
    model = ModelDefinition.from_dataclass(SafeNote, table_name="safe_notes")
    return Table(
        model,
        client=client,
        now=lambda: "2026-07-04T00:00:00.000000000Z",
        redact_item_images=redact_item_images,
    )


def _cancel_with_images(code: str = "ConditionalCheckFailed") -> ClientError:
    """A transaction cancellation response carrying item images in both shapes."""

    item = {"PK": {"S": "USER#1"}, "name": {"S": _SECRET}}
    return ClientError(
        {
            "Error": {"Code": "TransactionCanceledException", "Message": "transaction canceled"},
            "CancellationReasons": [
                {"Code": "None"},
                {
                    "Code": code,
                    "Message": "loser",
                    "Item": item,
                },
            ],
            "Item": item,
        },
        "TransactWriteItems",
    )


def _run_transact(table: Table[SafeNote]) -> ConditionFailedError:
    with pytest.raises(ConditionFailedError) as info:
        table.transact_write([TransactPut(SafeNote(pk="USER#1", sk="PROFILE", name="new"))])
    return info.value


def test_default_off_keeps_original_cause_identity_and_response() -> None:
    client = StatefulDynamoDBClient()
    table = _table(client)
    original = _cancel_with_images()
    client.queue_transaction_error(original)

    mapped = _run_transact(table)

    assert mapped.__cause__ is original
    assert original.response["CancellationReasons"][1]["Item"]["name"]["S"] == _SECRET
    assert original.response["Item"]["name"]["S"] == _SECRET


def test_opt_in_redacts_item_images_from_the_retained_cause() -> None:
    client = StatefulDynamoDBClient()
    table = _table(client, redact_item_images=True)
    original = _cancel_with_images()
    client.queue_transaction_error(original)

    mapped = _run_transact(table)

    cause = mapped.__cause__
    assert cause is not original
    assert isinstance(cause, ClientError)
    cause_response = cause.response
    assert "Item" not in cause_response["CancellationReasons"][1]
    assert "Item" not in cause_response
    assert _SECRET not in json.dumps(cause_response)

    # The original error is untouched: no in-place mutation happened.
    assert original.response["CancellationReasons"][1]["Item"]["name"]["S"] == _SECRET
    assert original.response["Item"]["name"]["S"] == _SECRET


def test_transaction_canceled_safe_dict_is_typed_and_never_leaks_items() -> None:
    err = TransactionCanceledError(
        message="transaction canceled",
        reason_codes=("None", "ConditionalCheckFailed"),
        reasons=(
            CancellationReason(index=0, code="None", message=""),
            CancellationReason(index=1, code="ConditionalCheckFailed", message="loser"),
        ),
    )

    assert err.safe_dict() == {
        "type": "TransactionCanceledError",
        "message": "transaction canceled",
        "reason_codes": ["None", "ConditionalCheckFailed"],
        "reasons": [
            {"index": 0, "code": "None", "message": ""},
            {"index": 1, "code": "ConditionalCheckFailed", "message": "loser"},
        ],
    }


def test_safe_dict_mapped_from_item_images_never_contains_the_secret() -> None:
    mapped = _map_transaction_error(_cancel_with_images(code="SomeUnmappedReason"))

    assert isinstance(mapped, TransactionCanceledError)
    safe = mapped.safe_dict()
    assert set(safe) == {"type", "message", "reason_codes", "reasons"}
    assert safe["reason_codes"] == ["None", "SomeUnmappedReason"]
    assert _SECRET not in json.dumps(safe)


def test_safe_dict_shapes_for_simple_errors() -> None:
    assert ConditionFailedError(
        "conditional check failed",
        reasons=(CancellationReason(index=0, code="ConditionalCheckFailed", message="lost"),),
    ).safe_dict() == {
        "type": "ConditionFailedError",
        "message": "conditional check failed",
        "reasons": [
            {"index": 0, "code": "ConditionalCheckFailed", "message": "lost"},
        ],
    }
    assert AwsError(code="UnknownError", message="boom").safe_dict() == {
        "type": "AwsError",
        "code": "UnknownError",
        "message": "boom",
    }
    assert BatchRetryExceededError(operation="batch_get", unprocessed_count=3).safe_dict() == {
        "type": "BatchRetryExceededError",
        "message": "batch_get: retry limit exceeded (unprocessed=3)",
        "operation": "batch_get",
        "unprocessed_count": 3,
    }


def test_strip_item_images_is_a_noop_when_there_is_nothing_to_strip() -> None:
    not_client_error = ValueError("nope")
    assert strip_item_images(not_client_error) is not_client_error

    clean = ClientError(
        {
            "Error": {"Code": "TransactionCanceledException", "Message": "canceled"},
            "CancellationReasons": [{"Code": "None"}, {"Code": "ConditionalCheckFailed"}],
        },
        "TransactWriteItems",
    )
    assert strip_item_images(clean) is clean


def test_strip_item_images_passes_through_non_dict_reason_entries() -> None:
    original = ClientError(
        {
            "Error": {"Code": "TransactionCanceledException", "Message": "canceled"},
            "CancellationReasons": ["not-a-dict", {"Code": "None", "Item": {"PK": {"S": "USER#1"}}}],
            "Item": {"PK": {"S": "USER#1"}},
        },
        "TransactWriteItems",
    )

    stripped = strip_item_images(original)

    assert isinstance(stripped, ClientError)
    assert stripped.operation_name == "TransactWriteItems"
    assert stripped.response["CancellationReasons"][0] == "not-a-dict"
    assert stripped.response["CancellationReasons"][1] == {"Code": "None"}
    assert "Item" not in stripped.response
    # The original still carries its images.
    assert original.response["CancellationReasons"][1]["Item"] == {"PK": {"S": "USER#1"}}


def test_strip_item_images_handles_response_without_reasons() -> None:
    original = ClientError(
        {
            "Error": {"Code": "ConditionalCheckFailedException", "Message": "failed"},
            "Item": {"PK": {"S": "USER#1"}, "name": {"S": _SECRET}},
        },
        "PutItem",
    )

    stripped = strip_item_images(original)

    assert isinstance(stripped, ClientError)
    assert stripped.operation_name == "PutItem"
    assert "Item" not in stripped.response
    assert _SECRET not in json.dumps(stripped.response)
    assert original.response["Item"]["name"]["S"] == _SECRET


def test_strip_item_images_tolerates_missing_operation_name() -> None:
    original = ClientError(
        {
            "Error": {"Code": "ConditionalCheckFailedException", "Message": "failed"},
            "Item": {"PK": {"S": "USER#1"}},
        },
        None,
    )

    stripped = strip_item_images(original)

    assert isinstance(stripped, ClientError)
    assert "Item" not in stripped.response


class _RaisingClient:
    """A minimal client whose single call raises the supplied error."""

    calls = 0

    def __init__(self, err: BaseException) -> None:
        self._err = err

    def put_item(self, **_: object) -> None:
        _RaisingClient.calls += 1
        raise self._err


def test_redacting_client_strips_item_images_and_leaves_the_original_alone() -> None:
    original = _cancel_with_images()
    inner = _RaisingClient(original)

    assert redacting_client(inner, False) is inner

    wrapped = redacting_client(inner, True)
    assert wrapped is not inner
    with pytest.raises(ClientError) as info:
        wrapped.put_item(TableName="safe_notes", Item={})

    raised = info.value
    assert raised is not original
    assert "Item" not in raised.response
    assert "Item" not in raised.response["CancellationReasons"][1]
    assert _SECRET not in json.dumps(raised.response)
    # Neither the cause nor the implicit context reaches the original error.
    assert raised.__cause__ is None
    assert raised.__context__ is None
    # The original object is untouched.
    assert original.response["Item"]["name"]["S"] == _SECRET


def test_with_lambda_timeout_preserves_redaction_and_canonical_client() -> None:
    class _Context:
        def get_remaining_time_in_millis(self) -> int:
            return 60_000

    client = StatefulDynamoDBClient()
    table = _table(client, redact_item_images=True)
    derived = table.with_lambda_timeout(_Context(), buffer_seconds=1.0)

    assert derived is not table
    assert derived._raw_client is client

    original = _cancel_with_images()
    client.queue_transaction_error(original)

    mapped = _run_transact(derived)

    cause = mapped.__cause__
    assert cause is not original
    assert isinstance(cause, ClientError)
    assert "Item" not in cause.response
    assert _SECRET not in json.dumps(cause.response)
    assert original.response["Item"]["name"]["S"] == _SECRET


class _SegmentImageErrorClient:
    """Thread-safe scan double whose segment 0 fails with an item-image error."""

    def __init__(self, err: ClientError) -> None:
        self._err = err
        self._lock = threading.Lock()
        self.calls: list[dict[str, object]] = []

    def scan(self, **req: object) -> dict[str, object]:
        with self._lock:
            self.calls.append(dict(req))
        if req.get("Segment") == 0:
            raise self._err
        return {"Items": []}


def _scan_with_images() -> ClientError:
    item = {"PK": {"S": "USER#1"}, "name": {"S": _SECRET}}
    return ClientError(
        {
            "Error": {"Code": "ProvisionedThroughputExceededException", "Message": "slow down"},
            "CancellationReasons": [{"Code": "ConditionalCheckFailed", "Item": item}],
            "Item": item,
        },
        "Scan",
    )


def test_segment_error_from_timeout_derived_table_stays_redacted() -> None:
    class _Context:
        def get_remaining_time_in_millis(self) -> int:
            return 60_000

    original = _scan_with_images()
    client = _SegmentImageErrorClient(original)
    derived = _table(client, redact_item_images=True).with_lambda_timeout(_Context(), buffer_seconds=1.0)

    with pytest.raises(AwsError) as info:
        derived.scan_all_segments(total_segments=2, max_workers=2)

    mapped = info.value
    assert mapped.code == "ProvisionedThroughputExceededException"
    assert _SECRET not in str(mapped)
    cause = mapped.__cause__
    assert isinstance(cause, ClientError)
    assert "Item" not in cause.response
    assert "Item" not in cause.response["CancellationReasons"][0]
    assert _SECRET not in json.dumps(cause.response)
    # The worker clone leads back to the caller's raw client, and the original
    # error object is untouched.
    assert derived._raw_client is client
    assert original.response["Item"]["name"]["S"] == _SECRET


def test_segment_error_keeps_the_non_redaction_default() -> None:
    class _Context:
        def get_remaining_time_in_millis(self) -> int:
            return 60_000

    original = _scan_with_images()
    client = _SegmentImageErrorClient(original)
    derived = _table(client).with_lambda_timeout(_Context(), buffer_seconds=1.0)

    with pytest.raises(AwsError) as info:
        derived.scan_all_segments(total_segments=2, max_workers=2)

    # With redaction off, no extra wrapper is inserted between the caller and
    # the client, so the retained cause still carries the item image.
    cause = info.value.__cause__
    assert isinstance(cause, ClientError)
    assert cause is original
