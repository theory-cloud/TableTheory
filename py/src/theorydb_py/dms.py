from __future__ import annotations

import json
from collections.abc import Mapping, Sequence
from dataclasses import MISSING, fields
from decimal import Decimal
from typing import Any, Union, cast, get_args, get_origin, get_type_hints

import yaml

from .errors import ValidationError
from .model import ModelDefinition


def parse_dms_document(raw: str) -> dict[str, Any]:
    try:
        parsed = yaml.safe_load(raw)
    except Exception as err:
        raise ValidationError("invalid DMS YAML/JSON") from err

    if not isinstance(parsed, dict):
        raise ValidationError("DMS document must be a map/object")

    _assert_json_compatible(parsed, path="dms")

    version = parsed.get("dms_version")
    if version != "0.1":
        raise ValidationError(f"unsupported dms_version: {version!r}")

    models = parsed.get("models")
    if not isinstance(models, list) or len(models) == 0:
        raise ValidationError("DMS document must include models[]")

    return parsed


def get_dms_model(doc: Mapping[str, Any], name: str) -> dict[str, Any]:
    models = doc.get("models")
    if not isinstance(models, list):
        raise ValidationError("DMS document missing models[]")
    for model in models:
        if isinstance(model, dict) and model.get("name") == name:
            return cast(dict[str, Any], model)
    raise ValidationError(f"DMS model not found: {name}")


def assert_model_definition_equivalent_to_dms(
    model: ModelDefinition[Any],
    dms_model: Mapping[str, Any],
    *,
    ignore_table_name: bool,
) -> None:
    want = _normalize_dms_model(dms_model, ignore_table_name=ignore_table_name)
    got = _normalize_dms_model(_model_definition_to_dms_model(model), ignore_table_name=ignore_table_name)
    if got != want:
        raise ValidationError(
            "model definition does not match DMS:\n"
            f"want={json.dumps(want, sort_keys=True, separators=(',', ':'))}\n"
            f"got={json.dumps(got, sort_keys=True, separators=(',', ':'))}"
        )


def _assert_json_compatible(value: Any, *, path: str) -> None:
    if value is None or isinstance(value, (str, bool, int, float)):
        if isinstance(value, float) and not (value == value and value not in (float("inf"), float("-inf"))):
            raise ValidationError(f"DMS contains non-finite float at {path}")
        return

    if isinstance(value, list):
        for idx, elem in enumerate(value):
            _assert_json_compatible(elem, path=f"{path}[{idx}]")
        return

    if isinstance(value, dict):
        for k, v in value.items():
            if not isinstance(k, str):
                raise ValidationError(f"DMS contains non-string key at {path}: {k!r}")
            _assert_json_compatible(v, path=f"{path}.{k}")
        return

    raise ValidationError(f"DMS contains non-JSON value at {path}: {type(value).__name__}")


def _normalize_dms_model(model: Mapping[str, Any], *, ignore_table_name: bool) -> dict[str, Any]:
    name = model.get("name")
    if not isinstance(name, str) or not name:
        raise ValidationError("DMS model missing name")

    keys = model.get("keys")
    if not isinstance(keys, dict):
        raise ValidationError(f"DMS model {name}: missing keys")

    partition = keys.get("partition")
    if not isinstance(partition, dict):
        raise ValidationError(f"DMS model {name}: missing keys.partition")

    out: dict[str, Any] = {
        "name": name,
        "keys": {
            "partition": {
                "attribute": cast(str, partition.get("attribute")),
                "type": cast(str, partition.get("type")),
            },
            "sort": None,
        },
        "attributes": [],
        "indexes": [],
    }

    sort = keys.get("sort")
    if isinstance(sort, dict):
        out["keys"] = {
            "partition": out["keys"]["partition"],
            "sort": {
                "attribute": cast(str, sort.get("attribute")),
                "type": cast(str, sort.get("type")),
            },
        }

    if not ignore_table_name:
        table = model.get("table")
        if not isinstance(table, dict):
            raise ValidationError(f"DMS model {name}: missing table")
        out["table"] = {"name": cast(str, table.get("name"))}

    attrs_raw = model.get("attributes")
    if not isinstance(attrs_raw, list):
        raise ValidationError(f"DMS model {name}: missing attributes[]")

    attrs: list[dict[str, Any]] = []
    for attr in attrs_raw:
        if not isinstance(attr, dict):
            raise ValidationError(f"DMS model {name}: attribute must be a map")
        attr_name = attr.get("attribute")
        attr_type = attr.get("type")
        if not isinstance(attr_name, str) or not attr_name:
            raise ValidationError(f"DMS model {name}: attribute missing attribute name")
        if not isinstance(attr_type, str) or not attr_type:
            raise ValidationError(f"DMS model {name}: attribute {attr_name}: missing type")

        roles = attr.get("roles")
        roles_out: list[str] = []
        if isinstance(roles, list):
            roles_out = sorted([r for r in roles if isinstance(r, str) and r])

        json_flag = bool(attr.get("json", False))
        binary_flag = bool(attr.get("binary", False))
        if json_flag and attr_type != "S":
            raise ValidationError(
                f"DMS model {name}: attribute {attr_name}: json requires type S (got {attr_type})"
            )
        if binary_flag and attr_type != "B":
            raise ValidationError(
                f"DMS model {name}: attribute {attr_name}: binary requires type B (got {attr_type})"
            )
        if json_flag and binary_flag:
            raise ValidationError(f"DMS model {name}: attribute {attr_name}: cannot be both json and binary")

        attrs.append(
            {
                "attribute": attr_name,
                "type": attr_type,
                "omit_empty": bool(attr.get("omit_empty", False)),
                "required": bool(attr.get("required", False)),
                "optional": bool(attr.get("optional", False)),
                "roles": roles_out,
                "encrypted": bool(attr.get("encryption") is not None),
                "json": json_flag,
                "binary": binary_flag,
            }
        )

    attrs.sort(key=lambda a: cast(str, a["attribute"]))
    out["attributes"] = attrs

    indexes_raw = model.get("indexes") or []
    if not isinstance(indexes_raw, list):
        raise ValidationError(f"DMS model {name}: indexes must be a list")

    indexes: list[dict[str, Any]] = []
    for idx in indexes_raw:
        if not isinstance(idx, dict):
            raise ValidationError(f"DMS model {name}: index must be a map")
        idx_name = idx.get("name")
        if not isinstance(idx_name, str) or not idx_name:
            raise ValidationError(f"DMS model {name}: index missing name")

        partition_key = idx.get("partition")
        if not isinstance(partition_key, dict):
            raise ValidationError(f"DMS model {name}: index {idx_name}: missing partition")

        sort_key = idx.get("sort")
        sort_out: dict[str, Any] | None = None
        if isinstance(sort_key, dict):
            sort_out = {
                "attribute": cast(str, sort_key.get("attribute")),
                "type": cast(str, sort_key.get("type")),
            }

        proj = idx.get("projection") or {}
        proj_type: str | None = None
        proj_fields: list[str] | None = None
        if isinstance(proj, dict):
            proj_type = cast(str | None, proj.get("type"))
            fields_raw = proj.get("fields")
            if isinstance(fields_raw, list):
                proj_fields = sorted([f for f in fields_raw if isinstance(f, str) and f])

        indexes.append(
            {
                "name": idx_name,
                "type": cast(str, idx.get("type")),
                "partition": {
                    "attribute": cast(str, partition_key.get("attribute")),
                    "type": cast(str, partition_key.get("type")),
                },
                "sort": sort_out,
                "projection": {"type": proj_type, "fields": proj_fields},
            }
        )

    indexes.sort(key=lambda i: cast(str, i["name"]))
    out["indexes"] = indexes
    return out


def _model_definition_to_dms_model(model: ModelDefinition[Any]) -> dict[str, Any]:
    dc_fields = {f.name: f for f in fields(model.model_type)}
    out_attrs: list[dict[str, Any]] = []

    for python_name, attr in model.attributes.items():
        dc_field = dc_fields.get(python_name)
        if dc_field is None:
            continue

        required = dc_field.default is MISSING and dc_field.default_factory is MISSING
        roles = sorted(attr.roles)

        out_attrs.append(
            {
                "attribute": attr.attribute_name,
                "type": _dms_type_for_field(model.model_type, python_name, attr),
                "required": required,
                "optional": not required,
                "omit_empty": attr.omitempty,
                "roles": roles,
                "json": attr.json,
                "binary": attr.binary,
                "encryption": {"v": 1} if attr.encrypted else None,
            }
        )

    out_attrs.sort(key=lambda a: cast(str, a["attribute"]))

    out: dict[str, Any] = {
        "name": model.model_type.__name__,
        "table": {"name": model.table_name or ""},
        "keys": {
            "partition": {"attribute": model.pk.attribute_name, "type": _key_type_for_attr(model, model.pk)},
            "sort": None,
        },
        "attributes": out_attrs,
        "indexes": [],
    }
    if model.sk is not None:
        out["keys"]["sort"] = {
            "attribute": model.sk.attribute_name,
            "type": _key_type_for_attr(model, model.sk),
        }

    for idx in model.indexes:
        out["indexes"].append(
            {
                "name": idx.name,
                "type": idx.type,
                "partition": {
                    "attribute": idx.partition,
                    "type": _scalar_key_type_for_attribute_name(model, idx.partition),
                },
                "sort": (
                    {
                        "attribute": idx.sort,
                        "type": _scalar_key_type_for_attribute_name(model, idx.sort),
                    }
                    if idx.sort is not None
                    else None
                ),
                "projection": {"type": idx.projection.type, "fields": sorted(idx.projection.fields)},
            }
        )

    out["indexes"].sort(key=lambda i: cast(str, i["name"]))
    return out


def _key_type_for_attr(model: ModelDefinition[Any], attr: Any) -> str:
    return _scalar_key_type_for_attribute_name(model, cast(str, attr.attribute_name))


def _scalar_key_type_for_attribute_name(model: ModelDefinition[Any], attribute_name: str) -> str:
    for python_name, attr_def in model.attributes.items():
        if attr_def.attribute_name == attribute_name:
            scalar = _dms_type_for_field(model.model_type, python_name, attr_def)
            if scalar not in {"S", "N", "B"}:
                raise ValidationError(f"key attribute must be S/N/B: {attribute_name} (got {scalar})")
            return scalar
    raise ValidationError(f"unknown attribute: {attribute_name}")


def _dms_type_for_field(model_type: type[Any], field_name: str, attr_def: Any) -> str:
    try:
        annotation = get_type_hints(model_type, include_extras=True).get(field_name, Any)
    except Exception:
        annotation = getattr(model_type, "__annotations__", {}).get(field_name, Any)
    annotation = _unwrap_optional(annotation)

    if getattr(attr_def, "set", False):
        origin = get_origin(annotation)
        if origin is set:
            (elem_type,) = get_args(annotation) or (Any,)
        else:
            elem_type = Any
        elem_type = _unwrap_optional(elem_type)

        if elem_type is str:
            return "SS"
        if elem_type in {int, float, Decimal}:
            return "NS"
        if elem_type in {bytes, bytearray}:
            return "BS"
        raise ValidationError(f"unsupported set element type: {field_name} ({elem_type})")

    if getattr(attr_def, "json", False):
        return "S"

    if getattr(attr_def, "binary", False) or annotation in {bytes, bytearray}:
        return "B"

    if annotation is str:
        return "S"
    if annotation in {int, float, Decimal}:
        return "N"
    if annotation is bool:
        return "BOOL"

    origin = get_origin(annotation)
    if origin in {dict, Mapping}:
        return "M"
    if origin in {list, Sequence, tuple}:
        return "L"

    # Default to string to avoid footguns in schema conversion.
    return "S"


def _unwrap_optional(annotation: Any) -> Any:
    origin = get_origin(annotation)
    if origin is not Union:
        return annotation
    args = get_args(annotation)
    if not args:
        return annotation
    non_none = [a for a in args if a is not type(None)]  # noqa: E721
    if len(args) == 2 and len(non_none) == 1:
        return non_none[0]
    return annotation
