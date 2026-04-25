from __future__ import annotations

import re
from collections.abc import Mapping, Sequence
from typing import Any

from .errors import ImmutableModelMutationError, ProtectedFieldMutationError, ValidationError
from .model import ModelDefinition


def is_write_once_model(model: ModelDefinition[Any]) -> bool:
    return model.write_policy.mode == "write_once"


def assert_mutable_write_policy(model: ModelDefinition[Any], operation: str) -> None:
    if not is_write_once_model(model):
        return
    raise ImmutableModelMutationError(operation or "mutation")


def assert_protected_fields_can_mutate(
    model: ModelDefinition[Any],
    fields: Sequence[str],
    extra_protected: Sequence[str] = (),
) -> None:
    protected = _protected_attribute_set(model, extra_protected)
    if not protected:
        return

    for field in fields:
        attr = root_attribute_name(canonical_attribute_name(model, field))
        if attr in protected:
            raise ProtectedFieldMutationError(attr)


def apply_write_once_create_condition(model: ModelDefinition[Any], request: dict[str, Any]) -> None:
    if not is_write_once_model(model):
        return

    names = dict(request.get("ExpressionAttributeNames") or {})
    placeholder = _write_once_pk_placeholder(names, model.pk.attribute_name)
    names[placeholder] = model.pk.attribute_name

    write_once_condition = f"attribute_not_exists({placeholder})"
    condition = request.get("ConditionExpression")
    if condition:
        request["ConditionExpression"] = f"({condition}) AND {write_once_condition}"
    else:
        request["ConditionExpression"] = write_once_condition
    request["ExpressionAttributeNames"] = names


def canonical_attribute_name(model: ModelDefinition[Any], field: str) -> str:
    field = str(field).strip()
    if not field:
        return field

    root = root_attribute_name(field)
    if not root:
        return field

    attr_def = model.attributes.get(root)
    if attr_def is not None:
        return _replace_root_attribute(field, attr_def.attribute_name)

    for candidate in model.attributes.values():
        if candidate.attribute_name == root:
            return field

    return field


def root_attribute_name(field: str) -> str:
    field = str(field).strip()
    if not field:
        return ""
    match = re.match(r"^[^\s.\[=,)]+", field)
    return match.group(0) if match else field


def _protected_attribute_set(
    model: ModelDefinition[Any],
    extra_protected: Sequence[str],
) -> set[str]:
    protected: set[str] = set()
    for attr in model.write_policy.protected_attributes:
        root = root_attribute_name(canonical_attribute_name(model, attr))
        if root:
            protected.add(root)
    for attr in _normalize_extra_protected(extra_protected):
        root = root_attribute_name(canonical_attribute_name(model, attr))
        if root:
            protected.add(root)
    return protected


def _normalize_extra_protected(extra_protected: Sequence[str]) -> tuple[str, ...]:
    if isinstance(extra_protected, str):
        raise ValidationError("protected_attributes must be a sequence of names")
    protected: set[str] = set()
    for attr in extra_protected:
        if not isinstance(attr, str):
            raise ValidationError("protected_attributes must contain strings")
        attr = attr.strip()
        if not attr:
            raise ValidationError("protected_attributes must be non-empty")
        protected.add(attr)
    return tuple(sorted(protected))


def _replace_root_attribute(field: str, replacement: str) -> str:
    root = root_attribute_name(field)
    if not root:
        return replacement
    return replacement + field[len(root) :]


def _write_once_pk_placeholder(names: Mapping[str, str], pk_attribute: str) -> str:
    if names.get("#pk") in {None, pk_attribute}:
        return "#pk"

    base = "#write_once_pk"
    placeholder = base
    index = 1
    while placeholder in names and names[placeholder] != pk_attribute:
        placeholder = f"{base}{index}"
        index += 1
    return placeholder
