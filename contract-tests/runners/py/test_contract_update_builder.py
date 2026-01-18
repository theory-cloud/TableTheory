from __future__ import annotations

from dataclasses import dataclass

from theorydb_py import ModelDefinition, Table, theorydb_field
from theorydb_py.mocks import FakeDynamoDBClient


@dataclass(frozen=True)
class ReservedUpdate:
    pk: str = theorydb_field(roles=["pk"])
    sk: str = theorydb_field(roles=["sk"])
    name: str = theorydb_field()
    version: int = theorydb_field(roles=["version"])


def test_update_builder_builds_safe_expressions() -> None:
    client = FakeDynamoDBClient()

    def validate(req: dict) -> None:
        assert req["UpdateExpression"] == "SET #u_name = :u1 ADD #u_version :u2"
        assert req["ConditionExpression"] == "#c_name = :c1"
        assert req["ExpressionAttributeNames"]["#u_name"] == "name"
        assert req["ExpressionAttributeNames"]["#u_version"] == "version"
        assert req["ExpressionAttributeValues"][":u1"] == {"S": "v1"}
        assert req["ExpressionAttributeValues"][":u2"] == {"N": "1"}
        assert req["ExpressionAttributeValues"][":c1"] == {"S": "v0"}

    client.expect("update_item", validate, response={})

    model = ModelDefinition.from_dataclass(ReservedUpdate, table_name="tbl")
    table: Table[ReservedUpdate] = Table(model, client=client)

    table.update_builder("A", "B").set("name", "v1").add("version", 1).condition("name", "=", "v0").execute()
    client.assert_no_pending()
