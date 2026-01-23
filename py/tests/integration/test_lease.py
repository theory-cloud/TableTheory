from __future__ import annotations

import os
import uuid

import boto3
import pytest

from theorydb_py.errors import LeaseHeldError, LeaseNotOwnedError
from theorydb_py.lease import LeaseManager


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


def test_lease_two_contenders() -> None:
    table_name = f"theorydb_py_lease_{uuid.uuid4().hex[:12]}"
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
    waiter = client.get_waiter("table_exists")
    waiter.wait(TableName=table_name)

    try:
        now = 1000.0

        def clock() -> float:
            return now

        mgr1 = LeaseManager(
            client=client,
            table_name=table_name,
            now=clock,
            token=lambda: "tok1",
            ttl_buffer_seconds=10,
        )
        mgr2 = LeaseManager(
            client=client,
            table_name=table_name,
            now=clock,
            token=lambda: "tok2",
            ttl_buffer_seconds=10,
        )

        key = mgr1.lock_key(pk="CACHE#py", sk="LOCK")
        l1 = mgr1.acquire(key, lease_seconds=30)
        assert l1.token == "tok1"

        with pytest.raises(LeaseHeldError):
            mgr2.acquire(key, lease_seconds=30)

        now = 2000.0
        l2 = mgr2.acquire(key, lease_seconds=30)
        assert l2.token == "tok2"

        with pytest.raises(LeaseNotOwnedError):
            mgr1.refresh(l1, lease_seconds=30)
    finally:
        client.delete_table(TableName=table_name)
