from __future__ import annotations

from collections.abc import Callable
from dataclasses import dataclass
from typing import Any, cast

import pytest

from tabletheory_py import (
    ItemWriterPath,
    ModelDefinition,
    StatefulDynamoDBClient,
    Table,
    TransactPut,
    ValidationError,
    marshal_item_for_writer_path,
    theorydb_field,
)

_TABLE_NAME = "preflight_matrix"


@dataclass(frozen=True)
class PreflightMatrix:
    pk: str = theorydb_field(name="PK", roles=["pk"])
    sk: str = theorydb_field(name="SK", roles=["sk"])
    nil_list: list[str] | None = theorydb_field(name="nilList", default=None)
    empty_list: list[str] = theorydb_field(name="emptyList", default_factory=list)
    nil_map: dict[str, str] | None = theorydb_field(name="nilMap", default=None)
    empty_map: dict[str, str] = theorydb_field(name="emptyMap", default_factory=dict)
    nil_list_omit: list[str] | None = theorydb_field(name="nilListOmit", omitempty=True, default=None)
    empty_list_omit: list[str] = theorydb_field(name="emptyListOmit", omitempty=True, default_factory=list)
    nil_map_omit: dict[str, str] | None = theorydb_field(name="nilMapOmit", omitempty=True, default=None)
    empty_map_omit: dict[str, str] = theorydb_field(name="emptyMapOmit", omitempty=True, default_factory=dict)
    false_bool: bool = theorydb_field(name="falseBool", default=False)
    zero_int: int = theorydb_field(name="zeroInt", default=0)
    empty_set: set[str] = theorydb_field(name="emptySet", set_=True, default_factory=set)
    nested: dict[str, Any] = theorydb_field(name="nested", default_factory=dict)


@dataclass(frozen=True)
class PreflightEncrypted:
    pk: str = theorydb_field(name="PK", roles=["pk"])
    secret: str = theorydb_field(name="secret", encrypted=True, default="x")


def _matrix_item() -> PreflightMatrix:
    return PreflightMatrix(
        pk="p1",
        sk="s1",
        empty_list=[],
        empty_map={},
        empty_list_omit=[],
        empty_map_omit={},
        nested={
            "nestedNilList": None,
            "nestedEmptyList": [],
            "nestedNilMap": None,
            "nestedEmptyMap": {},
        },
    )


def _table(client: StatefulDynamoDBClient) -> Table[PreflightMatrix]:
    model = ModelDefinition.from_dataclass(PreflightMatrix, table_name=_TABLE_NAME)
    return Table(model, client=client)


def test_preflight_byte_equals_captured_writer_payloads() -> None:
    item = _matrix_item()

    put_client = StatefulDynamoDBClient()
    _table(put_client).put(item)
    captured_put = put_client.items(_TABLE_NAME)[0]

    transact_client = StatefulDynamoDBClient()
    _table(transact_client).transact_write([TransactPut(item)])
    captured_transact = transact_client.items(_TABLE_NAME)[0]

    assert (
        marshal_item_for_writer_path(_table(StatefulDynamoDBClient()), item, ItemWriterPath.PUT)
        == captured_put
    )
    assert (
        marshal_item_for_writer_path(_table(StatefulDynamoDBClient()), item, ItemWriterPath.TRANSACT_WRITE)
        == captured_transact
    )

    # Python has one item serializer for both writer paths, so unlike Go the two
    # captured payloads agree here: None is stored as DynamoDB NULL on both.
    assert captured_put == captured_transact


@pytest.mark.parametrize("path", list(ItemWriterPath))
def test_preflight_equals_writer_output_for_both_paths(path: ItemWriterPath) -> None:
    client = StatefulDynamoDBClient()
    table = _table(client)
    item = _matrix_item()

    if path is ItemWriterPath.PUT:
        table.put(item)
    else:
        table.transact_write([TransactPut(item)])

    assert marshal_item_for_writer_path(table, item, path) == client.items(_TABLE_NAME)[0]


def test_preflight_refuses_unknown_path() -> None:
    table = _table(StatefulDynamoDBClient())
    with pytest.raises(ValidationError):
        marshal_item_for_writer_path(table, _matrix_item(), cast(ItemWriterPath, "nope"))


def test_preflight_refuses_encrypted_model() -> None:
    model = ModelDefinition.from_dataclass(PreflightEncrypted, table_name="preflight_encrypted")
    table = Table(
        model,
        client=StatefulDynamoDBClient(),
        kms_key_arn="arn:aws:kms:us-east-1:000000000000:key/preflight",
        kms_client=object(),
    )
    with pytest.raises(ValidationError, match="encrypted"):
        marshal_item_for_writer_path(table, PreflightEncrypted(pk="p1"), ItemWriterPath.PUT)


_STAMP = "2026-01-02T03:04:05.123456789Z"


@dataclass(frozen=True)
class PreflightStamped:
    pk: str = theorydb_field(name="PK", roles=["pk"])
    sk: str = theorydb_field(name="SK", roles=["sk"])
    created_at: str = theorydb_field(name="createdAt", roles=["created_at"], default="")
    updated_at: str = theorydb_field(name="updatedAt", roles=["updated_at"], default="")


def _stamped_table(
    client: StatefulDynamoDBClient, clock: Callable[[], str] | None = None
) -> Table[PreflightStamped]:
    model = ModelDefinition.from_dataclass(PreflightStamped, table_name="preflight_stamped")
    return Table(model, client=client, now=clock or (lambda: _STAMP))


def test_preflight_matches_a_fixed_lifecycle_clock_on_both_writer_paths() -> None:
    item = PreflightStamped(pk="p1", sk="s1")

    put_client = StatefulDynamoDBClient()
    _stamped_table(put_client).put(item)
    captured_put = put_client.items("preflight_stamped")[0]

    transact_client = StatefulDynamoDBClient()
    _stamped_table(transact_client).transact_write([TransactPut(item)])
    captured_transact = transact_client.items("preflight_stamped")[0]

    assert captured_put == captured_transact, "both writer paths stamp from the same clock"

    table = _stamped_table(StatefulDynamoDBClient())
    assert marshal_item_for_writer_path(table, item, ItemWriterPath.PUT, now=_STAMP) == captured_put
    assert (
        marshal_item_for_writer_path(table, item, ItemWriterPath.TRANSACT_WRITE, now=_STAMP)
        == captured_transact
    )


def test_preflight_refuses_a_lifecycle_model_without_the_writer_clock() -> None:
    table = _stamped_table(StatefulDynamoDBClient())
    item = PreflightStamped(pk="p1", sk="s1")

    for path in (ItemWriterPath.PUT, ItemWriterPath.TRANSACT_WRITE):
        with pytest.raises(ValidationError, match="lifecycle clock"):
            marshal_item_for_writer_path(table, item, path)


def test_preflight_reads_the_supplied_stamp_not_the_table_clock() -> None:
    stamps = iter(["2026-01-02T03:04:05.000000001Z", "2026-01-02T03:04:06.000000002Z"])
    advancing = _stamped_table(StatefulDynamoDBClient(), clock=lambda: next(stamps))
    item = PreflightStamped(pk="p1", sk="s1")

    captured_client = StatefulDynamoDBClient()
    _stamped_table(
        captured_client,
        clock=iter(["2026-01-02T03:04:05.000000001Z", "2026-01-02T03:04:06.000000002Z"]).__next__,
    ).put(item)
    captured = captured_client.items("preflight_stamped")[0]

    preflight = marshal_item_for_writer_path(
        advancing, item, ItemWriterPath.PUT, now="2026-01-02T03:04:05.000000001Z"
    )
    assert preflight == captured


def test_preflight_keeps_working_without_a_clock_for_models_without_lifecycle_fields() -> None:
    table = _table(StatefulDynamoDBClient())
    assert marshal_item_for_writer_path(table, _matrix_item(), ItemWriterPath.PUT)
