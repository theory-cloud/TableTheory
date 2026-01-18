from __future__ import annotations

from dataclasses import dataclass
from decimal import Decimal
from typing import Any, cast

import pytest
from botocore.exceptions import ClientError

from theorydb_py import (
    AwsError,
    EncryptionNotConfiguredError,
    ModelDefinition,
    SortKeyCondition,
    Table,
    TransactionCanceledError,
    ValidationError,
    theorydb_field,
)
from theorydb_py.model import AttributeDefinition, Projection, gsi
from theorydb_py.table import _coerce_value, _is_empty, _map_client_error, _map_transaction_error
from theorydb_py.transaction import (
    TransactConditionCheck,
    TransactDelete,
    TransactPut,
    TransactUpdate,
)


@dataclass(frozen=True)
class Item:
    pk: str = theorydb_field(name="PK", roles=["pk"])
    sk: str = theorydb_field(name="SK", roles=["sk"])
    value: int = theorydb_field(name="value")
    note: str = theorydb_field(name="note", default="")


class _StubClient:
    def __init__(self) -> None:
        self.query_reqs: list[dict[str, Any]] = []
        self.scan_reqs: list[dict[str, Any]] = []
        self.batch_get_reqs: list[dict[str, Any]] = []
        self.batch_write_reqs: list[dict[str, Any]] = []
        self.transact_write_reqs: list[dict[str, Any]] = []
        self.put_reqs: list[dict[str, Any]] = []
        self.delete_reqs: list[dict[str, Any]] = []
        self.update_reqs: list[dict[str, Any]] = []

        self._items: list[dict[str, Any]] = []
        self._last_key: dict[str, Any] | None = None
        self._batch_items: list[dict[str, Any]] = []
        self._update_attrs: dict[str, Any] | None = None

    def set_items(self, items: list[dict[str, Any]], *, last_key: dict[str, Any] | None = None) -> None:
        self._items = items
        self._last_key = last_key

    def set_batch_items(self, items: list[dict[str, Any]]) -> None:
        self._batch_items = items

    def set_update_attrs(self, attrs: dict[str, Any] | None) -> None:
        self._update_attrs = attrs

    def query(self, **req):  # noqa: ANN001
        self.query_reqs.append(req)
        out: dict[str, Any] = {"Items": self._items}
        if self._last_key is not None:
            out["LastEvaluatedKey"] = self._last_key
        return out

    def scan(self, **req):  # noqa: ANN001
        self.scan_reqs.append(req)
        out: dict[str, Any] = {"Items": self._items}
        if self._last_key is not None:
            out["LastEvaluatedKey"] = self._last_key
        return out

    def batch_get_item(self, *, RequestItems):  # noqa: N803
        self.batch_get_reqs.append(RequestItems)
        return {"Responses": {next(iter(RequestItems.keys())): self._batch_items}, "UnprocessedKeys": {}}

    def batch_write_item(self, *, RequestItems):  # noqa: N803
        self.batch_write_reqs.append(RequestItems)
        return {"UnprocessedItems": {}}

    def transact_write_items(self, *, TransactItems):  # noqa: N803
        self.transact_write_reqs.append({"TransactItems": TransactItems})
        return {}

    def put_item(self, **req):  # noqa: ANN001
        self.put_reqs.append(req)
        return {}

    def delete_item(self, **req):  # noqa: ANN001
        self.delete_reqs.append(req)
        return {}

    def update_item(self, **req):  # noqa: ANN001
        self.update_reqs.append(req)
        return {"Attributes": self._update_attrs} if self._update_attrs is not None else {}


def test_table_value_helpers_and_error_mapping_variants() -> None:
    assert _is_empty(None) is True
    assert _is_empty(False) is True
    assert _is_empty(0) is True
    assert _is_empty("") is True
    assert _is_empty([]) is True
    assert _is_empty("x") is False

    assert _coerce_value(None, int) is None
    assert _coerce_value(Decimal("2"), int) == 2
    assert _coerce_value(Decimal("2.5"), float) == 2.5
    assert _coerce_value({Decimal("1"), Decimal("2")}, set[int]) == {1, 2}

    mapped = _map_client_error(
        ClientError({"Error": {"Code": "ValidationException", "Message": "bad"}}, "Op")
    )
    assert isinstance(mapped, ValidationError)

    mapped = _map_client_error(
        ClientError({"Error": {"Code": "ResourceNotFoundException", "Message": "missing"}}, "Op")
    )
    assert mapped.__class__.__name__ == "NotFoundError"

    mapped = _map_client_error(ClientError({"Error": {"Code": "Nope", "Message": "x"}}, "Op"))
    assert isinstance(mapped, AwsError)

    tx_err = ClientError(
        {
            "Error": {"Code": "TransactionCanceledException", "Message": "boom"},
            "CancellationReasons": [{"Code": "TransactionConflict"}, {"Code": "None"}],
        },
        "TransactWriteItems",
    )
    mapped_tx = _map_transaction_error(tx_err)
    assert isinstance(mapped_tx, TransactionCanceledError)
    assert mapped_tx.reason_codes == ("TransactionConflict", "None")

    mapped_tx = _map_transaction_error(
        ClientError({"Error": {"Code": "ValidationException", "Message": "x"}}, "Op")
    )
    assert isinstance(mapped_tx, ValidationError)


def test_query_rejects_sort_condition_when_model_has_no_sort_key() -> None:
    @dataclass(frozen=True)
    class PKOnly:
        pk: str = theorydb_field(roles=["pk"])

    model = ModelDefinition.from_dataclass(PKOnly, table_name="tbl")
    table: Table[PKOnly] = Table(model, client=_StubClient())
    with pytest.raises(ValidationError, match="does not define a sort key"):
        table.query("A", sort=SortKeyCondition.eq("x"))


def test_query_scan_success_paths_include_index_limit_cursor_and_projection() -> None:
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

    item = Record(pk="A", sk="1", gsi_pk="G", value=1)
    last_key = table._to_key("A", "1")
    stub.set_items([table._to_item(item)], last_key=last_key)
    cursor = SortKeyCondition.begins_with("x")  # not used here; just ensure available
    del cursor

    from theorydb_py.query import encode_cursor

    start_cursor = encode_cursor(last_key)

    page = table.query(
        "G",
        index_name="gsi1",
        limit=1,
        cursor=start_cursor,
        projection=["pk", "sk", "gsi_pk", "value"],
    )
    assert page.items == [item]
    assert page.next_cursor is not None
    assert stub.query_reqs[0]["IndexName"] == "gsi1"
    assert stub.query_reqs[0]["Limit"] == 1
    assert "ExclusiveStartKey" in stub.query_reqs[0]
    assert "ProjectionExpression" in stub.query_reqs[0]

    page = table.scan(
        index_name="gsi1",
        limit=1,
        cursor=start_cursor,
        projection=["pk", "sk", "gsi_pk", "value"],
    )
    assert page.items == [item]
    assert page.next_cursor is not None
    assert stub.scan_reqs[0]["IndexName"] == "gsi1"
    assert stub.scan_reqs[0]["Limit"] == 1
    assert "ExclusiveStartKey" in stub.scan_reqs[0]
    assert "ExpressionAttributeNames" in stub.scan_reqs[0]


def test_batch_get_and_batch_write_pk_only_models_and_projection() -> None:
    @dataclass(frozen=True)
    class PKOnly:
        pk: str = theorydb_field(roles=["pk"])
        value: int = theorydb_field(default=0)

    model = ModelDefinition.from_dataclass(PKOnly, table_name="tbl")
    stub = _StubClient()
    table: Table[PKOnly] = Table(model, client=stub)

    assert table.batch_get([]) == []
    with pytest.raises(ValidationError, match="max_retries must be >= 0"):
        table.batch_get(["A"], max_retries=-1)

    with pytest.raises(ValidationError, match="expected key tuple"):
        table.batch_get([("A",)])  # type: ignore[list-item]
    with pytest.raises(ValidationError, match="sk must be None"):
        table.batch_get([("A", "1")])  # type: ignore[list-item]

    items = [PKOnly(pk="A", value=1), PKOnly(pk="B", value=2)]
    stub.set_batch_items([table._to_item(i) for i in items])
    got = table.batch_get(["A", ("B", None)], projection=["pk"])
    assert set(got) == set(items)

    with pytest.raises(ValidationError, match="model does not define sk"):
        table._to_key("A", "1")  # type: ignore[arg-type]

    table.batch_write(
        puts=[PKOnly(pk="C", value=3)],
        deletes=["A", ("B", None)],
    )
    requests = stub.batch_write_reqs[0]["tbl"]
    assert any("PutRequest" in r for r in requests)
    assert any("DeleteRequest" in r for r in requests)


def test_batch_get_rejects_invalid_key_shape_for_sk_models() -> None:
    model = ModelDefinition.from_dataclass(Item, table_name="tbl")
    table: Table[Item] = Table(model, client=_StubClient())
    with pytest.raises(ValidationError, match="expected key tuple"):
        table.batch_get(["A"])  # type: ignore[list-item]


def test_transact_write_builds_requests_for_all_action_types() -> None:
    model = ModelDefinition.from_dataclass(Item, table_name="tbl")
    stub = _StubClient()
    table: Table[Item] = Table(model, client=stub)

    with pytest.raises(ValidationError, match="actions is required"):
        table.transact_write([])

    action = TransactPut(item=Item(pk="A", sk="1", value=1))
    with pytest.raises(ValidationError, match="at most 100"):
        table.transact_write([action] * 101)

    with pytest.raises(ValidationError, match="unsupported transaction action"):
        table.transact_write([cast(Any, object())])

    stub.set_update_attrs(table._to_item(Item(pk="A", sk="1", value=2)))

    actions = [
        TransactPut(
            item=Item(pk="A", sk="1", value=1),
            condition_expression="attribute_not_exists(PK)",
            expression_attribute_names={"#n": "note"},
            expression_attribute_values={":v": "x"},
        ),
        TransactDelete(
            pk="A",
            sk="1",
            condition_expression="attribute_exists(PK)",
            expression_attribute_names={"#n": "note"},
            expression_attribute_values={":v": "x"},
        ),
        TransactUpdate(
            pk="A",
            sk="1",
            updates={"value": 2},
            condition_expression="attribute_exists(PK)",
            expression_attribute_names={"#x": "X"},
            expression_attribute_values={":x": "x"},
        ),
        TransactConditionCheck(
            pk="A",
            sk="1",
            condition_expression="attribute_exists(PK)",
            expression_attribute_names={"#n": "note"},
            expression_attribute_values={":v": "x"},
        ),
    ]

    table.transact_write(actions)
    assert len(stub.transact_write_reqs) == 1
    transact_items = stub.transact_write_reqs[0]["TransactItems"]
    assert {next(iter(i.keys())) for i in transact_items} == {"Put", "Delete", "Update", "ConditionCheck"}


def test_put_delete_update_expression_attribute_maps_and_build_request_merges() -> None:
    model = ModelDefinition.from_dataclass(Item, table_name="tbl")
    stub = _StubClient()
    table: Table[Item] = Table(model, client=stub)

    table.put(
        Item(pk="A", sk="1", value=1),
        condition_expression="attribute_not_exists(PK)",
        expression_attribute_names={"#n": "note"},
        expression_attribute_values={":v": "x"},
    )
    assert "ExpressionAttributeNames" in stub.put_reqs[0]
    assert stub.put_reqs[0]["ExpressionAttributeValues"][":v"]["S"] == "x"

    table.delete(
        "A",
        "1",
        condition_expression="attribute_exists(PK)",
        expression_attribute_names={"#n": "note"},
        expression_attribute_values={":v": "x"},
    )
    assert "ExpressionAttributeNames" in stub.delete_reqs[0]
    assert stub.delete_reqs[0]["ExpressionAttributeValues"][":v"]["S"] == "x"

    stub.set_update_attrs(table._to_item(Item(pk="A", sk="1", value=2)))
    got = table.update(
        "A",
        "1",
        {"value": 2},
        condition_expression="attribute_exists(PK)",
        expression_attribute_names={"#x": "X"},
        expression_attribute_values={":x": "x"},
    )
    assert got.value == 2
    assert stub.update_reqs[0]["ExpressionAttributeNames"]["#x"] == "X"
    assert stub.update_reqs[0]["ExpressionAttributeValues"][":x"]["S"] == "x"


def test_internal_helpers_cover_projection_and_serialization_error_paths() -> None:
    @dataclass(frozen=True)
    class Minimal:
        pk: str = theorydb_field(roles=["pk"])
        sk: str = theorydb_field(roles=["sk"])
        optional: str = theorydb_field(default="")
        ignored: str = theorydb_field(ignore=True, default="x")

    model = ModelDefinition.from_dataclass(Minimal, table_name="tbl")
    table: Table[Minimal] = Table(model, client=_StubClient())

    with pytest.raises(ValidationError, match="unknown index"):
        table._resolve_index("missing")

    with pytest.raises(ValidationError, match="unknown field"):
        table._projection_expression(["pk", "sk", "unknown"], {})

    values: dict[str, Any] = {}
    expr = table._apply_sort_condition("#pk = :pk", SortKeyCondition.eq("1"), values)
    assert expr.endswith(":sk")

    values = {}
    expr = table._apply_sort_condition("#pk = :pk", SortKeyCondition.between("1", "2"), values)
    assert ":sk1" in values and ":sk2" in values

    values = {}
    expr = table._apply_sort_condition("#pk = :pk", SortKeyCondition.begins_with("1"), values)
    assert ":sk" in values

    with pytest.raises(ValidationError, match="invalid sort key condition"):
        table._apply_sort_condition("#pk = :pk", SortKeyCondition(op="=", values=("1", "2")), {})

    with pytest.raises(ValidationError, match="unsupported sort key operator"):
        table._apply_sort_condition("#pk = :pk", SortKeyCondition(op="contains", values=("1",)), {})

    attr_def = AttributeDefinition(
        python_name="payload",
        attribute_name="payload",
        roles=(),
        omitempty=False,
        set=False,
        json=True,
        binary=False,
        encrypted=False,
    )
    av = table._serialize_attr_value(attr_def, {"b": 2, "a": 1})
    assert av["S"] == '{"a":1,"b":2}'

    empty_set_attr = AttributeDefinition(
        python_name="tags",
        attribute_name="tags",
        roles=(),
        omitempty=False,
        set=True,
        json=False,
        binary=False,
        encrypted=False,
    )
    assert table._serialize_attr_value(empty_set_attr, set()) == {"NULL": True}

    encrypted_attr = AttributeDefinition(
        python_name="secret",
        attribute_name="secret",
        roles=(),
        omitempty=False,
        set=False,
        json=False,
        binary=False,
        encrypted=True,
    )
    with pytest.raises(EncryptionNotConfiguredError):
        table._serialize_attr_value(encrypted_attr, "x")


def test_to_item_and_from_item_error_paths_and_json_unmarshal() -> None:
    @dataclass(frozen=True)
    class MissingPK:
        pk: str = theorydb_field(roles=["pk"], omitempty=True, default="")
        value: int = theorydb_field(default=0)

    model = ModelDefinition.from_dataclass(MissingPK, table_name="tbl")
    table: Table[MissingPK] = Table(model, client=_StubClient())
    with pytest.raises(ValidationError, match="missing pk"):
        table._to_item(MissingPK())

    @dataclass(frozen=True)
    class MissingSK:
        pk: str = theorydb_field(roles=["pk"])
        sk: str = theorydb_field(roles=["sk"], omitempty=True, default="")

    model = ModelDefinition.from_dataclass(MissingSK, table_name="tbl")
    table = Table(model, client=_StubClient())
    with pytest.raises(ValidationError, match="missing sk"):
        table._to_item(MissingSK(pk="A"))

    with pytest.raises(ValidationError, match="dataclass instance"):
        table._to_item(cast(Any, object()))

    @dataclass(frozen=True)
    class WithIgnoredRequired:
        pk: str = theorydb_field(roles=["pk"])
        ignored: str = theorydb_field(ignore=True)

    model = ModelDefinition.from_dataclass(WithIgnoredRequired, table_name="tbl")
    table = Table(model, client=_StubClient())
    with pytest.raises(ValidationError):
        table._from_item({"pk": {"S": "A"}})

    @dataclass(frozen=True)
    class WithJSON:
        pk: str = theorydb_field(roles=["pk"])
        sk: str = theorydb_field(roles=["sk"])
        payload: dict[str, int] = theorydb_field(json=True)

    model = ModelDefinition.from_dataclass(WithJSON, table_name="tbl")
    table = Table(model, client=_StubClient())
    got = table._from_item({"pk": {"S": "A"}, "sk": {"S": "1"}, "payload": {"S": '{"a":1}'}})
    assert got.payload == {"a": 1}


def test_from_item_encrypted_envelope_must_be_map() -> None:
    @dataclass(frozen=True)
    class Encrypted:
        pk: str = theorydb_field(roles=["pk"])
        sk: str = theorydb_field(roles=["sk"])
        secret: str = theorydb_field(encrypted=True, default="")

    model = ModelDefinition.from_dataclass(Encrypted, table_name="tbl")
    table: Table[Encrypted] = Table(
        model, client=_StubClient(), kms_key_arn="arn:aws:kms:us-east-1:1:key/x", kms_client=object()
    )

    with pytest.raises(ValidationError, match="encrypted envelope must be a map"):
        table._from_item({"pk": {"S": "A"}, "sk": {"S": "1"}, "secret": {"S": "nope"}})
