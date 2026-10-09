"""Schema migration and AttributeValue-level item transforms.

This module mirrors the Go ``pkg/schema`` auto-migration story for Python:
ensure a target table exists and, when requested, copy data from a source table
into it, optionally backing up the source first and transforming each raw
DynamoDB item on the way.
"""

from __future__ import annotations

import os
import time
from collections.abc import Callable, Mapping
from typing import Any

import boto3

from .errors import MigrationEncryptionError
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
    kms_key_arn: str | None = None,
    kms_client: Any | None = None,
    rand_bytes: Callable[[int], bytes] | None = None,
    sleep: Callable[[float], None] = time.sleep,
) -> None:
    """Ensure the target table exists and optionally copy data into it.

    ``target_model`` defaults to ``source_model``. When ``data_copy`` is true
    and the target table differs from the source table, the source table is
    scanned and written to the target table, passing each raw DynamoDB item
    through ``transform`` when one is supplied. ``backup_table`` copies the
    source shape before target creation, matching the Go
    ``Manager.AutoMigrateWithOptions`` walkthrough class.

    When the target model declares encrypted attributes that the source model
    does not, ``kms_key_arn`` (and optionally ``kms_client``/``rand_bytes``) is
    required to encrypt those values on the way across; the migration fails
    closed before touching either table when it is missing. This requirement
    applies only when ``data_copy`` is requested and the target table differs
    from the source table.
    """

    resolved_client = client if client is not None else boto3.client("dynamodb")
    target = target_model if target_model is not None else source_model
    size = batch_size if batch_size is not None and batch_size > 0 else _MAX_BATCH_SIZE

    source_table = _table_name(source_model)
    target_table = _table_name(target)
    will_copy_data = data_copy and source_table != target_table

    target_enc = _encrypted_names(target)
    src_enc = _encrypted_names(source_model)
    needs = sorted(target_enc - src_enc)
    encryption_configured = isinstance(kms_key_arn, str) and kms_key_arn != ""

    # A migration that copies no data cannot write plaintext into the target, so
    # the encryption preflight applies only when a copy will actually happen.
    if will_copy_data and needs and not encryption_configured:
        raise MigrationEncryptionError(
            'migration target model "'
            + _table_model_name(target)
            + '" requires encryption for attribute(s) '
            + ", ".join(needs)
            + "; supply kms_key_arn"
        )

    encrypt: Callable[[Any, str], Any] | None = None
    if will_copy_data and encryption_configured and needs:
        assert isinstance(kms_key_arn, str) and kms_key_arn != ""
        resolved_kms_client = kms_client if kms_client is not None else boto3.client("kms")
        encrypt = _make_encrypt(kms_key_arn, resolved_kms_client, rand_bytes)

    if backup_table:
        _backup_source_table(resolved_client, source_model, backup_table, size, sleep)

    ensure_table(target, client=resolved_client)

    if will_copy_data:
        _copy_data(
            resolved_client,
            source_table,
            target_table,
            transform,
            size,
            sleep,
            src_enc=frozenset(src_enc),
            target_enc=frozenset(target_enc),
            target_model_name=_table_model_name(target),
            encrypt=encrypt,
        )


def _table_name(model: ModelDefinition[Any]) -> str:
    name = model.table_name
    if not name:
        raise ValueError("ModelDefinition.table_name is required for migration")
    return name


def _table_model_name(model: ModelDefinition[Any]) -> str:
    declared = getattr(getattr(model, "model_type", None), "__name__", None)
    if isinstance(declared, str) and declared:
        return declared
    return _table_name(model)


def _encrypted_names(model: ModelDefinition[Any]) -> set[str]:
    attributes = getattr(model, "attributes", None)
    if not isinstance(attributes, Mapping):
        return set()
    return {
        attribute.attribute_name
        for attribute in attributes.values()
        if getattr(attribute, "encrypted", False)
    }


def _is_encrypted_envelope(av: Any) -> bool:
    if not isinstance(av, dict):
        return False
    inner = av.get("M")
    if not isinstance(inner, dict):
        return False
    version = inner.get("v")
    if not isinstance(version, dict) or version.get("N") != "1":
        return False
    for field in ("edk", "nonce", "ct"):
        wrapper = inner.get(field)
        if not isinstance(wrapper, dict):
            return False
        payload = wrapper.get("B")
        if not isinstance(payload, (bytes, bytearray)) or not payload:
            return False
    return True


def _make_encrypt(
    kms_key_arn: str,
    kms_client: Any,
    rand_bytes: Callable[[int], bytes] | None,
) -> Callable[[Any, str], MigrationItem]:
    def encrypt(av: Any, name: str) -> MigrationItem:
        from .encryption import encrypt_attribute_value

        envelope = encrypt_attribute_value(
            av,
            attr_name=name,
            kms_key_arn=kms_key_arn,
            kms_client=kms_client,
            rand_bytes=rand_bytes if rand_bytes is not None else os.urandom,
        )
        return {
            "M": {
                "v": {"N": "1"},
                "edk": {"B": envelope["edk"]},
                "nonce": {"B": envelope["nonce"]},
                "ct": {"B": envelope["ct"]},
            }
        }

    return encrypt


def _guard_migration_item(
    item: MigrationItem,
    src_enc: frozenset[str],
    target_enc: frozenset[str],
    target_model_name: str,
    encrypt: Callable[[Any, str], Any] | None,
) -> MigrationItem:
    for name in sorted(src_enc):
        if name in item and not _is_encrypted_envelope(item[name]):
            raise MigrationEncryptionError(
                'migration attribute "'
                + name
                + '" is declared encrypted in the source model but the copied value is not an encrypted envelope'
            )
    for name in sorted(src_enc - target_enc):
        if name in item and _is_encrypted_envelope(item[name]):
            raise MigrationEncryptionError(
                'migration cannot copy encrypted attribute "'
                + name
                + '" into a target attribute that is not encrypted'
            )
    for name in sorted(target_enc - src_enc):
        if name not in item:
            continue
        if _is_encrypted_envelope(item[name]):
            raise MigrationEncryptionError(
                'migration cannot re-encrypt attribute "'
                + name
                + '": the value is already an encrypted envelope under a different attribute name'
            )
        if encrypt is None:
            raise MigrationEncryptionError(
                'migration target model "'
                + target_model_name
                + '" requires encryption for attribute(s) '
                + name
                + "; supply kms_key_arn"
            )
        item[name] = encrypt(item[name], name)
    return item


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
    *,
    src_enc: frozenset[str] = frozenset(),
    target_enc: frozenset[str] = frozenset(),
    target_model_name: str = "",
    encrypt: Callable[[Any, str], Any] | None = None,
) -> None:
    start_key: MigrationItem | None = None
    while True:
        kwargs: dict[str, Any] = {"TableName": source_table, "Limit": batch_size}
        if start_key is not None:
            kwargs["ExclusiveStartKey"] = start_key

        resp = client.scan(**kwargs)
        items = list(resp.get("Items", []))
        if items:
            requests: list[dict[str, Any]] = []
            for item in items:
                transformed = transform(item) if transform is not None else item
                guarded = _guard_migration_item(transformed, src_enc, target_enc, target_model_name, encrypt)
                requests.append({"PutRequest": {"Item": guarded}})
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
