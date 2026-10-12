from __future__ import annotations

from dataclasses import dataclass
from typing import Any

from tabletheory_py import ModelDefinition, Table, gsi, theorydb_field
from tabletheory_py.mocks import FakeDynamoDBClient


@dataclass(frozen=True)
class User:
    pk: str = theorydb_field(name="PK", roles=["pk"])
    sk: str = theorydb_field(name="SK", roles=["sk"])
    gsi1pk: str = theorydb_field(name="GSI1PK", roles=["gsi1pk"])
    gsi1sk: str = theorydb_field(name="GSI1SK", roles=["gsi1sk"])
    email: str = theorydb_field()


def main() -> None:
    model = ModelDefinition.from_dataclass(
        User,
        table_name="users_contract",
        indexes=[gsi("gsi_email", partition="gsi1pk", sort="gsi1sk")],
    )

    client = FakeDynamoDBClient()
    response: dict[str, Any] = {
        "Items": [
            {
                "PK": {"S": "USER#ada"},
                "SK": {"S": "PROFILE"},
                "GSI1PK": {"S": "EMAIL#ada@example.com"},
                "GSI1SK": {"S": "ada@example.com"},
                "email": {"S": "ada@example.com"},
            }
        ]
    }
    client.expect(
        "query",
        {
            "TableName": "users_contract",
            "IndexName": "gsi_email",
            "KeyConditionExpression": "#pk = :pk",
            "ExpressionAttributeNames": {"#pk": "GSI1PK"},
        },
        response=response,
    )

    table = Table(model, client=client)
    page = table.query("EMAIL#ada@example.com", index_name="gsi_email", limit=25)
    client.assert_no_pending()

    print(f"gsi_items={len(page.items)} email={page.items[0].email}")


if __name__ == "__main__":
    main()
