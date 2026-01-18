from __future__ import annotations

import os
import uuid

import boto3


def _dynamodb_endpoint() -> str:
    return os.environ.get("DYNAMODB_ENDPOINT", "http://localhost:8000")


def test_dynamodb_local_smoke_put_get_delete() -> None:
    table_name = f"theorydb_py_smoke_{uuid.uuid4().hex[:12]}"
    dynamodb = boto3.resource(
        "dynamodb",
        endpoint_url=_dynamodb_endpoint(),
        region_name=os.environ.get("AWS_REGION", "us-east-1"),
        aws_access_key_id=os.environ.get("AWS_ACCESS_KEY_ID", "dummy"),
        aws_secret_access_key=os.environ.get("AWS_SECRET_ACCESS_KEY", "dummy"),
    )
    table = dynamodb.create_table(
        TableName=table_name,
        KeySchema=[{"AttributeName": "pk", "KeyType": "HASH"}, {"AttributeName": "sk", "KeyType": "RANGE"}],
        AttributeDefinitions=[
            {"AttributeName": "pk", "AttributeType": "S"},
            {"AttributeName": "sk", "AttributeType": "S"},
        ],
        BillingMode="PAY_PER_REQUEST",
    )
    table.wait_until_exists()
    try:
        table.put_item(Item={"pk": "A", "sk": "B", "value": 1})
        resp = table.get_item(Key={"pk": "A", "sk": "B"})
        assert resp.get("Item") == {"pk": "A", "sk": "B", "value": 1}
        table.delete_item(Key={"pk": "A", "sk": "B"})
    finally:
        table.delete()
