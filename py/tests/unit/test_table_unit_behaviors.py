from __future__ import annotations

from dataclasses import dataclass

import pytest
from botocore.exceptions import ClientError

from tabletheory_py import (
    EncryptionNotConfiguredError,
    FilterCondition,
    ModelDefinition,
    NotFoundError,
    SortKeyCondition,
    Table,
    ValidationError,
    VersionConflictError,
    theorydb_field,
)
from tabletheory_py.model import Projection, gsi
from tabletheory_py.table import _backoff_seconds, _chunked, _map_client_error, _map_transaction_error


@dataclass(frozen=True)
class Item:
    pk: str = theorydb_field(name="PK", roles=["pk"])
    sk: str = theorydb_field(name="SK", roles=["sk"])
    value: int = theorydb_field(name="value")
    note: str = theorydb_field(name="note", omitempty=True, default="")


@dataclass(frozen=True)
class LifecycleItem:
    pk: str = theorydb_field(name="PK", roles=["pk"])
    sk: str = theorydb_field(name="SK", roles=["sk"])
    value: int = theorydb_field(name="value")
    created_at: str = theorydb_field(name="createdAt", roles=["created_at"], default="")
    updated_at: str = theorydb_field(name="updatedAt", roles=["updated_at"], default="")
    version: int | None = theorydb_field(name="version", roles=["version"], default=None)


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
        self._count = 0
        self._scanned_count = 0
        self._get_error: Exception | None = None

    def set_get_item(self, item: dict | None) -> None:
        self._get_item = item

    def set_get_error(self, err: Exception | None) -> None:
        self._get_error = err

    def set_update_attrs(self, attrs: dict | None) -> None:
        self._update_attrs = attrs

    def set_query_items(
        self,
        items: list[dict],
        *,
        last_key: dict | None = None,
        count: int | None = None,
        scanned_count: int | None = None,
    ) -> None:
        self._items = items
        self._last_key = last_key
        self._count = len(items) if count is None else count
        self._scanned_count = self._count if scanned_count is None else scanned_count

    def put_item(self, **req):  # noqa: ANN001
        self.put_reqs.append(req)
        return {}

    def get_item(self, **req):  # noqa: ANN001
        self.get_reqs.append(req)
        if self._get_error is not None:
            raise self._get_error
        return {"Item": self._get_item} if self._get_item is not None else {}

    def delete_item(self, **req):  # noqa: ANN001
        self.delete_reqs.append(req)
        return {}

    def update_item(self, **req):  # noqa: ANN001
        self.update_reqs.append(req)
        return {"Attributes": self._update_attrs} if self._update_attrs is not None else {}

    def query(self, **req):  # noqa: ANN001
        self.query_reqs.append(req)
        out: dict = {"Items": self._items, "Count": self._count, "ScannedCount": self._scanned_count}
        if self._last_key is not None:
            out["LastEvaluatedKey"] = self._last_key
            self._last_key = None
        return out

    def scan(self, **req):  # noqa: ANN001
        self.scan_reqs.append(req)
        out: dict = {"Items": self._items, "Count": self._count, "ScannedCount": self._scanned_count}
        if self._last_key is not None:
            out["LastEvaluatedKey"] = self._last_key
            self._last_key = None
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

    stub.set_get_item(table._to_item(Item(pk="A", sk="1", value=1)))
    assert table.get_or_none("A", "1") == Item(pk="A", sk="1", value=1, note="")

    stub.set_get_item(None)
    assert table.get_or_none("A", "1") is None

    stub.set_get_error(
        ClientError({"Error": {"Code": "ValidationException", "Message": "real error"}}, "GetItem")
    )
    with pytest.raises(ValidationError, match="real error"):
        table.get_or_none("A", "1")

    stub.set_get_error(
        ClientError({"Error": {"Code": "ResourceNotFoundException", "Message": "missing table"}}, "GetItem")
    )
    with pytest.raises(NotFoundError, match="missing table"):
        table.get_or_none("A", "1")
    stub.set_get_error(None)

    table.delete("A", "1", condition_expression="attribute_exists(PK)")
    assert stub.delete_reqs[0]["ConditionExpression"] == "attribute_exists(PK)"

    stub.set_update_attrs(table._to_item(Item(pk="A", sk="1", value=2)))
    updated = table.update("A", "1", {"value": 2})
    assert updated.value == 2
    assert "UpdateExpression" in stub.update_reqs[0]

    stub.set_update_attrs(None)
    with pytest.raises(ValidationError, match="did not return Attributes"):
        table.update("A", "1", {"value": 3})


def test_table_query_count_and_scan_count_use_select_count_without_materialization_controls() -> None:
    model = ModelDefinition.from_dataclass(Item, table_name="tbl")
    stub = _StubClient()
    table: Table[Item] = Table(model, client=stub)
    last_key = {"PK": {"S": "A"}, "SK": {"S": "1"}}
    stub.set_query_items(
        [table._to_item(Item(pk="A", sk="1", value=1))], last_key=last_key, count=2, scanned_count=5
    )

    query_count = table.query_count(
        "A",
        sort=SortKeyCondition.begins_with("ITEM#"),
        limit=1,
        projection=["PK"],
        filter=FilterCondition.eq("value", 1),
    )

    assert query_count == 4
    assert len(stub.query_reqs) == 2
    assert stub.query_reqs[0]["Select"] == "COUNT"
    assert "Limit" not in stub.query_reqs[0]
    assert "ProjectionExpression" not in stub.query_reqs[0]
    assert stub.query_reqs[1]["ExclusiveStartKey"] == last_key

    stub.set_query_items([table._to_item(Item(pk="A", sk="1", value=1))], count=3, scanned_count=7)
    scan_count = table.scan_count(limit=1, projection=["PK"], filter=FilterCondition.eq("value", 1))

    assert scan_count == 3
    assert stub.scan_reqs[0]["Select"] == "COUNT"
    assert "Limit" not in stub.scan_reqs[0]
    assert "ProjectionExpression" not in stub.scan_reqs[0]


def test_table_populates_lifecycle_fields_and_initial_version() -> None:
    model = ModelDefinition.from_dataclass(LifecycleItem, table_name="tbl")
    table: Table[LifecycleItem] = Table(
        model, client=_StubClient(), now=lambda: "2026-05-28T00:00:00.000000000Z"
    )

    item = table._to_item(LifecycleItem(pk="A", sk="1", value=1))

    assert item["createdAt"] == {"S": "2026-05-28T00:00:00.000000000Z"}
    assert item["updatedAt"] == {"S": "2026-05-28T00:00:00.000000000Z"}
    assert item["version"] == {"N": "0"}


def test_table_update_expected_version_sets_lifecycle_and_lock() -> None:
    model = ModelDefinition.from_dataclass(LifecycleItem, table_name="tbl")
    table: Table[LifecycleItem] = Table(
        model, client=_StubClient(), now=lambda: "2026-05-28T00:00:01.000000000Z"
    )

    req = table._build_update_request(
        "A",
        "1",
        {"value": 2},
        expected_version=0,
        return_values="ALL_NEW",
    )

    assert req["ConditionExpression"] == "#d_version = :d_expected_version"
    assert "SET #d_updated_at = :d_updated_at, #d_value = :d_value" in req["UpdateExpression"]
    assert "ADD #d_version :d_version_increment" in req["UpdateExpression"]
    assert req["ExpressionAttributeNames"]["#d_updated_at"] == "updatedAt"
    assert req["ExpressionAttributeNames"]["#d_version"] == "version"
    assert req["ExpressionAttributeValues"][":d_updated_at"] == {"S": "2026-05-28T00:00:01.000000000Z"}
    assert req["ExpressionAttributeValues"][":d_expected_version"] == {"N": "0"}
    assert req["ExpressionAttributeValues"][":d_version_increment"] == {"N": "1"}


@pytest.mark.parametrize("field_name", ["created_at", "updated_at"])
def test_table_update_rejects_lifecycle_timestamp_tampering(field_name: str) -> None:
    model = ModelDefinition.from_dataclass(LifecycleItem, table_name="tbl")
    stub = _StubClient()
    table: Table[LifecycleItem] = Table(model, client=stub, now=lambda: "2026-05-28T00:00:01.000000000Z")

    with pytest.raises(ValidationError, match="lifecycle timestamp field"):
        table.update("A", "1", {field_name: "1970-01-01T00:00:00.000000000Z"})

    assert stub.update_reqs == []


def test_table_update_stamps_updated_at_for_valid_update_without_lock() -> None:
    model = ModelDefinition.from_dataclass(LifecycleItem, table_name="tbl")
    table: Table[LifecycleItem] = Table(
        model, client=_StubClient(), now=lambda: "2026-05-28T00:00:02.000000000Z"
    )

    req = table._build_update_request("A", "1", {"value": 3}, return_values="ALL_NEW")

    assert req["UpdateExpression"] == "SET #d_updated_at = :d_updated_at, #d_value = :d_value"
    assert req["ExpressionAttributeNames"]["#d_updated_at"] == "updatedAt"
    assert req["ExpressionAttributeNames"]["#d_value"] == "value"
    assert req["ExpressionAttributeValues"][":d_updated_at"] == {"S": "2026-05-28T00:00:02.000000000Z"}
    assert req["ExpressionAttributeValues"][":d_value"] == {"N": "3"}
    assert ":d_created_at" not in req["ExpressionAttributeValues"]


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

    req = table._build_update_request("A", "1", {"note": "", "value": 1})
    assert "REMOVE #d_note" in req["UpdateExpression"]
    assert "#d_note = :d_note" not in req["UpdateExpression"]

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

    version_mapped = _map_client_error(err, version_conflict=True)
    assert isinstance(version_mapped, VersionConflictError)
    assert version_mapped.__class__.__name__ == "VersionConflictError"

    tx_err = ClientError(
        {"Error": {"Code": "TransactionCanceledException", "Message": "ConditionalCheckFailed"}},
        "TransactWriteItems",
    )
    mapped_tx = _map_transaction_error(tx_err)
    assert mapped_tx.__class__.__name__ == "ConditionFailedError"
