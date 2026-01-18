from __future__ import annotations

import os
import uuid
from dataclasses import dataclass

import boto3
import pytest

from theorydb_py import ConditionFailedError, ModelDefinition, NotFoundError, Table, theorydb_field


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
class Note:
    pk: str = theorydb_field(roles=["pk"])
    sk: str = theorydb_field(roles=["sk"])
    value: int = theorydb_field()
    note: str = theorydb_field(omitempty=True, default="")
    payload: dict[str, int] = theorydb_field(json=True, omitempty=True, default_factory=dict)


def test_table_crud_round_trip_and_conditions() -> None:
    table_name = f"theorydb_py_table_{uuid.uuid4().hex[:12]}"
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
        model = ModelDefinition.from_dataclass(Note, table_name=table_name)
        table = Table(model, client=client)

        item = Note(pk="A", sk="B", value=1, note="", payload={"a": 1})

        table.put(
            item,
            condition_expression="attribute_not_exists(#pk)",
            expression_attribute_names={"#pk": "pk"},
        )

        with pytest.raises(ConditionFailedError):
            table.put(
                item,
                condition_expression="attribute_not_exists(#pk)",
                expression_attribute_names={"#pk": "pk"},
            )

        got = table.get("A", "B")
        assert got == item

        updated = table.update(
            "A",
            "B",
            updates={"value": 2},
            condition_expression="#v = :expected",
            expression_attribute_names={"#v": "value"},
            expression_attribute_values={":expected": 1},
        )
        assert updated.value == 2

        with pytest.raises(ConditionFailedError):
            table.update(
                "A",
                "B",
                updates={"value": 3},
                condition_expression="#v = :expected",
                expression_attribute_names={"#v": "value"},
                expression_attribute_values={":expected": 999},
            )

        table.delete("A", "B")
        with pytest.raises(NotFoundError):
            table.get("A", "B")
    finally:
        client.delete_table(TableName=table_name)
