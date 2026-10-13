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


def _root(name: str) -> Callable[[dict[str, Any]], str]:
    def read(item: dict[str, Any]) -> str:
        if name not in item:
            return "ABSENT"
        return next(iter(item[name]))

    return read


def _nested(name: str) -> Callable[[dict[str, Any]], str]:
    def read(item: dict[str, Any]) -> str:
        container = item.get("nested")
        if not isinstance(container, dict):
            return "ABSENT"
        values = container.get("M")
        if not isinstance(values, dict) or name not in values:
            return "ABSENT"
        return next(iter(values[name]))

    return read


def test_preflight_byte_equals_captured_writer_payloads() -> None:
    item = _matrix_item()

    put_client = StatefulDynamoDBClient()
    _table(put_client).put(item)
    captured_put = put_client.items(_TABLE_NAME)[0]

    transact_client = StatefulDynamoDBClient()
    _table(transact_client).transact_write([TransactPut(item)])
    captured_transact = transact_client.items(_TABLE_NAME)[0]

    assert (
        _table(StatefulDynamoDBClient()).marshal_item_for_writer_path(item, ItemWriterPath.PUT)
        == captured_put
    )
    assert (
        _table(StatefulDynamoDBClient()).marshal_item_for_writer_path(item, ItemWriterPath.TRANSACT_WRITE)
        == captured_transact
    )

    # Python has one item serializer for both writer paths, so unlike Go the two
    # captured payloads agree here: None is stored as DynamoDB NULL on both.
    assert captured_put == captured_transact


# Python maps None to DynamoDB NULL on every writer path. That matches Go's put
# family but not Go's TransactWrite, which stores an empty L/M instead; the
# Python runtime has no such split.
_MATRIX: list[tuple[str, Callable[[dict[str, Any]], str], str]] = [
    ("root nil list without omitempty", _root("nilList"), "NULL"),
    ("root empty list without omitempty", _root("emptyList"), "L"),
    ("root nil map without omitempty", _root("nilMap"), "NULL"),
    ("root empty map without omitempty", _root("emptyMap"), "M"),
    ("root nil list with omitempty", _root("nilListOmit"), "ABSENT"),
    ("root empty list with omitempty", _root("emptyListOmit"), "ABSENT"),
    ("root nil map with omitempty", _root("nilMapOmit"), "ABSENT"),
    ("root empty map with omitempty", _root("emptyMapOmit"), "ABSENT"),
    ("root false bool without omitempty", _root("falseBool"), "BOOL"),
    ("root zero int without omitempty", _root("zeroInt"), "N"),
    ("root empty set on set-backed attribute", _root("emptySet"), "NULL"),
    ("nested nil list", _nested("nestedNilList"), "NULL"),
    ("nested empty list", _nested("nestedEmptyList"), "L"),
    ("nested nil map", _nested("nestedNilMap"), "NULL"),
    ("nested empty map", _nested("nestedEmptyMap"), "M"),
]


@pytest.mark.parametrize("path", list(ItemWriterPath))
@pytest.mark.parametrize(("name", "read", "expected"), _MATRIX, ids=[case[0] for case in _MATRIX])
def test_preflight_nil_empty_matrix(
    path: ItemWriterPath, name: str, read: Callable[[dict[str, Any]], str], expected: str
) -> None:
    table = _table(StatefulDynamoDBClient())
    item = table.marshal_item_for_writer_path(_matrix_item(), path)
    assert read(item) == expected, name


@pytest.mark.parametrize("path", list(ItemWriterPath))
def test_preflight_equals_writer_output_for_both_paths(path: ItemWriterPath) -> None:
    client = StatefulDynamoDBClient()
    table = _table(client)
    item = _matrix_item()

    if path is ItemWriterPath.PUT:
        table.put(item)
    else:
        table.transact_write([TransactPut(item)])

    assert table.marshal_item_for_writer_path(item, path) == client.items(_TABLE_NAME)[0]


def test_preflight_refuses_unknown_path() -> None:
    table = _table(StatefulDynamoDBClient())
    with pytest.raises(ValidationError):
        table.marshal_item_for_writer_path(_matrix_item(), cast(ItemWriterPath, "nope"))


def test_preflight_refuses_encrypted_model() -> None:
    model = ModelDefinition.from_dataclass(PreflightEncrypted, table_name="preflight_encrypted")
    table = Table(
        model,
        client=StatefulDynamoDBClient(),
        kms_key_arn="arn:aws:kms:us-east-1:000000000000:key/preflight",
        kms_client=object(),
    )
    with pytest.raises(ValidationError, match="encrypted"):
        table.marshal_item_for_writer_path(PreflightEncrypted(pk="p1"), ItemWriterPath.PUT)
