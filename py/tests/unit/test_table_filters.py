from __future__ import annotations

from dataclasses import dataclass

import pytest

from theorydb_py import ModelDefinition, Table, ValidationError, theorydb_field
from theorydb_py.mocks import FakeDynamoDBClient
from theorydb_py.query import FilterCondition, FilterGroup


@dataclass(frozen=True)
class Record:
    pk: str = theorydb_field(roles=["pk"])
    sk: str = theorydb_field(roles=["sk"])
    status: str = theorydb_field()
    note: str = theorydb_field(default="")


def test_query_builds_filter_expression_and_groups() -> None:
    client = FakeDynamoDBClient()
    client.expect(
        "query",
        {
            "TableName": "tbl",
            "KeyConditionExpression": "#pk = :pk",
            "ExpressionAttributeNames": {
                "#pk": "pk",
                "#f_note": "note",
            },
            "ExpressionAttributeValues": {
                ":pk": {"S": "P1"},
                ":f1": {"S": "a"},
                ":f2": {"S": "b"},
            },
            "FilterExpression": "(#f_note = :f1 OR #f_note = :f2)",
            "ScanIndexForward": True,
            "ConsistentRead": False,
        },
        response={"Items": [], "LastEvaluatedKey": None},
    )

    model = ModelDefinition.from_dataclass(Record, table_name="tbl")
    table: Table[Record] = Table(model, client=client)
    page = table.query(
        "P1",
        filter=FilterGroup.or_(
            FilterCondition.eq("note", "a"),
            FilterCondition.eq("note", "b"),
        ),
    )
    assert page.items == []
    client.assert_no_pending()


def test_scan_builds_filter_expression_and_values() -> None:
    client = FakeDynamoDBClient()

    def validate(req: dict) -> None:
        assert req["FilterExpression"] == "#f_status IN (:f1, :f2)"
        assert req["ExpressionAttributeNames"]["#f_status"] == "status"
        assert req["ExpressionAttributeValues"][":f1"] == {"S": "A"}
        assert req["ExpressionAttributeValues"][":f2"] == {"S": "B"}

    client.expect(
        "scan",
        validate,
        response={"Items": [], "LastEvaluatedKey": None},
    )

    model = ModelDefinition.from_dataclass(Record, table_name="tbl")
    table: Table[Record] = Table(model, client=client)
    table.scan(filter=FilterCondition.in_("status", ["A", "B"]))
    client.assert_no_pending()


def test_filter_rejects_unknown_field() -> None:
    client = FakeDynamoDBClient()
    model = ModelDefinition.from_dataclass(Record, table_name="tbl")
    table: Table[Record] = Table(model, client=client)
    with pytest.raises(ValidationError, match="unknown field"):
        table.query("P1", filter=FilterCondition.eq("nope", "x"))


def test_filter_rejects_value_shape_errors() -> None:
    client = FakeDynamoDBClient()
    model = ModelDefinition.from_dataclass(Record, table_name="tbl")
    table: Table[Record] = Table(model, client=client)

    with pytest.raises(ValidationError, match="BETWEEN requires two values"):
        table.query("P1", filter=FilterCondition(field="note", op="between", values=("a",)))


def test_scan_supports_filter_operators_and_grouping() -> None:
    client = FakeDynamoDBClient()

    def validate(req: dict) -> None:
        assert req["FilterExpression"] == (
            "(#f_note BETWEEN :f1 AND :f2 AND #f_note <> :f3 AND #f_note < :f4 AND #f_note <= :f5 "
            "AND #f_note > :f6 AND #f_note >= :f7 AND begins_with(#f_note, :f8) AND contains(#f_note, :f9) "
            "AND attribute_exists(#f_status) AND attribute_not_exists(#f_sk))"
        )
        assert req["ExpressionAttributeNames"]["#f_note"] == "note"
        assert req["ExpressionAttributeNames"]["#f_status"] == "status"
        assert req["ExpressionAttributeNames"]["#f_sk"] == "sk"
        assert req["ExpressionAttributeValues"][":f1"] == {"S": "a"}
        assert req["ExpressionAttributeValues"][":f2"] == {"S": "z"}
        assert req["ExpressionAttributeValues"][":f9"] == {"S": "mid"}

    client.expect(
        "scan",
        validate,
        response={"Items": [], "LastEvaluatedKey": None},
    )

    model = ModelDefinition.from_dataclass(Record, table_name="tbl")
    table: Table[Record] = Table(model, client=client)
    table.scan(
        filter=FilterGroup.and_(
            FilterCondition.between("note", "a", "z"),
            FilterCondition.ne("note", "x"),
            FilterCondition.lt("note", "b"),
            FilterCondition.lte("note", "c"),
            FilterCondition.gt("note", "d"),
            FilterCondition.gte("note", "e"),
            FilterCondition.begins_with("note", "pre"),
            FilterCondition.contains("note", "mid"),
            FilterCondition.exists("status"),
            FilterCondition.not_exists("sk"),
        )
    )
    client.assert_no_pending()
