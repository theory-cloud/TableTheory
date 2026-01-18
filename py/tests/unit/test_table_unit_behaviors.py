from __future__ import annotations

from dataclasses import dataclass

import pytest
from botocore.exceptions import ClientError

from theorydb_py import (
    EncryptionNotConfiguredError,
    ModelDefinition,
    NotFoundError,
    SortKeyCondition,
    Table,
    ValidationError,
    theorydb_field,
)
from theorydb_py.model import Projection, gsi
from theorydb_py.table import _backoff_seconds, _chunked, _map_client_error, _map_transaction_error


@dataclass(frozen=True)
class Item:
    pk: str = theorydb_field(name="PK", roles=["pk"])
    sk: str = theorydb_field(name="SK", roles=["sk"])
    value: int = theorydb_field(name="value")
    note: str = theorydb_field(name="note", omitempty=True, default="")


class _StubClient:
    def __init__(self) -> None:
        self.put_reqs: list[dict] = []
        self.get_reqs: list[dict] = []
        self.delete_reqs: list[dict] = []
        self.update_reqs: list[dict] = []
        self.query_reqs: list[dict] = []
        self.scan_reqs: list[dict] = []
        self._get_item: dict | None = None
        self._update_attrs: dict | None = None
        self._items: list[dict] = []
        self._last_key: dict | None = None

    def set_get_item(self, item: dict | None) -> None:
        self._get_item = item

    def set_update_attrs(self, attrs: dict | None) -> None:
        self._update_attrs = attrs

    def set_query_items(self, items: list[dict], *, last_key: dict | None = None) -> None:
        self._items = items
        self._last_key = last_key

    def put_item(self, **req):  # noqa: ANN001
        self.put_reqs.append(req)
        return {}

    def get_item(self, **req):  # noqa: ANN001
        self.get_reqs.append(req)
        return {"Item": self._get_item} if self._get_item is not None else {}

    def delete_item(self, **req):  # noqa: ANN001
        self.delete_reqs.append(req)
        return {}

    def update_item(self, **req):  # noqa: ANN001
        self.update_reqs.append(req)
        return {"Attributes": self._update_attrs} if self._update_attrs is not None else {}

    def query(self, **req):  # noqa: ANN001
        self.query_reqs.append(req)
        out: dict = {"Items": self._items}
        if self._last_key is not None:
            out["LastEvaluatedKey"] = self._last_key
        return out

    def scan(self, **req):  # noqa: ANN001
        self.scan_reqs.append(req)
        out: dict = {"Items": self._items}
        if self._last_key is not None:
            out["LastEvaluatedKey"] = self._last_key
        return out


def test_chunked_and_backoff_helpers() -> None:
    assert _chunked([1, 2, 3, 4, 5], 2) == [[1, 2], [3, 4], [5]]
    with pytest.raises(ValueError):
        _chunked([1], 0)

    assert _backoff_seconds(1) == 0.05
    assert _backoff_seconds(2) == 0.1
    assert _backoff_seconds(10) == 1.0


def test_table_requires_table_name_and_encryption_config() -> None:
    model = ModelDefinition.from_dataclass(Item)
    with pytest.raises(ValueError):
        Table(model, client=object())

    @dataclass(frozen=True)
    class Secret:
        pk: str = theorydb_field(roles=["pk"])
        secret: str = theorydb_field(encrypted=True)

    secret_model = ModelDefinition.from_dataclass(Secret, table_name="tbl")
    with pytest.raises(EncryptionNotConfiguredError):
        Table(secret_model, client=object())


def test_table_put_get_delete_update_happy_path_and_validation() -> None:
    model = ModelDefinition.from_dataclass(Item, table_name="tbl")
    stub = _StubClient()
    table: Table[Item] = Table(model, client=stub)

    table.put(Item(pk="A", sk="1", value=1))
    assert stub.put_reqs[0]["TableName"] == "tbl"
    assert "Item" in stub.put_reqs[0]

    stub.set_get_item(table._to_item(Item(pk="A", sk="1", value=1)))
    got = table.get("A", "1", consistent_read=True)
    assert got == Item(pk="A", sk="1", value=1, note="")
    assert stub.get_reqs[0]["ConsistentRead"] is True

    stub.set_get_item(None)
    with pytest.raises(NotFoundError):
        table.get("A", "1")

    table.delete("A", "1", condition_expression="attribute_exists(PK)")
    assert stub.delete_reqs[0]["ConditionExpression"] == "attribute_exists(PK)"

    stub.set_update_attrs(table._to_item(Item(pk="A", sk="1", value=2)))
    updated = table.update("A", "1", {"value": 2})
    assert updated.value == 2
    assert "UpdateExpression" in stub.update_reqs[0]

    stub.set_update_attrs(None)
    with pytest.raises(ValidationError, match="did not return Attributes"):
        table.update("A", "1", {"value": 3})


def test_table_key_and_update_request_validation_errors() -> None:
    model = ModelDefinition.from_dataclass(Item, table_name="tbl")
    table: Table[Item] = Table(model, client=_StubClient())

    with pytest.raises(ValidationError, match="pk is required"):
        table.get(None, "1")  # type: ignore[arg-type]
    with pytest.raises(ValidationError, match="sk is required"):
        table.get("A", None)  # type: ignore[arg-type]

    with pytest.raises(ValidationError, match="unknown field"):
        table._build_update_request("A", "1", {"nope": 1})
    with pytest.raises(ValidationError, match="cannot update key field"):
        table._build_update_request("A", "1", {"pk": "X"})
    with pytest.raises(ValidationError, match="no updates provided"):
        table._build_update_request("A", "1", {})

    req = table._build_update_request("A", "1", {"note": None, "value": 1})
    assert "REMOVE" in req["UpdateExpression"]
    assert "SET" in req["UpdateExpression"]

    with pytest.raises(ValidationError, match="name collision"):
        table._build_update_request(
            "A",
            "1",
            {"value": 1},
            expression_attribute_names={"#d_value": "value"},
        )

    with pytest.raises(ValidationError, match="value collision"):
        table._build_update_request(
            "A",
            "1",
            {"value": 1},
            expression_attribute_values={":d_value": 1},
        )


def test_table_query_scan_validations_and_projection() -> None:
    @dataclass(frozen=True)
    class Record:
        pk: str = theorydb_field(name="PK", roles=["pk"])
        sk: str = theorydb_field(name="SK", roles=["sk"])
        gsi_pk: str = theorydb_field(name="gsi_pk")
        value: int = theorydb_field(name="value")

    model = ModelDefinition.from_dataclass(
        Record,
        table_name="tbl",
        indexes=[gsi("gsi1", partition="gsi_pk", projection=Projection.all())],
    )
    stub = _StubClient()
    table: Table[Record] = Table(model, client=stub)

    with pytest.raises(ValidationError, match="consistent_read is not supported"):
        table.query("A", index_name="gsi1", consistent_read=True)
    with pytest.raises(ValidationError, match="partition is required"):
        table.query(None)  # type: ignore[arg-type]
    with pytest.raises(ValidationError, match="limit must be > 0"):
        table.query("A", limit=0)

    with pytest.raises(ValidationError, match="limit must be > 0"):
        table.scan(limit=0)
    with pytest.raises(ValidationError, match="invalid cursor"):
        table.scan(cursor="not-a-cursor")

    item = Record(pk="A", sk="1", gsi_pk="G", value=1)
    stub.set_query_items([table._to_item(item)], last_key=table._to_key("A", "1"))
    page = table.query(
        "A", sort=SortKeyCondition.begins_with("1"), projection=["pk", "sk", "gsi_pk", "value"]
    )
    assert page.items[0] == item
    assert page.next_cursor is not None
    assert "ProjectionExpression" in stub.query_reqs[0]

    with pytest.raises(ValidationError, match="projection is missing required fields"):
        table.query("A", projection=["pk", "sk"])


def test_error_mapping_helpers() -> None:
    err = ClientError({"Error": {"Code": "ConditionalCheckFailedException", "Message": "no"}}, "PutItem")
    mapped = _map_client_error(err)
    assert mapped.__class__.__name__ == "ConditionFailedError"

    tx_err = ClientError(
        {"Error": {"Code": "TransactionCanceledException", "Message": "ConditionalCheckFailed"}},
        "TransactWriteItems",
    )
    mapped_tx = _map_transaction_error(tx_err)
    assert mapped_tx.__class__.__name__ == "ConditionFailedError"
