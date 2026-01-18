from __future__ import annotations

from dataclasses import dataclass

import pytest

from theorydb_py import ModelDefinition, Table, ValidationError, theorydb_field
from theorydb_py.mocks import FakeDynamoDBClient, FakeKmsClient


@dataclass(frozen=True)
class Item:
    pk: str = theorydb_field(roles=["pk"])
    sk: str = theorydb_field(roles=["sk"])
    name: str = theorydb_field()
    version: int = theorydb_field(roles=["version"])
    nickname: str = theorydb_field(default="")
    tags: set[str] = theorydb_field(set_=True, default_factory=set)
    items: list[object] = theorydb_field(default_factory=list)


def test_update_builder_builds_update_and_condition_expressions() -> None:
    client = FakeDynamoDBClient()

    def validate(req: dict) -> None:
        assert req["UpdateExpression"] == (
            "SET #u_name = :u1, #u_nickname = if_not_exists(#u_nickname, :u2), "
            "#u_items = list_append(#u_items, :u4) REMOVE #u_items[1] "
            "ADD #u_version :u3 DELETE #u_tags :u5"
        )
        assert req["ConditionExpression"] == (
            "#c_name = :c1 OR #c_version > :c2 AND attribute_exists(#c_name) AND attribute_not_exists(#c_nickname) "
            "AND #c_version = :c3"
        )
        assert req["ExpressionAttributeNames"]["#u_name"] == "name"
        assert req["ExpressionAttributeNames"]["#u_nickname"] == "nickname"
        assert req["ExpressionAttributeNames"]["#u_items"] == "items"
        assert req["ExpressionAttributeNames"]["#u_version"] == "version"
        assert req["ExpressionAttributeNames"]["#u_tags"] == "tags"
        assert req["ExpressionAttributeNames"]["#c_name"] == "name"
        assert req["ExpressionAttributeNames"]["#c_version"] == "version"
        assert req["ExpressionAttributeNames"]["#c_nickname"] == "nickname"
        assert req["ExpressionAttributeValues"][":u1"] == {"S": "v1"}
        assert req["ExpressionAttributeValues"][":u3"] == {"N": "1"}
        assert req["ExpressionAttributeValues"][":u5"] == {"SS": ["a"]}

    client.expect(
        "update_item",
        validate,
        response={
            "Attributes": {
                "pk": {"S": "A"},
                "sk": {"S": "B"},
                "name": {"S": "v1"},
                "version": {"N": "1"},
            }
        },
    )

    model = ModelDefinition.from_dataclass(Item, table_name="tbl")
    table: Table[Item] = Table(model, client=client)
    out = (
        table.update_builder("A", "B")
        .set("name", "v1")
        .set_if_not_exists("nickname", None, "default")
        .add("version", 1)
        .append_to_list("items", ["x"])
        .remove_from_list_at("items", 1)
        .delete("tags", {"a"})
        .condition("name", "=", "v0")
        .or_condition("version", ">", 0)
        .condition_exists("name")
        .condition_not_exists("nickname")
        .condition_version(7)
        .return_values("ALL_NEW")
        .execute()
    )

    assert out is not None
    assert out.name == "v1"
    assert out.version == 1
    client.assert_no_pending()


def test_update_builder_rejects_encrypted_fields_in_conditions() -> None:
    @dataclass(frozen=True)
    class Secret:
        pk: str = theorydb_field(roles=["pk"])
        sk: str = theorydb_field(roles=["sk"])
        secret: str = theorydb_field(encrypted=True)

    kms = FakeKmsClient(plaintext_key=b"\x01" * 32, ciphertext_blob=b"x")
    ddb = FakeDynamoDBClient()
    model = ModelDefinition.from_dataclass(Secret, table_name="tbl")
    table: Table[Secret] = Table(
        model,
        client=ddb,
        kms_key_arn="arn:aws:kms:us-east-1:111111111111:key/test",
        kms_client=kms,
        rand_bytes=lambda n: b"\x02" * n,
    )

    with pytest.raises(ValidationError, match="encrypted fields cannot be used in conditions"):
        table.update_builder("A", "B").set("secret", "x").condition("secret", "=", "y").execute()


def test_update_builder_rejects_invalid_updates_and_conditions() -> None:
    client = FakeDynamoDBClient()
    model = ModelDefinition.from_dataclass(Item, table_name="tbl")
    table: Table[Item] = Table(model, client=client)

    with pytest.raises(ValidationError, match="unknown field: nope"):
        table.update_builder("A", "B").set("nope", "x").execute()

    with pytest.raises(ValidationError, match="cannot update key field: pk"):
        table.update_builder("A", "B").set("pk", "x").execute()

    with pytest.raises(ValidationError, match="ADD requires a numeric value"):
        table.update_builder("A", "B").add("version", "x").execute()

    with pytest.raises(ValidationError, match="DELETE requires a set field"):
        table.update_builder("A", "B").delete("name", {"x"}).execute()

    with pytest.raises(ValidationError, match="list index must be a non-negative integer"):
        table.update_builder("A", "B").remove_from_list_at("items", -1).execute()

    with pytest.raises(ValidationError, match="list index must be a non-negative integer"):
        table.update_builder("A", "B").set_list_element("items", -1, "x").execute()

    with pytest.raises(ValidationError, match="IN requires a sequence of values"):
        table.update_builder("A", "B").set("name", "x").condition("name", "IN", "abc").execute()

    with pytest.raises(ValidationError, match="IN supports maximum 100 values"):
        table.update_builder("A", "B").set("name", "x").condition("name", "IN", list(range(101))).execute()

    with pytest.raises(ValidationError, match="BETWEEN requires two values"):
        table.update_builder("A", "B").set("name", "x").condition("version", "BETWEEN", [1]).execute()

    with pytest.raises(ValidationError, match="EXISTS does not take a value"):
        table.update_builder("A", "B").set("name", "x").condition("name", "EXISTS", "x").execute()

    with pytest.raises(ValidationError, match="= requires one value"):
        table.update_builder("A", "B").set("name", "x").condition("name", "=").execute()


def test_update_builder_execute_returns_none_when_no_attributes() -> None:
    client = FakeDynamoDBClient()
    client.expect("update_item", response={})

    model = ModelDefinition.from_dataclass(Item, table_name="tbl")
    table: Table[Item] = Table(model, client=client)

    out = table.update_builder("A", "B").set("name", "x").execute()
    assert out is None
    client.assert_no_pending()
