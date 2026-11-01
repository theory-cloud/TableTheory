from __future__ import annotations

import os
from dataclasses import dataclass

import boto3
import pytest

from tabletheory_py import (
    ModelDefinition,
    TTLAttributeMismatchError,
    ValidationError,
    theorydb_field,
)
from tabletheory_py.schema import create_table, delete_table, ensure_table


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
class TtlRecord:
    id: str = theorydb_field(name="ID", roles=["pk"])
    expires_at: int = theorydb_field(name="expiresAt", roles=["ttl"])


MODEL = ModelDefinition.from_dataclass(
    TtlRecord,
    table_name="tabletheory_local_t15_ttl_create_py",
)


def test_ensure_table_leaves_existing_disabled_ttl_untouched() -> None:
    client = _client()
    delete_table(MODEL, client=client, ignore_missing=True)
    try:
        client.create_table(
            TableName=MODEL.table_name,
            BillingMode="PAY_PER_REQUEST",
            KeySchema=[{"AttributeName": "ID", "KeyType": "HASH"}],
            AttributeDefinitions=[{"AttributeName": "ID", "AttributeType": "S"}],
        )
        ensure_table(MODEL, client=client)

        ttl = client.describe_time_to_live(TableName=MODEL.table_name)["TimeToLiveDescription"]
        assert ttl["TimeToLiveStatus"] == "DISABLED"
        assert "AttributeName" not in ttl
    finally:
        delete_table(MODEL, client=client, ignore_missing=True)


def test_create_table_existing_ttl_table_is_idempotent() -> None:
    client = _client()
    delete_table(MODEL, client=client, ignore_missing=True)
    try:
        create_table(MODEL, client=client)
        # A second CreateTable on the existing, TTL-enabled table must behave
        # like the already-exists outcome of a table without TTL: no error.
        create_table(MODEL, client=client)

        ttl = client.describe_time_to_live(TableName=MODEL.table_name)["TimeToLiveDescription"]
        assert ttl["TimeToLiveStatus"] == "ENABLED"
        assert ttl["AttributeName"] == "expiresAt"
    finally:
        delete_table(MODEL, client=client, ignore_missing=True)


def test_create_table_existing_table_ttl_on_different_attribute_raises_clear_error() -> None:
    client = _client()
    delete_table(MODEL, client=client, ignore_missing=True)
    try:
        client.create_table(
            TableName=MODEL.table_name,
            BillingMode="PAY_PER_REQUEST",
            KeySchema=[{"AttributeName": "ID", "KeyType": "HASH"}],
            AttributeDefinitions=[{"AttributeName": "ID", "AttributeType": "S"}],
        )
        client.get_waiter("table_exists").wait(TableName=MODEL.table_name)
        client.update_time_to_live(
            TableName=MODEL.table_name,
            TimeToLiveSpecification={
                "AttributeName": "LegacyTtl",
                "Enabled": True,
            },
        )

        with pytest.raises(TTLAttributeMismatchError) as excinfo:
            create_table(MODEL, client=client)

        assert isinstance(excinfo.value, ValidationError)
        message = str(excinfo.value)
        assert "expiresAt" in message
        assert "LegacyTtl" in message
        assert "different attribute" in message
    finally:
        delete_table(MODEL, client=client, ignore_missing=True)
