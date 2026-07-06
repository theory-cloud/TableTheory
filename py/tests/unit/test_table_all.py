from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass
from typing import Any

from tabletheory_py.mocks import FakeDynamoDBClient
from tabletheory_py.model import ModelDefinition, theorydb_field
from tabletheory_py.table import Table


@dataclass
class User:
    pk: str = theorydb_field(name="PK", roles=["pk"])
    sk: str = theorydb_field(name="SK", roles=["sk"])
    version: int = theorydb_field(name="version")


def test_query_all_paginates_until_cursor_exhausted() -> None:
    client = FakeDynamoDBClient()
    model = ModelDefinition.from_dataclass(User, table_name="users")
    table: Table[User] = Table(model, client=client)

    last = {"PK": {"S": "A"}, "SK": {"S": "1"}}

    def first(req: Mapping[str, Any]) -> None:
        assert req["TableName"] == "users"
        assert "ExclusiveStartKey" not in req

    def second(req: Mapping[str, Any]) -> None:
        assert req["TableName"] == "users"
        assert req["ExclusiveStartKey"] == last

    client.expect(
        "query",
        expected=first,
        response={
            "Items": [{"PK": {"S": "A"}, "SK": {"S": "1"}, "version": {"N": "1"}}],
            "LastEvaluatedKey": last,
        },
    )
    client.expect(
        "query",
        expected=second,
        response={"Items": [{"PK": {"S": "A"}, "SK": {"S": "2"}, "version": {"N": "2"}}]},
    )

    items = table.query_all("A")
    assert [i.version for i in items] == [1, 2]
    client.assert_no_pending()


def test_scan_all_paginates_until_cursor_exhausted() -> None:
    client = FakeDynamoDBClient()
    model = ModelDefinition.from_dataclass(User, table_name="users")
    table: Table[User] = Table(model, client=client)

    last = {"PK": {"S": "A"}, "SK": {"S": "1"}}

    def first(req: Mapping[str, Any]) -> None:
        assert req["TableName"] == "users"
        assert "ExclusiveStartKey" not in req

    def second(req: Mapping[str, Any]) -> None:
        assert req["TableName"] == "users"
        assert req["ExclusiveStartKey"] == last

    client.expect(
        "scan",
        expected=first,
        response={
            "Items": [{"PK": {"S": "A"}, "SK": {"S": "1"}, "version": {"N": "1"}}],
            "LastEvaluatedKey": last,
        },
    )
    client.expect(
        "scan",
        expected=second,
        response={"Items": [{"PK": {"S": "A"}, "SK": {"S": "2"}, "version": {"N": "2"}}]},
    )

    items = table.scan_all()
    assert [i.version for i in items] == [1, 2]
    client.assert_no_pending()


def test_query_iter_fetches_next_page_lazily() -> None:
    client = FakeDynamoDBClient()
    model = ModelDefinition.from_dataclass(User, table_name="users")
    table: Table[User] = Table(model, client=client)

    last = {"PK": {"S": "A"}, "SK": {"S": "1"}}
    client.expect(
        "query",
        response={
            "Items": [{"PK": {"S": "A"}, "SK": {"S": "1"}, "version": {"N": "1"}}],
            "LastEvaluatedKey": last,
        },
    )
    client.expect(
        "query",
        response={"Items": [{"PK": {"S": "A"}, "SK": {"S": "2"}, "version": {"N": "2"}}]},
    )

    iterator = table.query_iter("A")
    page = next(iterator)
    assert [item.version for item in page.items] == [1]
    iterator.close()

    assert len(client._expected) == 1  # noqa: SLF001 - asserts lazy early-stop behavior


def test_scan_iter_fetches_next_page_lazily() -> None:
    client = FakeDynamoDBClient()
    model = ModelDefinition.from_dataclass(User, table_name="users")
    table: Table[User] = Table(model, client=client)

    last = {"PK": {"S": "A"}, "SK": {"S": "1"}}
    client.expect(
        "scan",
        response={
            "Items": [{"PK": {"S": "A"}, "SK": {"S": "1"}, "version": {"N": "1"}}],
            "LastEvaluatedKey": last,
        },
    )
    client.expect(
        "scan",
        response={"Items": [{"PK": {"S": "A"}, "SK": {"S": "2"}, "version": {"N": "2"}}]},
    )

    iterator = table.scan_iter()
    page = next(iterator)
    assert [item.version for item in page.items] == [1]
    iterator.close()

    assert len(client._expected) == 1  # noqa: SLF001 - asserts lazy early-stop behavior
