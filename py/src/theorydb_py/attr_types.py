from __future__ import annotations

import json
import math
from collections.abc import Mapping, Sequence
from dataclasses import is_dataclass
from decimal import Decimal
from typing import Any, Union, get_args, get_origin

from .errors import ValidationError

JSON_STORAGE_TYPES = {"S", "N", "BOOL", "NULL", "L", "M"}


def unwrap_optional(annotation: Any) -> Any:
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


def infer_storage_type(annotation: Any, *, is_set: bool, is_json: bool, is_binary: bool) -> str:
    annotation = unwrap_optional(annotation)

    if is_set:
        origin = get_origin(annotation)
        if origin is set:
            (elem_type,) = get_args(annotation) or (Any,)
        else:
            elem_type = Any
        elem_type = unwrap_optional(elem_type)

        if elem_type is str:
            return "SS"
        if elem_type in {int, float, Decimal}:
            return "NS"
        if elem_type in {bytes, bytearray}:
            return "BS"
        raise ValidationError(f"unsupported set element type: {elem_type}")

    if is_binary or annotation in {bytes, bytearray}:
        return "B"

    if is_json:
        return infer_json_storage_type(annotation)

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

    return "S"


def infer_json_storage_type(annotation: Any) -> str:
    annotation = unwrap_optional(annotation)

    if annotation in {str, bytes, bytearray, Any}:
        return "S"
    if annotation in {int, float, Decimal}:
        return "N"
    if annotation is bool:
        return "BOOL"
    if annotation is type(None):  # noqa: E721
        return "NULL"

    origin = get_origin(annotation)
    if origin in {dict, Mapping}:
        return "M"
    if origin in {list, Sequence, tuple}:
        return "L"

    return "S"


def validate_json_storage_type(storage_type: str, *, field_name: str) -> None:
    if storage_type not in JSON_STORAGE_TYPES:
        raise ValidationError(
            f"json fields must use S/N/BOOL/NULL/L/M storage: {field_name} (got {storage_type})"
        )


def normalize_json_field_for_storage(value: Any, *, storage_type: str, field_name: str) -> Any:
    if value is None:
        return None

    validate_json_storage_type(storage_type, field_name=field_name)

    if storage_type == "S":
        return _to_json_text(value, field_name=field_name)

    if isinstance(value, (str, bytes, bytearray)):
        normalized = _parse_json_text(value, field_name=field_name)
    else:
        normalized = _normalize_json_compatible(value, path=field_name)

    _validate_json_value_matches_storage(normalized, storage_type=storage_type, field_name=field_name)
    return normalized


def decode_json_field_from_storage(raw: Any, *, storage_type: str, field_name: str) -> Any:
    if raw is None:
        return None

    validate_json_storage_type(storage_type, field_name=field_name)

    if storage_type == "S":
        if isinstance(raw, str):
            return raw
        return json.dumps(
            _normalize_json_compatible(raw, path=field_name), separators=(",", ":"), sort_keys=True
        )

    if isinstance(raw, str):
        normalized = _parse_json_text(raw, field_name=field_name)
    else:
        normalized = _normalize_json_compatible(raw, path=field_name)

    _validate_json_value_matches_storage(normalized, storage_type=storage_type, field_name=field_name)
    return normalized


def _to_json_text(value: Any, *, field_name: str) -> str:
    if isinstance(value, str):
        return value
    if isinstance(value, (bytes, bytearray)):
        try:
            return bytes(value).decode("utf-8")
        except UnicodeDecodeError as err:
            raise ValidationError(f"invalid utf-8 JSON text for {field_name}") from err
    return json.dumps(
        _normalize_json_compatible(value, path=field_name), separators=(",", ":"), sort_keys=True
    )


def _parse_json_text(value: str | bytes | bytearray, *, field_name: str) -> Any:
    if isinstance(value, (bytes, bytearray)):
        try:
            text = bytes(value).decode("utf-8")
        except UnicodeDecodeError as err:
            raise ValidationError(f"invalid utf-8 JSON text for {field_name}") from err
    else:
        text = value

    try:
        parsed = json.loads(text)
    except Exception as err:
        raise ValidationError(f"invalid JSON value for {field_name}") from err

    return _normalize_json_compatible(parsed, path=field_name)


def _normalize_json_compatible(value: Any, *, path: str) -> Any:
    if value is None or isinstance(value, (str, bool, int)):
        return value

    if isinstance(value, float):
        if not math.isfinite(value):
            raise ValidationError(f"non-finite JSON number at {path}")
        return value

    if isinstance(value, Decimal):
        if not value.is_finite():
            raise ValidationError(f"non-finite JSON number at {path}")
        if value == value.to_integral_value():
            return int(value)
        return float(value)

    if isinstance(value, (bytes, bytearray)):
        raise ValidationError(f"non-JSON value at {path}: {type(value).__name__}")

    if is_dataclass(value):
        value = value.__dict__

    if isinstance(value, Mapping):
        out: dict[str, Any] = {}
        for key in sorted(value.keys()):
            if not isinstance(key, str):
                raise ValidationError(f"non-string JSON key at {path}: {key!r}")
            out[key] = _normalize_json_compatible(value[key], path=f"{path}.{key}")
        return out

    if isinstance(value, Sequence) and not isinstance(value, (str, bytes, bytearray)):
        return [_normalize_json_compatible(elem, path=f"{path}[]") for elem in value]

    raise ValidationError(f"non-JSON value at {path}: {type(value).__name__}")


def _validate_json_value_matches_storage(value: Any, *, storage_type: str, field_name: str) -> None:
    if value is None:
        return

    if storage_type == "N":
        if isinstance(value, bool) or not isinstance(value, (int, float)):
            raise ValidationError(f"json field {field_name} requires a numeric value")
        return

    if storage_type == "BOOL":
        if not isinstance(value, bool):
            raise ValidationError(f"json field {field_name} requires a boolean value")
        return

    if storage_type == "NULL":
        if value is not None:
            raise ValidationError(f"json field {field_name} requires null")
        return

    if storage_type == "L":
        if not isinstance(value, list):
            raise ValidationError(f"json field {field_name} requires a list value")
        return

    if storage_type == "M":
        if not isinstance(value, dict):
            raise ValidationError(f"json field {field_name} requires an object value")
        return
