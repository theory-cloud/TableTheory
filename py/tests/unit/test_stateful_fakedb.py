from __future__ import annotations

from dataclasses import dataclass

import pytest

from tabletheory_py import StatefulDynamoDBClient as AliasStatefulDynamoDBClient
from theorydb_py import (
    FilterCondition,
    ModelDefinition,
    SortKeyCondition,
    StatefulDynamoDBClient,
    Table,
    VersionConflictError,
    theorydb_field,
)
from theorydb_py.testkit import StatefulDynamoDBClient as KitStatefulDynamoDBClient


@dataclass(frozen=True)
class StatefulNote:
    pk: str = theorydb_field(name="PK", roles=["pk"])
    sk: str = theorydb_field(name="SK", roles=["sk"])
    email: str = theorydb_field(name="email")
    name: str = theorydb_field(name="name")
    created_at: str = theorydb_field(name="createdAt", roles=["created_at"], default="")
    updated_at: str = theorydb_field(name="updatedAt", roles=["updated_at"], default="")
    version: int | None = theorydb_field(name="version", roles=["version"], default=None)
    ttl: int = theorydb_field(name="ttl", roles=["ttl"], default=0)


def _table(client: StatefulDynamoDBClient) -> Table[StatefulNote]:
    model = ModelDefinition.from_dataclass(StatefulNote, table_name="notes_stateful")
    return Table(model, client=client, now=lambda: "2026-07-04T00:00:00.000000000Z")


def test_stateful_dynamodb_client_write_query_update_and_version_conflict() -> None:
    client = StatefulDynamoDBClient()
    table = _table(client)

    table.put(
        StatefulNote(
            pk="USER#1",
            sk="PROFILE",
            email="test@example.com",
            name="one",
            ttl=1_700_000_000,
        )
    )

    page = table.query(
        "USER#1",
        sort=SortKeyCondition.begins_with("PRO"),
        filter=FilterCondition.eq("email", "test@example.com"),
    )

    assert len(page.items) == 1
    assert page.items[0].created_at == "2026-07-04T00:00:00.000000000Z"
    assert page.items[0].updated_at == "2026-07-04T00:00:00.000000000Z"
    assert page.items[0].version == 0
    assert client.items("notes_stateful")[0]["ttl"] == {"N": "1700000000"}

    updated = table.update("USER#1", "PROFILE", {"name": "two"}, expected_version=0)
    assert updated.name == "two"
    assert updated.version == 1

    with pytest.raises(VersionConflictError):
        table.update("USER#1", "PROFILE", {"name": "stale"}, expected_version=0)

    assert table.get("USER#1", "PROFILE").name == "two"


def test_stateful_dynamodb_client_is_exported_from_testkit_and_top_level_aliases() -> None:
    assert KitStatefulDynamoDBClient is StatefulDynamoDBClient
    assert AliasStatefulDynamoDBClient is StatefulDynamoDBClient
