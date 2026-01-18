from __future__ import annotations

import os
import uuid
from dataclasses import dataclass

import boto3
import pytest

from theorydb_py import ModelDefinition, Table, theorydb_field


def _dynamodb_endpoint() -> str:
    return os.environ.get("DYNAMODB_ENDPOINT", "http://localhost:8000")


def _client():
    return boto3.client(
        "dynamodb",
        endpoint_url=_dynamodb_endpoint(),
        region_name=os.environ.get("AWS_REGION", "us-east-1"),
        aws_access_key_id=os.environ.get("AWS_ACCESS_KEY_ID", "dummy"),
        aws_secret_access_key=os.environ.get("AWS_SECRET_ACCESS_KEY", "dummy"),
    )


@dataclass(frozen=True)
class ReservedWordItem:
    pk: str = theorydb_field(roles=["pk"])
    sk: str = theorydb_field(roles=["sk"])
    name: str = theorydb_field()


def test_reserved_word_update_escapes_attribute_names() -> None:
    if os.environ.get("SKIP_INTEGRATION") in {"1", "true"}:
        pytest.skip("SKIP_INTEGRATION set")

    table_name = f"theorydb_contract_reserved_{uuid.uuid4().hex[:12]}"
    client = _client()
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
        model = ModelDefinition.from_dataclass(ReservedWordItem, table_name=table_name)
        table = Table(model, client=client)

        table.put(ReservedWordItem(pk="A", sk="B", name="v0"))
        updated = table.update("A", "B", updates={"name": "v1"})
        assert updated.name == "v1"
    finally:
        client.delete_table(TableName=table_name)

