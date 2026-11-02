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
from dataclasses import dataclass
from typing import TYPE_CHECKING, Any

import boto3

from .encryption import decrypt_attribute_value, encrypt_attribute_value
from .errors import MigrationEncryptionError
from .model import ModelDefinition
from .schema import create_table, describe_table, ensure_table

if TYPE_CHECKING:
    from .encryption import RecordBinding

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
    encryption_configured = isinstance(kms_key_arn, str) and kms_key_arn != ""

    # Every target encrypted attribute needs the provider when data is copied:
    # a new name must be encrypted, and a same-name attribute must be rebound to
    # the target record instead of carrying the source binding unchanged.
    if will_copy_data and target_enc and not encryption_configured:
        raise MigrationEncryptionError(
            'migration target model "'
            + _table_model_name(target)
            + '" requires encryption for attribute(s) '
            + ", ".join(sorted(target_enc))
            + "; supply kms_key_arn"
        )

    plan: _MigrationEncryptionPlan | None = None
    if will_copy_data:
        resolved_kms_client = None
        if encryption_configured:
            assert isinstance(kms_key_arn, str)
            resolved_kms_client = kms_client if kms_client is not None else boto3.client("kms")
        plan = _MigrationEncryptionPlan(
            source_model=source_model,
            source_table=source_table,
            target_model=target,
            target_table=target_table,
            target_model_name=_table_model_name(target),
            src_enc=frozenset(src_enc),
            target_enc=frozenset(target_enc),
            kms_key_arn=kms_key_arn if encryption_configured else None,
            kms_client=resolved_kms_client,
            rand_bytes=rand_bytes,
        )

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
            plan=plan,
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


def _envelope_from_wire_attribute_value(av: Any, *, name: str) -> dict[str, Any]:
    inner = av.get("M") if isinstance(av, Mapping) else None
    if not isinstance(inner, Mapping):
        raise MigrationEncryptionError('migration attribute "' + name + '" is not an encrypted envelope')
    try:
        return {
            "v": int(inner["v"]["N"]),
            "edk": bytes(inner["edk"]["B"]),
            "nonce": bytes(inner["nonce"]["B"]),
            "ct": bytes(inner["ct"]["B"]),
        }
    except (KeyError, TypeError, ValueError) as err:
        raise MigrationEncryptionError(
            'migration attribute "' + name + '" is not a valid encrypted envelope'
        ) from err


def _envelope_to_attribute_value(envelope: Mapping[str, Any]) -> MigrationItem:
    return {
        "M": {
            "v": {"N": str(envelope["v"])},
            "edk": {"B": envelope["edk"]},
            "nonce": {"B": envelope["nonce"]},
            "ct": {"B": envelope["ct"]},
        }
    }


def _is_encrypted_envelope(av: Any) -> bool:
    if not isinstance(av, dict):
        return False
    inner = av.get("M")
    if not isinstance(inner, dict):
        return False
    version = inner.get("v")
    if not isinstance(version, dict) or version.get("N") not in {"1", "2"}:
        return False
    for field in ("edk", "nonce", "ct"):
        wrapper = inner.get(field)
        if not isinstance(wrapper, dict):
            return False
        payload = wrapper.get("B")
        if not isinstance(payload, (bytes, bytearray)) or not payload:
            return False
    return True


def _binding_from_item(
    item: Mapping[str, Any],
    target_model: ModelDefinition[Any],
    target_table: str,
) -> RecordBinding:
    """Build the v2 envelope binding from the target record's key attributes.

    A target item that lacks its key attributes cannot be bound to a record.
    Fail closed with the same error class as Go/TS instead of silently writing
    an unbound v1 envelope.
    """

    pk = getattr(target_model, "pk", None)
    pk_name = getattr(pk, "attribute_name", None)
    if not isinstance(pk_name, str):
        raise MigrationEncryptionError(
            'migration cannot bind encrypted attributes: target model "'
            + getattr(getattr(target_model, "model_type", None), "__name__", target_table)
            + '" has no partition key attribute'
        )
    partition_key = item.get(pk_name)
    if not isinstance(partition_key, dict):
        raise MigrationEncryptionError(
            'migration cannot bind encrypted attributes: item is missing partition key "' + pk_name + '"'
        )

    sort_key = None
    sk = getattr(target_model, "sk", None)
    if sk is not None:
        sk_name = getattr(sk, "attribute_name", None)
        if not isinstance(sk_name, str):
            raise MigrationEncryptionError(
                'migration cannot bind encrypted attributes: target model "'
                + getattr(getattr(target_model, "model_type", None), "__name__", target_table)
                + '" has no sort key attribute'
            )
        sort_key = item.get(sk_name)
        if not isinstance(sort_key, dict):
            raise MigrationEncryptionError(
                'migration cannot bind encrypted attributes: item is missing sort key "' + sk_name + '"'
            )

    from .encryption import RecordBinding

    return RecordBinding(table=target_table, partition_key=partition_key, sort_key=sort_key)


@dataclass(frozen=True)
class _MigrationEncryptionPlan:
    """Per-item encryption decisions for a data-copying migration.

    A same-name encrypted attribute is opened with the source record binding and
    re-sealed with the target binding, so a copied envelope only decrypts at the
    target record. Every decision runs before the page is handed to the batch
    writer.
    """

    source_model: ModelDefinition[Any]
    source_table: str
    target_model: ModelDefinition[Any]
    target_table: str
    target_model_name: str
    src_enc: frozenset[str]
    target_enc: frozenset[str]
    kms_key_arn: str | None
    kms_client: Any | None
    rand_bytes: Callable[[int], bytes] | None

    def guard_item(self, item: MigrationItem) -> MigrationItem:
        for name in sorted(self.src_enc):
            if name in item and not _is_encrypted_envelope(item[name]):
                raise MigrationEncryptionError(
                    'migration attribute "'
                    + name
                    + '" is declared encrypted in the source model but the copied value is not an encrypted envelope'
                )
        for name in sorted(self.target_enc & self.src_enc):
            if name in item:
                self._rebind(item, name)
        for name in sorted(self.src_enc - self.target_enc):
            if name in item and _is_encrypted_envelope(item[name]):
                raise MigrationEncryptionError(
                    'migration cannot copy encrypted attribute "'
                    + name
                    + '" into a target attribute that is not encrypted'
                )
        for name in sorted(self.target_enc - self.src_enc):
            if name not in item:
                continue
            if _is_encrypted_envelope(item[name]):
                raise MigrationEncryptionError(
                    'migration cannot re-encrypt attribute "'
                    + name
                    + '": the value is already an encrypted envelope under a different attribute name'
                )
            self._encrypt_new(item, name)
        return item

    def _require_provider(self, name: str) -> tuple[str, Any]:
        if self.kms_key_arn is None or self.kms_client is None:
            raise MigrationEncryptionError(
                'migration target model "'
                + self.target_model_name
                + '" requires encryption for attribute(s) '
                + name
                + "; supply kms_key_arn"
            )
        return self.kms_key_arn, self.kms_client

    def _rebind(self, item: MigrationItem, name: str) -> None:
        key_arn, kms_client = self._require_provider(name)
        source_binding = _binding_from_item(item, self.source_model, self.source_table)
        target_binding = _binding_from_item(item, self.target_model, self.target_table)
        plaintext = decrypt_attribute_value(
            _envelope_from_wire_attribute_value(item[name], name=name),
            attr_name=name,
            kms_key_arn=key_arn,
            kms_client=kms_client,
            binding=source_binding,
        )
        item[name] = _envelope_to_attribute_value(
            encrypt_attribute_value(
                plaintext,
                attr_name=name,
                kms_key_arn=key_arn,
                kms_client=kms_client,
                rand_bytes=self.rand_bytes if self.rand_bytes is not None else os.urandom,
                binding=target_binding,
            )
        )

    def _encrypt_new(self, item: MigrationItem, name: str) -> None:
        key_arn, kms_client = self._require_provider(name)
        target_binding = _binding_from_item(item, self.target_model, self.target_table)
        item[name] = _envelope_to_attribute_value(
            encrypt_attribute_value(
                item[name],
                attr_name=name,
                kms_key_arn=key_arn,
                kms_client=kms_client,
                rand_bytes=self.rand_bytes if self.rand_bytes is not None else os.urandom,
                binding=target_binding,
            )
        )


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
    plan: _MigrationEncryptionPlan | None = None,
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
                guarded = plan.guard_item(transformed) if plan is not None else transformed
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
