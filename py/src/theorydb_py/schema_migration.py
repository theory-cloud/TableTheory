"""Schema migration and AttributeValue-level item transforms.

This module mirrors the Go ``pkg/schema`` auto-migration story for Python:
ensure a target table exists and, when requested, copy data from a source table
into it, optionally backing up the source first and transforming each raw
DynamoDB item on the way.
"""

from __future__ import annotations

import time
from collections.abc import Callable, Mapping
from typing import Any

import boto3

from .model import ModelDefinition
from .schema import create_table, describe_table, ensure_table

# A raw DynamoDB item (attribute name -> AttributeValue).
type MigrationItem = dict[str, Any]

# A transform applied to each item during a data-copying migration. It mirrors
# the Go ``schema.TransformFunc`` AttributeValue-level transforms.
type MigrationTransform = Callable[[Mapping[str, Any]], MigrationItem]

_MAX_BATCH_SIZE = 25
_MAX_RETRIES = 5


def auto_migrate(
    source_model: ModelDefinition[Any],
    *,
    client: Any | None = None,
    target_model: ModelDefinition[Any] | None = None,
    transform: MigrationTransform | None = None,
    backup_table: str | None = None,
    batch_size: int | None = None,
    data_copy: bool = False,
    sleep: Callable[[float], None] = time.sleep,
) -> None:
    """Ensure the target table exists and optionally copy data into it.

    ``target_model`` defaults to ``source_model``. When ``data_copy`` is true
    and the target table differs from the source table, the source table is
    scanned and written to the target table, passing each raw DynamoDB item
    through ``transform`` when one is supplied. ``backup_table`` copies the
    source shape before target creation, matching the Go
    ``Manager.AutoMigrateWithOptions`` walkthrough class.
    """

    resolved_client = client if client is not None else boto3.client("dynamodb")
    target = target_model if target_model is not None else source_model
    size = batch_size if batch_size is not None and batch_size > 0 else _MAX_BATCH_SIZE

    if backup_table:
        _backup_source_table(resolved_client, source_model, backup_table, size, sleep)

    ensure_table(target, client=resolved_client)

    source_table = _table_name(source_model)
    target_table = _table_name(target)
    if data_copy and source_table != target_table:
        _copy_data(resolved_client, source_table, target_table, transform, size, sleep)


def _table_name(model: ModelDefinition[Any]) -> str:
    name = model.table_name
    if not name:
        raise ValueError("ModelDefinition.table_name is required for migration")
    return name


def _backup_source_table(
    client: Any,
    source_model: ModelDefinition[Any],
    backup_table: str,
    batch_size: int,
    sleep: Callable[[float], None],
) -> None:
    # Fail closed if the source table does not exist, matching the Go/TS behavior.
    describe_table(source_model, client=client)
    create_table(source_model, client=client, table_name=backup_table)
    _copy_data(client, _table_name(source_model), backup_table, None, batch_size, sleep)


def _copy_data(
    client: Any,
    source_table: str,
    target_table: str,
    transform: MigrationTransform | None,
    batch_size: int,
    sleep: Callable[[float], None],
) -> None:
    start_key: MigrationItem | None = None
    while True:
        kwargs: dict[str, Any] = {"TableName": source_table, "Limit": batch_size}
        if start_key is not None:
            kwargs["ExclusiveStartKey"] = start_key

        resp = client.scan(**kwargs)
        items = list(resp.get("Items", []))
        if items:
            requests = [
                {"PutRequest": {"Item": transform(item) if transform is not None else item}}
                for item in items
            ]
            _batch_write_all(client, target_table, requests, sleep)

        maybe_start_key = resp.get("LastEvaluatedKey")
        if not maybe_start_key:
            break
        start_key = dict(maybe_start_key)


def _batch_write_all(
    client: Any,
    table_name: str,
    requests: list[dict[str, Any]],
    sleep: Callable[[float], None],
) -> None:
    for start in range(0, len(requests), _MAX_BATCH_SIZE):
        pending = requests[start : start + _MAX_BATCH_SIZE]
        for attempt in range(1, _MAX_RETRIES + 1):
            if not pending:
                break
            resp = client.batch_write_item(RequestItems={table_name: pending})
            pending = list(resp.get("UnprocessedItems", {}).get(table_name, []))
            if pending and attempt < _MAX_RETRIES:
                sleep(attempt * attempt * 0.1)

        # Fall back to individual puts, mirroring the Go/TS batched writer.
        for req in pending:
            item = req.get("PutRequest", {}).get("Item")
            if item is not None:
                client.put_item(TableName=table_name, Item=item)


def copy_all_fields() -> MigrationTransform:
    """Return a transform that passes every attribute through unchanged."""

    def _transform(item: Mapping[str, Any]) -> MigrationItem:
        return dict(item)

    return _transform


def rename_field(old_name: str, new_name: str) -> MigrationTransform:
    """Return a transform that renames one attribute."""

    def _transform(item: Mapping[str, Any]) -> MigrationItem:
        return {(new_name if key == old_name else key): value for key, value in item.items()}

    return _transform


def add_field(name: str, value: Any) -> MigrationTransform:
    """Return a transform that adds an attribute, overwriting if present."""

    def _transform(item: Mapping[str, Any]) -> MigrationItem:
        out = dict(item)
        out[name] = value
        return out

    return _transform


def remove_field(name: str) -> MigrationTransform:
    """Return a transform that drops an attribute."""

    def _transform(item: Mapping[str, Any]) -> MigrationItem:
        return {key: value for key, value in item.items() if key != name}

    return _transform


def chain_transforms(*transforms: MigrationTransform) -> MigrationTransform:
    """Compose transforms left to right."""

    def _transform(item: Mapping[str, Any]) -> MigrationItem:
        current = dict(item)
        for transform in transforms:
            current = transform(current)
        return current

    return _transform
