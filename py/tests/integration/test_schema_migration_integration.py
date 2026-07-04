from __future__ import annotations

import os
import uuid
from dataclasses import dataclass
from typing import Any

import boto3

from theorydb_py import ModelDefinition, theorydb_field
from theorydb_py.schema import delete_table, ensure_table
from theorydb_py.schema_migration import add_field, auto_migrate, chain_transforms, rename_field


def _dynamodb_endpoint() -> str:
    return os.environ.get("DYNAMODB_ENDPOINT", "http://localhost:8000")


def _client() -> Any:
    return boto3.client(
        "dynamodb",
        endpoint_url=_dynamodb_endpoint(),
        region_name=os.environ.get("AWS_REGION", "us-east-1"),
        aws_access_key_id=os.environ.get("AWS_ACCESS_KEY_ID", "dummy"),
        aws_secret_access_key=os.environ.get("AWS_SECRET_ACCESS_KEY", "dummy"),
    )


@dataclass(frozen=True)
class UserV1:
    pk: str = theorydb_field(name="PK", roles=["pk"])
    sk: str = theorydb_field(name="SK", roles=["sk"])
    name: str = theorydb_field(default="")


@dataclass(frozen=True)
class UserV2:
    pk: str = theorydb_field(name="PK", roles=["pk"])
    sk: str = theorydb_field(name="SK", roles=["sk"])
    display_name: str = theorydb_field(name="displayName", default="")
    status: str = theorydb_field(default="")


def test_schema_migration_walkthrough_against_dynamodb_local() -> None:
    suffix = uuid.uuid4().hex[:12]
    v1_table = f"users_migration_v1_{suffix}"
    v2_table = f"users_migration_v2_{suffix}"
    backup_table = f"users_migration_backup_{suffix}"
    client = _client()

    user_v1 = ModelDefinition.from_dataclass(UserV1, table_name=v1_table)
    user_v2 = ModelDefinition.from_dataclass(UserV2, table_name=v2_table)

    try:
        delete_table(user_v1, client=client, ignore_missing=True)
        delete_table(user_v2, client=client, ignore_missing=True)
        delete_table(user_v1, client=client, table_name=backup_table, ignore_missing=True)

        ensure_table(user_v1, client=client)
        seed = [("USER#1", "Ada"), ("USER#2", "Grace")]
        for pk, name in seed:
            client.put_item(
                TableName=v1_table,
                Item={"PK": {"S": pk}, "SK": {"S": "v1"}, "name": {"S": name}},
            )

        # Migrate v1 -> v2: back up the source, rename `name` -> `displayName`, add `status`.
        auto_migrate(
            user_v1,
            client=client,
            target_model=user_v2,
            data_copy=True,
            backup_table=backup_table,
            transform=chain_transforms(
                rename_field("name", "displayName"),
                add_field("status", {"S": "active"}),
            ),
        )

        migrated = client.scan(TableName=v2_table)
        assert len(migrated.get("Items", [])) == 2
        for item in migrated.get("Items", []):
            assert item.get("displayName", {}).get("S")
            assert item.get("status", {}).get("S") == "active"
            assert "name" not in item

        backup = client.scan(TableName=backup_table)
        assert len(backup.get("Items", [])) == 2
        for item in backup.get("Items", []):
            assert item.get("name", {}).get("S")
    finally:
        delete_table(user_v1, client=client, ignore_missing=True)
        delete_table(user_v2, client=client, ignore_missing=True)
        delete_table(user_v1, client=client, table_name=backup_table, ignore_missing=True)
