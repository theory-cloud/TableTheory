from __future__ import annotations

import os
import uuid
from dataclasses import dataclass

import boto3
import pytest

from theorydb_py import FilterCondition, FilterGroup, ModelDefinition, Table, theorydb_field


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
class FilterItem:
    pk: str = theorydb_field(roles=["pk"])
    sk: str = theorydb_field(roles=["sk"])
    tag: str | None = theorydb_field(omitempty=True, default=None)
    name: str | None = theorydb_field(omitempty=True, default=None)


def test_filters_groups_projection_and_cursor_handoff() -> None:
    if os.environ.get("SKIP_INTEGRATION") in {"1", "true"}:
        pytest.skip("SKIP_INTEGRATION set")

    table_name = f"theorydb_contract_filters_{uuid.uuid4().hex[:12]}"
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
        model = ModelDefinition.from_dataclass(FilterItem, table_name=table_name)
        table: Table[FilterItem] = Table(model, client=client)

        table.put(FilterItem(pk="A", sk="0"))
        table.put(FilterItem(pk="A", sk="1", tag="X", name="Alice"))
        table.put(FilterItem(pk="A", sk="2", tag="Y", name="Bob"))

        first = table.query("A", limit=1, filter=FilterCondition.exists("tag"))
        assert first.items == []
        assert first.next_cursor

        second = table.query(
            "A",
            limit=1,
            cursor=first.next_cursor,
            filter=FilterCondition.exists("tag"),
        )
        assert [r.sk for r in second.items] == ["1"]

        grouped = table.query(
            "A",
            projection=["pk", "sk", "name"],
            filter=FilterGroup.or_(FilterCondition.eq("tag", "X"), FilterCondition.eq("tag", "Y")),
        )
        assert sorted([r.name for r in grouped.items if r.name is not None]) == ["Alice", "Bob"]

        scanned = table.scan_all_segments(total_segments=2, max_workers=2)
        assert {r.sk for r in scanned} == {"0", "1", "2"}
    finally:
        client.delete_table(TableName=table_name)

