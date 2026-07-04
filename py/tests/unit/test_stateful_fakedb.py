from __future__ import annotations

from dataclasses import dataclass

import pytest
from botocore.exceptions import ClientError

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
from theorydb_py import fakedb as fakedb_module
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


def _av_s(value: str) -> dict[str, str]:
    return {"S": value}


def _av_n(value: str) -> dict[str, str]:
    return {"N": value}


def _key(pk: str, sk: str) -> dict[str, dict[str, str]]:
    return {"PK": _av_s(pk), "SK": _av_s(sk)}


def _item(pk: str, sk: str, name: str, score: str) -> dict[str, object]:
    return {
        **_key(pk, sk),
        "name": _av_s(name),
        "score": _av_n(score),
        "tag": _av_s(name[:1]),
        "ttl": _av_n("1700000000"),
    }


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


def test_stateful_dynamodb_client_admin_batches_scans_and_transactions() -> None:
    client = StatefulDynamoDBClient()
    table_name = "notes_direct"

    client.create_table(
        TableName=table_name,
        KeySchema=[
            {"AttributeName": "PK", "KeyType": "HASH"},
            {"AttributeName": "SK", "KeyType": "RANGE"},
        ],
    )
    with pytest.raises(ClientError, match="ResourceInUseException"):
        client.create_table(TableName=table_name)

    ttl = client.update_time_to_live(
        TableName=table_name,
        TimeToLiveSpecification={"AttributeName": "ttl", "Enabled": True},
    )
    assert ttl["TimeToLiveSpecification"]["AttributeName"] == "ttl"

    client.seed(
        table_name,
        _item("USER#1", "A", "one", "10"),
        _item("USER#1", "B", "two", "20"),
        _item("USER#2", "A", "three", "30"),
    )
    assert len(client.items(table_name)) == 3
    assert client.describe_table(TableName=table_name)["Table"]["ItemCount"] == 3

    projected = client.get_item(
        TableName=table_name,
        Key=_key("USER#1", "A"),
        ProjectionExpression="PK, #n",
        ExpressionAttributeNames={"#n": "name"},
    )
    assert projected["Item"]["name"] == _av_s("one")
    assert "score" not in projected["Item"]
    assert client.get_item(TableName=table_name, Key=_key("missing", "A")) == {}

    scanned = client.scan(
        TableName=table_name,
        FilterExpression="(#score >= :min AND begins_with(#name, :prefix))",
        ExpressionAttributeNames={"#name": "name", "#score": "score"},
        ExpressionAttributeValues={":min": _av_n("10"), ":prefix": _av_s("t")},
        Limit=1,
    )
    assert len(scanned["Items"]) == 1
    assert scanned["LastEvaluatedKey"]

    counted = client.scan(TableName=table_name, Select="COUNT")
    assert counted["Count"] == 3
    assert "Items" not in counted

    query_page = client.query(
        TableName=table_name,
        KeyConditionExpression="#pk = :pk AND #sk >= :sk",
        ExpressionAttributeNames={"#pk": "PK", "#sk": "SK"},
        ExpressionAttributeValues={":pk": _av_s("USER#1"), ":sk": _av_s("A")},
        ScanIndexForward=False,
        Limit=1,
    )
    assert query_page["Items"][0]["SK"] == _av_s("B")

    next_page = client.query(
        TableName=table_name,
        KeyConditionExpression="#pk = :pk AND #sk >= :sk",
        ExpressionAttributeNames={"#pk": "PK", "#sk": "SK"},
        ExpressionAttributeValues={":pk": _av_s("USER#1"), ":sk": _av_s("A")},
        ExclusiveStartKey=query_page["LastEvaluatedKey"],
        ScanIndexForward=False,
    )
    assert next_page["Items"][0]["SK"] == _av_s("A")

    client.batch_write_item(
        RequestItems={
            table_name: [
                {"PutRequest": {"Item": _item("USER#1", "C", "four", "40")}},
                {"DeleteRequest": {"Key": _key("USER#1", "B")}},
            ]
        }
    )
    batch = client.batch_get_item(
        RequestItems={table_name: {"Keys": [_key("USER#1", "A"), _key("USER#1", "C")]}}
    )
    assert len(batch["Responses"][table_name]) == 2

    updated = client.update_item(
        TableName=table_name,
        Key=_key("USER#1", "C"),
        UpdateExpression="SET #note = :note REMOVE #tag ADD #score :inc",
        ExpressionAttributeNames={"#note": "note", "#score": "score", "#tag": "tag"},
        ExpressionAttributeValues={":inc": _av_n("2"), ":note": _av_s("seeded")},
        ReturnValues="ALL_NEW",
    )
    assert updated["Attributes"]["score"] == _av_n("42")
    assert updated["Attributes"]["note"] == _av_s("seeded")
    assert "tag" not in updated["Attributes"]

    transact_read = client.transact_get_items(
        TransactItems=[
            {"Get": {"TableName": table_name, "Key": _key("USER#1", "C")}},
            {"Get": {"TableName": table_name, "Key": _key("missing", "A")}},
        ]
    )
    assert "Item" in transact_read["Responses"][0]
    assert transact_read["Responses"][1] == {}

    client.transact_write_items(
        TransactItems=[
            {"Put": {"TableName": table_name, "Item": _item("TX#1", "A", "tx", "1")}},
            {
                "ConditionCheck": {
                    "TableName": table_name,
                    "Key": _key("USER#1", "C"),
                    "ConditionExpression": "attribute_exists(PK)",
                }
            },
            {
                "Update": {
                    "TableName": table_name,
                    "Key": _key("TX#1", "A"),
                    "UpdateExpression": "ADD #score :inc",
                    "ExpressionAttributeNames": {"#score": "score"},
                    "ExpressionAttributeValues": {":inc": _av_n("1")},
                }
            },
            {"Delete": {"TableName": table_name, "Key": _key("USER#2", "A")}},
        ]
    )
    with pytest.raises(ClientError, match="TransactionCanceledException"):
        client.transact_write_items(
            TransactItems=[
                {
                    "ConditionCheck": {
                        "TableName": table_name,
                        "Key": _key("USER#1", "C"),
                        "ConditionExpression": "attribute_not_exists(PK)",
                    }
                }
            ]
        )

    client.delete_item(
        TableName=table_name,
        Key=_key("USER#1", "A"),
        ConditionExpression="attribute_exists(PK)",
    )
    client.delete_table(TableName=table_name)
    with pytest.raises(ClientError, match="ResourceNotFoundException"):
        client.describe_table(TableName=table_name)
    client.reset()
    assert client.items(table_name) == []


def test_stateful_fakedb_expression_helpers_cover_edge_branches() -> None:
    item = {
        "PK": _av_s("USER#1"),
        "SK": _av_s("A"),
        "blob": {"B": b"abc"},
        "flag": {"BOOL": True},
        "name": _av_s("one"),
        "score": _av_n("7"),
    }
    names = {"#missing": "missing", "#name": "name", "#score": "score"}
    values = {
        ":max": _av_n("9"),
        ":min": _av_n("1"),
        ":not_name": _av_s("two"),
        ":score": _av_n("7"),
        ":value": _av_s("one"),
    }

    for expr in [
        "(#score >= :score AND (#name = :value OR #name = :not_name))",
        "#score <> :min",
        "#score < :max",
        "#score <= :score",
        "#score > :min",
        "attribute_exists(#name)",
        "attribute_not_exists(#missing)",
        "begins_with(#name, :value)",
    ]:
        assert fakedb_module._matches(expr, item, names, values)

    assert not fakedb_module._matches("contains(#name, :value)", item, names, values)
    assert fakedb_module._compare_av({"B": b"abc"}, {"B": b"abc"}) == 0
    assert fakedb_module._compare_av({"BOOL": True}, {"BOOL": True}) == 0
    assert fakedb_module._add_av(None, _av_s("fallback")) == _av_s("fallback")
    assert fakedb_module._strip_parens("((#name = :value))") == "#name = :value"
    assert fakedb_module._table_description("pk_only", fakedb_module._TableState(sk=None))["KeySchema"] == [
        {"AttributeName": "PK", "KeyType": "HASH"}
    ]
