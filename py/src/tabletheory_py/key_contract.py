from __future__ import annotations

import json
import math
from decimal import Decimal
from pathlib import Path
from typing import Any, cast

import yaml

from .errors import ValidationError

KeyContractInputValue = str | int | float | bool

TRANSFORM_TRIM = "trim"
TRANSFORM_WILDCARD_EMPTY = "wildcard_empty"
TRANSFORM_LOWERCASE = "lowercase"
TRANSFORM_URL_ENCODE = "url_encode"

SUPPORTED_CONTRACT_VERSIONS = {"0.1", "0.2"}
SUPPORTED_TRANSFORMS = {
    TRANSFORM_TRIM,
    TRANSFORM_WILDCARD_EMPTY,
    TRANSFORM_LOWERCASE,
    TRANSFORM_URL_ENCODE,
}


def parse_derived_key_contract(raw: str) -> dict[str, Any]:
    try:
        parsed = yaml.safe_load(raw)
    except Exception as err:
        raise ValidationError("invalid key contract YAML/JSON") from err
    validate_derived_key_contract(parsed)
    return cast(dict[str, Any], parsed)


def load_key_contract_file(path: str | Path) -> dict[str, Any]:
    return parse_derived_key_contract(Path(path).read_text(encoding="utf-8"))


def evaluate_derived_key(
    contract: dict[str, Any],
    name: str,
    input: dict[str, KeyContractInputValue] | None = None,
) -> str:
    validate_derived_key_contract(contract)
    for key in contract["derived_keys"]:
        if key["name"] == name:
            return evaluate_derived_key_definition(key, input or {})
    raise ValidationError(f"derived key not found: {name}")


def evaluate_derived_key_definition(
    key: dict[str, Any],
    input: dict[str, KeyContractInputValue] | None = None,
) -> str:
    validate_derived_key_definition(key)
    values = input or {}
    input_types = {
        input_def["name"]: input_def.get("type", "")
        for input_def in key.get("inputs", [])
        if isinstance(input_def, dict) and isinstance(input_def.get("name"), str)
    }
    parts: list[str] = []
    for index, segment in enumerate(key["segments"]):
        evaluated_value, omit = _evaluate_segment(key["name"], index, segment, values, input_types)
        if not omit:
            parts.append(f"{segment.get('prefix', '')}{evaluated_value}{segment.get('suffix', '')}")
    return str(key.get("join", "")).join(parts)


def verify_derived_key_fixtures(contract: dict[str, Any]) -> None:
    validate_derived_key_contract(contract)
    for key in contract["derived_keys"]:
        for fixture in key.get("fixtures", []):
            got = evaluate_derived_key_definition(key, fixture["input"])
            if got != fixture["expect"]:
                raise ValidationError(
                    f"derived key {key['name']} fixture {fixture['name']}: "
                    f"expected {json.dumps(fixture['expect'])}, got {json.dumps(got)}"
                )


def validate_derived_key_contract(value: Any) -> None:
    if not isinstance(value, dict):
        raise ValidationError("key contract document must be an object")
    version = value.get("tabletheory_model_contract_version")
    if version not in SUPPORTED_CONTRACT_VERSIONS:
        raise ValidationError(f"unsupported tabletheory_model_contract_version: {version!r}")
    derived_keys = value.get("derived_keys")
    if not isinstance(derived_keys, list) or not derived_keys:
        raise ValidationError("key contract must include derived_keys[]")

    names: set[str] = set()
    for key in derived_keys:
        validate_derived_key_definition(key)
        key_name = key["name"]
        if key_name in names:
            raise ValidationError(f"duplicate derived key: {key_name}")
        names.add(key_name)

    models = value.get("models")
    if models is None:
        return
    if not isinstance(models, list):
        raise ValidationError("key contract models must be an array")
    for model in models:
        if not isinstance(model, dict) or not isinstance(model.get("name"), str) or not model["name"]:
            raise ValidationError("model contract missing name")
        derived = model.get("derived_keys")
        if derived is None:
            continue
        if not isinstance(derived, list):
            raise ValidationError(f"model {model['name']} derived_keys must be an array")
        for key_name in derived:
            if not isinstance(key_name, str) or key_name not in names:
                raise ValidationError(f"model {model['name']} references unknown derived key: {key_name!r}")


def validate_derived_key_definition(value: Any) -> None:
    if not isinstance(value, dict) or not isinstance(value.get("name"), str) or not value["name"]:
        raise ValidationError("derived key missing name")
    if not isinstance(value.get("join"), str):
        raise ValidationError(f"derived key {value['name']}: join must be a string")
    segments = value.get("segments")
    if not isinstance(segments, list) or not segments:
        raise ValidationError(f"derived key {value['name']}: missing segments[]")

    input_names: set[str] = set()
    inputs = value.get("inputs", [])
    if inputs is None:
        inputs = []
    if not isinstance(inputs, list):
        raise ValidationError(f"derived key {value['name']}: inputs must be an array")
    for input_def in inputs:
        if (
            not isinstance(input_def, dict)
            or not isinstance(input_def.get("name"), str)
            or not input_def["name"]
        ):
            raise ValidationError(f"derived key {value['name']}: input missing name")
        input_name = input_def["name"]
        if input_name in input_names:
            raise ValidationError(f"derived key {value['name']}: duplicate input {input_name}")
        input_names.add(input_name)

    for index, segment in enumerate(segments):
        _validate_segment(value["name"], index, segment, input_names)

    fixtures = value.get("fixtures", [])
    if fixtures is None:
        fixtures = []
    if not isinstance(fixtures, list):
        raise ValidationError(f"derived key {value['name']}: fixtures must be an array")
    fixture_names: set[str] = set()
    for fixture in fixtures:
        if not isinstance(fixture, dict) or not isinstance(fixture.get("name"), str) or not fixture["name"]:
            raise ValidationError(f"derived key {value['name']}: fixture missing name")
        fixture_name = fixture["name"]
        if fixture_name in fixture_names:
            raise ValidationError(f"derived key {value['name']}: duplicate fixture {fixture_name}")
        fixture_names.add(fixture_name)
        if not isinstance(fixture.get("input"), dict):
            raise ValidationError(f"derived key {value['name']} fixture {fixture_name}: missing input")
        if not isinstance(fixture.get("expect"), str):
            raise ValidationError(
                f"derived key {value['name']} fixture {fixture_name}: expect must be a string"
            )


def _validate_segment(key_name: str, index: int, value: Any, input_names: set[str]) -> None:
    label = (
        value.get("name")
        if isinstance(value, dict) and isinstance(value.get("name"), str)
        else f"#{index + 1}"
    )
    if not isinstance(value, dict):
        raise ValidationError(f"derived key {key_name} segment {label}: segment must be an object")
    segment_value = value.get("value")
    source_count = (
        (1 if isinstance(value.get("literal"), str) else 0)
        + (
            1
            if isinstance(segment_value, dict)
            and isinstance(segment_value.get("input"), str)
            and segment_value.get("input") != ""
            else 0
        )
        + (1 if isinstance(segment_value, dict) and isinstance(segment_value.get("literal"), str) else 0)
    )
    if source_count != 1:
        raise ValidationError(f"derived key {key_name} segment {label}: exactly one value source is required")

    input_name = segment_value.get("input") if isinstance(segment_value, dict) else None
    if isinstance(input_name, str) and input_name and input_names and input_name not in input_names:
        raise ValidationError(f"derived key {key_name} segment {label}: unknown input {input_name}")

    transforms = value.get("transforms", [])
    if transforms is None:
        transforms = []
    if not isinstance(transforms, list):
        raise ValidationError(f"derived key {key_name} segment {label}: transforms must be an array")
    for transform in transforms:
        if transform not in SUPPORTED_TRANSFORMS:
            raise ValidationError(
                f"derived key {key_name} segment {label}: unsupported transform {transform!r}"
            )


def _evaluate_segment(
    key_name: str,
    index: int,
    segment: dict[str, Any],
    input_values: dict[str, KeyContractInputValue],
    input_types: dict[str, Any],
) -> tuple[str, bool]:
    label = (
        segment.get("name")
        if isinstance(segment.get("name"), str) and segment.get("name")
        else f"#{index + 1}"
    )
    value, present, from_input = _resolve_segment_value(
        segment, input_values, input_types, key_name, str(label)
    )
    if not present and segment.get("optional") is True and segment.get("default") is None:
        return "", True

    value, generated_wildcard, percent_encoded = _apply_transforms(value, segment.get("transforms") or [])
    if from_input and not generated_wildcard and not percent_encoded:
        value = _url_encode(value)
    if value == "" and segment.get("default") is not None:
        value = str(segment["default"])

    if _should_omit_segment(value, segment):
        return "", True
    if value == "" and segment.get("optional") is True and segment.get("default") is None:
        return "", True
    return value, False


def _resolve_segment_value(
    segment: dict[str, Any],
    input_values: dict[str, KeyContractInputValue],
    input_types: dict[str, Any],
    key_name: str,
    label: str,
) -> tuple[str, bool, bool]:
    if isinstance(segment.get("literal"), str):
        return segment["literal"], True, False
    value = segment.get("value")
    if isinstance(value, dict) and isinstance(value.get("literal"), str):
        return value["literal"], True, False
    if isinstance(value, dict) and isinstance(value.get("input"), str) and value["input"]:
        input_name = value["input"]
        if input_name not in input_values:
            if segment.get("default") is not None or segment.get("optional") is True:
                return "", False, False
            raise ValidationError(
                f"derived key {key_name} segment {label}: missing required input {input_name!r}"
            )
        return (
            _scalar_to_string(input_values[input_name], input_name, str(input_types.get(input_name, ""))),
            True,
            True,
        )
    raise ValidationError(f"derived key {key_name} segment {label}: missing value source")


def _apply_transforms(value: str, transforms: list[str]) -> tuple[str, bool, bool]:
    generated_wildcard = False
    percent_encoded = False
    for transform in transforms:
        if transform == TRANSFORM_TRIM:
            value = _trim_contract_whitespace(value)
        elif transform == TRANSFORM_WILDCARD_EMPTY:
            if value == "":
                value = "*"
                generated_wildcard = True
        elif transform == TRANSFORM_LOWERCASE:
            value = _ascii_lowercase(value)
        elif transform == TRANSFORM_URL_ENCODE:
            value = _url_encode(value)
            percent_encoded = True
        else:
            raise ValidationError(f"unsupported transform {transform!r}")
    return value, generated_wildcard, percent_encoded


def _should_omit_segment(value: str, segment: dict[str, Any]) -> bool:
    omit_when = segment.get("omit_when")
    if not isinstance(omit_when, dict):
        return False
    if omit_when.get("empty") is True and value == "":
        return True
    if (
        omit_when.get("default") is True
        and segment.get("default") is not None
        and value == str(segment["default"])
    ):
        return True
    values = omit_when.get("values", [])
    return isinstance(values, list) and any(value == candidate for candidate in values)


def _scalar_to_string(value: Any, input_name: str, input_type: str = "") -> str:
    if value is None:
        raise ValidationError(f"derived key input {input_name} must not be null")
    if isinstance(value, str):
        if input_type == "number":
            try:
                decimal = Decimal(value)
            except Exception as err:
                raise ValidationError(f"derived key input {input_name} must be a finite number") from err
            if not decimal.is_finite():
                raise ValidationError(f"derived key input {input_name} must be a finite number")
            if decimal == 0:
                return "0"
            return format(decimal, "f")
        return value
    if isinstance(value, bool):
        return "true" if value else "false"
    if isinstance(value, int):
        return str(value)
    if isinstance(value, float):
        if not math.isfinite(value):
            raise ValidationError(f"derived key input {input_name} must be a finite number")
        if value == 0:
            return "0"
        return _expand_exponent_decimal(str(value))
    if isinstance(value, Decimal):
        if not value.is_finite():
            raise ValidationError(f"derived key input {input_name} must be a finite number")
        if value == 0:
            return "0"
        return format(value, "f")
    raise ValidationError(f"derived key input {input_name} must be a scalar")


_UNRESERVED_BYTES = set(b"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~")


def _url_encode(value: str) -> str:
    out: list[str] = []
    for byte in value.encode("utf-8"):
        if byte in _UNRESERVED_BYTES:
            out.append(chr(byte))
        else:
            out.append(f"%{byte:02X}")
    return "".join(out)


def _ascii_lowercase(value: str) -> str:
    return "".join(chr(ord(ch) + 0x20) if "A" <= ch <= "Z" else ch for ch in value)


_TRIM_CODEPOINTS = {
    0x0009,
    0x000A,
    0x000B,
    0x000C,
    0x000D,
    0x0020,
    0x0085,
    0x00A0,
    0x1680,
    0x2000,
    0x2001,
    0x2002,
    0x2003,
    0x2004,
    0x2005,
    0x2006,
    0x2007,
    0x2008,
    0x2009,
    0x200A,
    0x2028,
    0x2029,
    0x202F,
    0x205F,
    0x3000,
    0xFEFF,
}


def _trim_contract_whitespace(value: str) -> str:
    start = 0
    while start < len(value) and ord(value[start]) in _TRIM_CODEPOINTS:
        start += 1
    end = len(value)
    while end > start and ord(value[end - 1]) in _TRIM_CODEPOINTS:
        end -= 1
    return value[start:end]


def _expand_exponent_decimal(value: str) -> str:
    if "e" not in value and "E" not in value:
        return value
    return format(Decimal(value), "f")
