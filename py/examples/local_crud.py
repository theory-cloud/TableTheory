from __future__ import annotations

import os
import uuid
from dataclasses import dataclass

import boto3

from theorydb_py import ModelDefinition, SortKeyCondition, Table, theorydb_field


@dataclass(frozen=True)
class Note:
    pk: str = theorydb_field(roles=["pk"])
    sk: str = theorydb_field(roles=["sk"])
    value: int = theorydb_field()


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
    table_name = f"theorydb_py_example_{uuid.uuid4().hex[:12]}"

    client.create_table(
        TableName=table_name,
        KeySchema=[{"AttributeName": "pk", "KeyType": "HASH"}, {"AttributeName": "sk", "KeyType": "RANGE"}],
        AttributeDefinitions=[
            {"AttributeName": "pk", "AttributeType": "S"},
            {"AttributeName": "sk", "AttributeType": "S"},
        ],
        BillingMode="PAY_PER_REQUEST",
    )
    client.get_waiter("table_exists").wait(TableName=table_name)

    try:
        model = ModelDefinition.from_dataclass(Note, table_name=table_name)
        table = Table(model, client=client)

        table.put(Note(pk="A", sk="001", value=1))
        table.put(Note(pk="A", sk="010", value=10))
        table.put(Note(pk="A", sk="100", value=100))

        print("get:", table.get("A", "010"))

        page = table.query("A", sort=SortKeyCondition.begins_with("0"))
        print("query begins_with('0'):", page.items)
    finally:
        client.delete_table(TableName=table_name)


if __name__ == "__main__":
    main()
