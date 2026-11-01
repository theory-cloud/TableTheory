from __future__ import annotations

import os
from dataclasses import dataclass

import boto3

from tabletheory_py import ModelDefinition, Table, gsi, theorydb_field

TABLE_NAME = "tabletheory_py_gsi_query"


@dataclass(frozen=True)
class User:
    pk: str = theorydb_field(name="PK", roles=["pk"])
    sk: str = theorydb_field(name="SK", roles=["sk"])
    gsi1pk: str = theorydb_field(name="GSI1PK", roles=["gsi1pk"])
    gsi1sk: str = theorydb_field(name="GSI1SK", roles=["gsi1sk"])
    email: str = theorydb_field()


def _client():
    return boto3.client(
        "dynamodb",
        endpoint_url=os.environ.get("DYNAMODB_ENDPOINT", "http://localhost:8000"),
        region_name=os.environ.get("AWS_REGION", "us-east-1"),
        aws_access_key_id=os.environ.get("AWS_ACCESS_KEY_ID", "dummy"),
        aws_secret_access_key=os.environ.get("AWS_SECRET_ACCESS_KEY", "dummy"),
    )


def main() -> None:
    client = _client()
    client.create_table(
        TableName=TABLE_NAME,
        AttributeDefinitions=[
            {"AttributeName": "PK", "AttributeType": "S"},
            {"AttributeName": "SK", "AttributeType": "S"},
            {"AttributeName": "GSI1PK", "AttributeType": "S"},
            {"AttributeName": "GSI1SK", "AttributeType": "S"},
        ],
        KeySchema=[
            {"AttributeName": "PK", "KeyType": "HASH"},
            {"AttributeName": "SK", "KeyType": "RANGE"},
        ],
        GlobalSecondaryIndexes=[
            {
                "IndexName": "gsi_email",
                "KeySchema": [
                    {"AttributeName": "GSI1PK", "KeyType": "HASH"},
                    {"AttributeName": "GSI1SK", "KeyType": "RANGE"},
                ],
                "Projection": {"ProjectionType": "ALL"},
            }
        ],
        BillingMode="PAY_PER_REQUEST",
    )
    client.get_waiter("table_exists").wait(TableName=TABLE_NAME)

    try:
        model = ModelDefinition.from_dataclass(
            User,
            table_name=TABLE_NAME,
            indexes=[gsi("gsi_email", partition="gsi1pk", sort="gsi1sk")],
        )
        table = Table(model, client=client)

        table.put(
            User(
                pk="USER#ada",
                sk="PROFILE",
                gsi1pk="EMAIL#ada@example.com",
                gsi1sk="ada@example.com",
                email="ada@example.com",
            )
        )
        table.put(
            User(
                pk="USER#grace",
                sk="PROFILE",
                gsi1pk="EMAIL#grace@example.com",
                gsi1sk="grace@example.com",
                email="grace@example.com",
            )
        )

        page = table.query("EMAIL#ada@example.com", index_name="gsi_email", limit=25)
        if len(page.items) != 1 or page.items[0].email != "ada@example.com":
            raise AssertionError(
                f"gsi_email query returned {len(page.items)} items: {page.items!r}; "
                "want exactly ada@example.com"
            )

        print(f"gsi_items={len(page.items)} email={page.items[0].email}")
    finally:
        client.delete_table(TableName=TABLE_NAME)


if __name__ == "__main__":
    main()
