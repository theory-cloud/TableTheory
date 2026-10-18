"""Internal glue between a Table and the envelope encryption helpers.

Keeping this here keeps ``table.py`` focused on DynamoDB request/response
shaping and keeps every encryption-specific decision in one place.
"""

from __future__ import annotations

from collections.abc import Mapping
from typing import TYPE_CHECKING, Any, cast

from .attr_types import normalize_json_field_for_storage
from .encryption import (
    RecordBinding,
    decrypt_attribute_value,
    encrypt_attribute_value,
)
from .errors import EncryptionNotConfiguredError, ValidationError

if TYPE_CHECKING:
    from .model import AttributeDefinition
    from .table import Table


def serialize_attr_value(
    table: Table[Any],
    attr_def: AttributeDefinition,
    value: Any,
    *,
    binding: RecordBinding | None = None,
) -> Any:
    if attr_def.converter is not None and value is not None:
        value = attr_def.converter.to_dynamodb(value)

    if attr_def.set and isinstance(value, set) and len(value) == 0:
        return table._serializer.serialize(None)

    if attr_def.json and value is not None:
        value = normalize_json_field_for_storage(
            value,
            storage_type=table._attribute_storage_type(attr_def),
            field_name=attr_def.attribute_name,
        )

    av = table._serializer.serialize(value)

    if attr_def.encrypted:
        if not table._kms_key_arn or table._kms_client is None:
            raise EncryptionNotConfiguredError(
                f"encrypted field requires kms_key_arn: {attr_def.python_name}"
            )

        envelope = encrypt_attribute_value(
            av,
            attr_name=attr_def.attribute_name,
            kms_key_arn=table._kms_key_arn,
            kms_client=table._kms_client,
            rand_bytes=table._rand_bytes,
            binding=binding,
        )
        return table._serializer.serialize(envelope)

    return av


def decrypt_attr_value(
    table: Table[Any],
    attr_def: AttributeDefinition,
    stored: Any,
    *,
    binding: RecordBinding | None = None,
) -> Any:
    if not table._kms_key_arn or table._kms_client is None:
        raise EncryptionNotConfiguredError(f"encrypted field requires kms_key_arn: {attr_def.python_name}")

    envelope = table._deserializer.deserialize(stored)
    if not isinstance(envelope, dict):
        raise ValidationError(f"encrypted envelope must be a map: {attr_def.python_name}")

    decrypted_av = decrypt_attribute_value(
        envelope,
        attr_name=attr_def.attribute_name,
        kms_key_arn=table._kms_key_arn,
        kms_client=table._kms_client,
        binding=binding,
    )
    return table._deserializer.deserialize(cast(Any, decrypted_av))


def record_binding_from_key(table: Table[Any], key: Mapping[str, Any]) -> RecordBinding | None:
    """Build a v2 envelope binding from a raw key map.

    Returns ``None`` when the key is incomplete, which happens for projections
    that omit key attributes. Callers pass that through so v1 envelopes still
    decrypt while v2 fails closed.
    """
    model = table._model
    partition_key = key.get(model.pk.attribute_name)
    if partition_key is None:
        return None

    sort_key = None
    if model.sk is not None:
        sort_key = key.get(model.sk.attribute_name)
        if sort_key is None:
            return None

    return RecordBinding(
        table=table._table_name,
        partition_key=partition_key,
        sort_key=sort_key,
    )


def record_binding_from_item(table: Table[Any], item: Any) -> RecordBinding | None:
    """Build a binding from a dataclass item before its own serialization."""
    model = table._model
    pk_value = getattr(item, model.pk.python_name)
    if pk_value is None:
        return None
    partition_key = serialize_attr_value(table, model.pk, pk_value)

    sort_key = None
    if model.sk is not None:
        sk_value = getattr(item, model.sk.python_name)
        if sk_value is None:
            return None
        sort_key = serialize_attr_value(table, model.sk, sk_value)

    return RecordBinding(
        table=table._table_name,
        partition_key=partition_key,
        sort_key=sort_key,
    )
