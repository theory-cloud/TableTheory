"""A clean consumer of the built Python distribution.

This file is copied into an empty directory that has installed the built wheel
and nothing from this repository, so the imports below resolve through the
published package entry points. It exercises one real call path against
DynamoDB Local: create a table with boto3, define a model, write and read an
item through TableTheory, and read back an omitted empty attribute.
"""

from __future__ import annotations

import os
import sys
import time
import uuid
from dataclasses import dataclass

import boto3

from tabletheory_py import ModelDefinition, Table, theorydb_field

if not os.environ.get("DYNAMODB_ENDPOINT"):
    raise SystemExit("DYNAMODB_ENDPOINT is required: this check proves a real call path")

endpoint = os.environ["DYNAMODB_ENDPOINT"]
table_name = f"tabletheory_py_consumer_{uuid.uuid4().hex[:12]}"

client = boto3.client(
    "dynamodb",
    endpoint_url=endpoint,
    region_name=os.environ.get("AWS_REGION", "us-east-1"),
    aws_access_key_id=os.environ.get("AWS_ACCESS_KEY_ID", "dummy"),
    aws_secret_access_key=os.environ.get("AWS_SECRET_ACCESS_KEY", "dummy"),
)


@dataclass(frozen=True)
class Note:
    pk: str = theorydb_field(roles=["pk"])
    sk: str = theorydb_field(roles=["sk"])
    body: str = theorydb_field(omitempty=True, default="")


client.create_table(
    TableName=table_name,
    KeySchema=[
        {"AttributeName": "pk", "KeyType": "HASH"},
        {"AttributeName": "sk", "KeyType": "RANGE"},
    ],
    AttributeDefinitions=[
        {"AttributeName": "pk", "AttributeType": "S"},
        {"AttributeName": "sk", "AttributeType": "S"},
    ],
    BillingMode="PAY_PER_REQUEST",
)
client.get_waiter("table_exists").wait(TableName=table_name)

try:
    table = Table(ModelDefinition.from_dataclass(Note, table_name=table_name), client=client)

    item = Note(pk="NOTE#consumer", sk="V1", body="installed from the built wheel")
    table.put(item)
    assert table.get("NOTE#consumer", "V1") == item

    empty = Note(pk="NOTE#consumer", sk="V2")
    table.put(empty)
    assert not table.get("NOTE#consumer", "V2").body

    table.delete("NOTE#consumer", "V1")
    assert table.get_or_none("NOTE#consumer", "V1") is None

    print(
        f"consumer-install(py): PASS on python {sys.version.split()[0]} via {endpoint} "
        f"(package {__import__('tabletheory_py').__name__})"
    )
finally:
    # DynamoDB Local honours DeleteTable synchronously; the retry keeps the
    # receipt from failing on a slow local backend.
    for attempt in range(5):
        try:
            client.delete_table(TableName=table_name)
            break
        except Exception:  # noqa: BLE001 - last-attempt failure surfaces below
            if attempt == 4:
                raise
            time.sleep(0.2)
