from __future__ import annotations

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
