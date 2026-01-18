from __future__ import annotations

import os
import uuid
from dataclasses import dataclass

import boto3
import pytest

from theorydb_py import (
    FilterCondition,
    FilterGroup,
    ModelDefinition,
    SortKeyCondition,
    Table,
    ValidationError,
    gsi,
    lsi,
    theorydb_field,
)


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
class Record:
    pk: str = theorydb_field(roles=["pk"])
    sk: str = theorydb_field(roles=["sk"])
    value: int = theorydb_field()
    gsi_pk: str = theorydb_field()
    gsi_sk: str = theorydb_field()
    lsi_sk: str = theorydb_field()
    tag: str | None = theorydb_field(omitempty=True, default=None)
    note: str = theorydb_field(default="")


def test_table_query_scan_indexes_and_pagination() -> None:
    table_name = f"theorydb_py_query_{uuid.uuid4().hex[:12]}"
    client = _client()
    client.create_table(
        TableName=table_name,
        KeySchema=[{"AttributeName": "pk", "KeyType": "HASH"}, {"AttributeName": "sk", "KeyType": "RANGE"}],
        AttributeDefinitions=[
            {"AttributeName": "pk", "AttributeType": "S"},
            {"AttributeName": "sk", "AttributeType": "S"},
            {"AttributeName": "gsi_pk", "AttributeType": "S"},
            {"AttributeName": "gsi_sk", "AttributeType": "S"},
            {"AttributeName": "lsi_sk", "AttributeType": "S"},
        ],
        GlobalSecondaryIndexes=[
            {
                "IndexName": "gsi1",
                "KeySchema": [
                    {"AttributeName": "gsi_pk", "KeyType": "HASH"},
                    {"AttributeName": "gsi_sk", "KeyType": "RANGE"},
                ],
                "Projection": {"ProjectionType": "ALL"},
            }
        ],
        LocalSecondaryIndexes=[
            {
                "IndexName": "lsi1",
                "KeySchema": [
                    {"AttributeName": "pk", "KeyType": "HASH"},
                    {"AttributeName": "lsi_sk", "KeyType": "RANGE"},
                ],
                "Projection": {"ProjectionType": "ALL"},
            }
        ],
        BillingMode="PAY_PER_REQUEST",
    )
    waiter = client.get_waiter("table_exists")
    waiter.wait(TableName=table_name)

    try:
        model = ModelDefinition.from_dataclass(
            Record,
            table_name=table_name,
            indexes=[gsi("gsi1", partition="gsi_pk", sort="gsi_sk"), lsi("lsi1", sort="lsi_sk")],
        )
        table = Table(model, client=client)

        pk_items = [
            ("000", "A", "X000"),
            ("001", "B", "X001"),
            ("010", "C", "Y010"),
            ("100", "D", "Y100"),
            ("101", "E", "Y101"),
        ]
        for sk, lsi_sk, gsi_sk in pk_items:
            tag = None
            if sk == "001":
                tag = "A"
            if sk == "100":
                tag = "B"
            table.put(
                Record(
                    pk="P1",
                    sk=sk,
                    value=int(sk),
                    gsi_pk="G1",
                    gsi_sk=gsi_sk,
                    lsi_sk=lsi_sk,
                    tag=tag,
                    note=f"note-{sk}",
                )
            )

        table.put(
            Record(pk="P2", sk="200", value=200, gsi_pk="G2", gsi_sk="Z200", lsi_sk="Z", note="note-200")
        )

        page = table.query("P1", sort=SortKeyCondition.begins_with("0"))
        assert [r.sk for r in page.items] == ["000", "001", "010"]

        page = table.query("P1", sort=SortKeyCondition.between("001", "100"))
        assert [r.sk for r in page.items] == ["001", "010", "100"]

        page = table.query("P1", sort=SortKeyCondition.lt("010"))
        assert [r.sk for r in page.items] == ["000", "001"]

        collected: list[str] = []
        cursor: str | None = None
        while True:
            page = table.query("P1", limit=2, cursor=cursor)
            collected.extend([r.sk for r in page.items])
            if page.next_cursor is None:
                break
            cursor = page.next_cursor
        assert collected == ["000", "001", "010", "100", "101"]

        page = table.query("P1", limit=2, scan_forward=False)
        assert [r.sk for r in page.items] == ["101", "100"]

        page = table.query("G1", index_name="gsi1", sort=SortKeyCondition.begins_with("X"))
        assert [r.gsi_sk for r in page.items] == ["X000", "X001"]

        with pytest.raises(ValidationError):
            table.query("G1", index_name="gsi1", consistent_read=True)

        page = table.query("P1", index_name="lsi1", sort=SortKeyCondition.between("B", "D"))
        assert [r.lsi_sk for r in page.items] == ["B", "C", "D"]

        with pytest.raises(ValidationError):
            table.query("P1", projection=["pk", "sk"])

        page = table.query(
            "P1",
            projection=["pk", "sk", "value", "gsi_pk", "gsi_sk", "lsi_sk"],
        )
        assert all(r.note == "" for r in page.items)

        filtered0 = table.query("P1", limit=1, filter=FilterCondition.exists("tag"))
        assert filtered0.items == []
        assert filtered0.next_cursor

        filtered1 = table.query(
            "P1",
            limit=1,
            cursor=filtered0.next_cursor,
            filter=FilterGroup.or_(FilterCondition.eq("tag", "A"), FilterCondition.eq("tag", "B")),
        )
        assert [r.tag for r in filtered1.items] == ["A"]
        assert filtered1.next_cursor

        scan_keys: set[tuple[str, str]] = set()
        cursor = None
        while True:
            page = table.scan(limit=2, cursor=cursor)
            for r in page.items:
                scan_keys.add((r.pk, r.sk))
            if page.next_cursor is None:
                break
            cursor = page.next_cursor

        assert scan_keys == {
            ("P1", "000"),
            ("P1", "001"),
            ("P1", "010"),
            ("P1", "100"),
            ("P1", "101"),
            ("P2", "200"),
        }

        parallel = table.scan_all_segments(total_segments=2, max_workers=2)
        assert {(r.pk, r.sk) for r in parallel} == scan_keys
    finally:
        client.delete_table(TableName=table_name)
