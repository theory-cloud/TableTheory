from __future__ import annotations

import os
import uuid
from dataclasses import dataclass

import boto3
import pytest

from theorydb_py import (
    ConditionFailedError,
    ModelDefinition,
    NotFoundError,
    Table,
    TransactConditionCheck,
    TransactPut,
    TransactUpdate,
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
class Note:
    pk: str = theorydb_field(roles=["pk"])
    sk: str = theorydb_field(roles=["sk"])
    value: int = theorydb_field()


def test_table_batch_get_write_and_transactions() -> None:
    table_name = f"theorydb_py_batch_{uuid.uuid4().hex[:12]}"
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

        items = [
            Note(pk="A", sk="1", value=1),
            Note(pk="A", sk="2", value=2),
            Note(pk="B", sk="1", value=3),
        ]

        table.batch_write(puts=items)

        got = table.batch_get([("A", "1"), ("A", "2"), ("B", "1")])
        assert set(got) == set(items)

        table.batch_write(deletes=[("A", "1")])
        with pytest.raises(NotFoundError):
            table.get("A", "1")

        table.put(Note(pk="T", sk="1", value=1))
        table.transact_write(
            [
                TransactUpdate(
                    pk="T",
                    sk="1",
                    updates={"value": 2},
                    condition_expression="#v = :expected",
                    expression_attribute_names={"#v": "value"},
                    expression_attribute_values={":expected": 1},
                )
            ]
        )
        assert table.get("T", "1").value == 2

        with pytest.raises(ConditionFailedError):
            table.transact_write(
                [
                    TransactUpdate(
                        pk="T",
                        sk="1",
                        updates={"value": 3},
                        condition_expression="#v = :expected",
                        expression_attribute_names={"#v": "value"},
                        expression_attribute_values={":expected": 999},
                    )
                ]
            )

        table.put(Note(pk="C", sk="1", value=5))
        table.transact_write(
            [
                TransactConditionCheck(
                    pk="C",
                    sk="1",
                    condition_expression="#v = :expected",
                    expression_attribute_names={"#v": "value"},
                    expression_attribute_values={":expected": 5},
                ),
                TransactPut(
                    item=Note(pk="C", sk="2", value=6),
                    condition_expression="attribute_not_exists(#pk)",
                    expression_attribute_names={"#pk": "pk"},
                ),
            ]
        )
        assert table.get("C", "2").value == 6

        with pytest.raises(ConditionFailedError):
            table.transact_write(
                [
                    TransactConditionCheck(
                        pk="C",
                        sk="1",
                        condition_expression="#v = :expected",
                        expression_attribute_names={"#v": "value"},
                        expression_attribute_values={":expected": 999},
                    ),
                    TransactPut(
                        item=Note(pk="C", sk="3", value=7),
                        condition_expression="attribute_not_exists(#pk)",
                        expression_attribute_names={"#pk": "pk"},
                    ),
                ]
            )

        with pytest.raises(NotFoundError):
            table.get("C", "3")
    finally:
        client.delete_table(TableName=table_name)
