# Migration Guide (Python)

This guide helps migrate from raw boto3 DynamoDB usage to `tabletheory_py` / `theorydb_py`.

## Migration: raw `put_item` → `table.put`

### Problem

Hand-authored `client.put_item(...)` payloads are verbose and drift-prone across services.

### Solution

Define a model once and use typed helpers.

```python
table = Table(model, client=client)
table.put(Note(pk="A", sk="1", value=123))
```

## Schema migration (`auto_migrate`)

This mirrors the Go [migration guide](https://github.com/theory-cloud/tabletheory/blob/main/docs/migration-guide.md)
schema-migration story: move data from a v1 table shape to a v2 shape, optionally backing up the source first and
transforming each item on the way. It is reproducible against DynamoDB Local — no AWS account required.

`auto_migrate(source_model, client=client, options...)` ensures the target table exists and, when `data_copy=True`, scans
the source table and writes each item into the target table through an optional `transform`. Item-level transforms mirror
the Go helpers:

- `rename_field(old_name, new_name)`
- `add_field(name, value)`
- `remove_field(name)`
- `copy_all_fields()`
- `chain_transforms(*transforms)` composes them left to right

### Walkthrough

```python
from __future__ import annotations

import os
from dataclasses import dataclass

import boto3

from tabletheory_py import ModelDefinition, ensure_table, theorydb_field
from tabletheory_py.schema_migration import add_field, auto_migrate, chain_transforms, rename_field

client = boto3.client(
    "dynamodb",
    endpoint_url=os.environ.get("DYNAMODB_ENDPOINT", "http://localhost:8000"),
    region_name=os.environ.get("AWS_REGION", "us-east-1"),
    aws_access_key_id="dummy",
    aws_secret_access_key="dummy",
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


user_v1 = ModelDefinition.from_dataclass(UserV1, table_name="users_migration_v1")
user_v2 = ModelDefinition.from_dataclass(UserV2, table_name="users_migration_v2")

# Seed the v1 table.
ensure_table(user_v1, client=client)
client.put_item(
    TableName=user_v1.table_name,
    Item={"PK": {"S": "USER#1"}, "SK": {"S": "v1"}, "name": {"S": "Ada"}},
)

# Back up v1, then migrate v1 -> v2: rename `name` -> `displayName` and add `status`.
auto_migrate(
    user_v1,
    client=client,
    target_model=user_v2,
    data_copy=True,
    backup_table="users_migration_backup",
    transform=chain_transforms(
        rename_field("name", "displayName"),
        add_field("status", {"S": "active"}),
    ),
)
# users_migration_v2 now holds { PK, SK, displayName: "Ada", status: "active" };
# users_migration_backup holds the original { PK, SK, name: "Ada" }.
```

A runnable version of this walkthrough lives in `py/tests/integration/test_schema_migration.py` and runs on the Python
integration lane against DynamoDB Local.
