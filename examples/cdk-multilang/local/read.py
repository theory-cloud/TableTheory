"""Python reader for the local cross-language no-drift check.

Reads the item written by the Go writer against the same local DynamoDB table
using the in-repo Python runtime, and fails if the shape drifted.
"""

from __future__ import annotations

import os
from dataclasses import dataclass

import boto3

from tabletheory_py import ModelDefinition, Table, theorydb_field


@dataclass(frozen=True)
class DemoItem:
    pk: str = theorydb_field(name="PK", roles=["pk"])
    sk: str = theorydb_field(name="SK", roles=["sk"])
    value: str = theorydb_field(default="")
    lang: str = theorydb_field(default="")


def main() -> None:
    client = boto3.client(
        "dynamodb",
        endpoint_url=os.environ.get("DYNAMODB_ENDPOINT", "http://localhost:8020"),
        region_name=os.environ.get("AWS_REGION", "us-east-1"),
        aws_access_key_id=os.environ.get("AWS_ACCESS_KEY_ID", "dummy"),
        aws_secret_access_key=os.environ.get("AWS_SECRET_ACCESS_KEY", "dummy"),
    )
    table_name = os.environ.get("DEMO_TABLE_NAME", "demo_multilang_local")
    model = ModelDefinition.from_dataclass(DemoItem, table_name=table_name)
    table = Table(model, client=client)

    item = table.get("demo#1", "v1")
    if item.value != "shared-value" or item.lang != "go":
        raise SystemExit(f"python: drift detected: {item}")
    print(f"python: read demo#1/v1 value={item.value!r} lang={item.lang!r}")


if __name__ == "__main__":
    main()
