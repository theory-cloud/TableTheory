from __future__ import annotations

from dataclasses import dataclass
from decimal import Decimal
from typing import Any

import pytest

from tabletheory_py.attr_types import (
    _normalize_json_compatible,
    _parse_json_text,
    _to_json_text,
    _validate_json_value_matches_storage,
    decode_json_field_from_storage,
    infer_json_storage_type,
    infer_storage_type,
    normalize_json_field_for_storage,
    resolve_attribute_storage_type,
    unwrap_optional,
    validate_json_storage_type,
)
from tabletheory_py.errors import ValidationError


def test_unwrap_optional_handles_optional_and_general_unions() -> None:
    assert unwrap_optional(int | None) is int
    assert unwrap_optional(str) is str

    with pytest.raises(ValidationError, match="unsupported union annotation"):
        unwrap_optional(int | str)

    with pytest.raises(ValidationError, match="unsupported union annotation"):
        unwrap_optional(int | str | None)


def test_infer_storage_type_covers_set_binary_json_and_scalar_variants() -> None:
    assert infer_storage_type(set[str], is_set=True, is_json=False, is_binary=False) == "SS"
    assert infer_storage_type(set[Decimal], is_set=True, is_json=False, is_binary=False) == "NS"
    assert infer_storage_type(set[bytes], is_set=True, is_json=False, is_binary=False) == "BS"

    with pytest.raises(ValidationError, match="unsupported set element type"):
        infer_storage_type(set[complex], is_set=True, is_json=False, is_binary=False)

    assert infer_storage_type(bytes, is_set=False, is_json=False, is_binary=False) == "B"
    assert infer_storage_type(dict[str, int], is_set=False, is_json=True, is_binary=False) == "M"
    assert infer_storage_type(list[int], is_set=False, is_json=False, is_binary=False) == "L"
    assert infer_storage_type(bool, is_set=False, is_json=False, is_binary=False) == "BOOL"
    assert infer_storage_type(str, is_set=False, is_json=False, is_binary=False) == "S"


def test_infer_storage_type_rejects_unsupported_unions() -> None:
    with pytest.raises(ValidationError, match="unsupported union annotation"):
        infer_storage_type(int | str | None, is_set=False, is_json=False, is_binary=False)


def test_resolve_attribute_storage_type_uses_shared_fallbacks() -> None:
    @dataclass(frozen=True)
    class Attr:
        storage_type: str | None = None
        json: bool = False
        binary: bool = False
        set: bool = False

    assert resolve_attribute_storage_type(Attr(storage_type="M", json=True)) == "M"
    assert resolve_attribute_storage_type(Attr(json=True)) == "S"
    assert resolve_attribute_storage_type(Attr(binary=True)) == "B"
    assert resolve_attribute_storage_type(Attr(set=True)) == "SS"
    assert resolve_attribute_storage_type(Attr()) == "S"


def test_infer_json_storage_type_covers_branches() -> None:
    assert infer_json_storage_type(str) == "S"
    assert infer_json_storage_type(bytes) == "S"
    assert infer_json_storage_type(Any) == "S"
    assert infer_json_storage_type(int) == "N"
    assert infer_json_storage_type(bool) == "BOOL"
    assert infer_json_storage_type(type(None)) == "NULL"
    assert infer_json_storage_type(dict[str, int]) == "M"
    assert infer_json_storage_type(list[int]) == "L"
    assert infer_json_storage_type(complex) == "S"


def test_validate_json_storage_type_rejects_invalid_values() -> None:
    validate_json_storage_type("M", field_name="payload")

    with pytest.raises(ValidationError, match="json fields must use"):
        validate_json_storage_type("SS", field_name="payload")


def test_normalize_json_field_for_storage_covers_string_and_structured_variants() -> None:
    assert normalize_json_field_for_storage(None, storage_type="S", field_name="payload") is None
    assert (
        normalize_json_field_for_storage({"b": 2, "a": 1}, storage_type="S", field_name="payload")
        == '{"a":1,"b":2}'
    )
    assert normalize_json_field_for_storage('{"count":2}', storage_type="M", field_name="payload") == {
        "count": 2
    }
    assert normalize_json_field_for_storage(Decimal("2"), storage_type="N", field_name="payload") == 2

    with pytest.raises(ValidationError, match="requires a boolean value"):
        normalize_json_field_for_storage("1", storage_type="BOOL", field_name="payload")


def test_decode_json_field_from_storage_covers_string_and_native_variants() -> None:
    assert decode_json_field_from_storage(None, storage_type="S", field_name="payload") is None
    assert (
        decode_json_field_from_storage('{"kept":true}', storage_type="S", field_name="payload")
        == '{"kept":true}'
    )
    assert (
        decode_json_field_from_storage({"b": 2, "a": 1}, storage_type="S", field_name="payload")
        == '{"a":1,"b":2}'
    )
    assert decode_json_field_from_storage("[1,2]", storage_type="L", field_name="payload") == [1, 2]
    assert decode_json_field_from_storage({"count": 2}, storage_type="M", field_name="payload") == {
        "count": 2
    }

    with pytest.raises(ValidationError, match="requires an object value"):
        decode_json_field_from_storage([1, 2], storage_type="M", field_name="payload")


def test_json_text_helpers_cover_utf8_and_parse_errors() -> None:
    assert _to_json_text(b'{"count":2}', field_name="payload") == '{"count":2}'

    with pytest.raises(ValidationError, match="invalid utf-8 JSON text"):
        _to_json_text(b"\xff", field_name="payload")

    assert _parse_json_text(b'{"count":2}', field_name="payload") == {"count": 2}

    with pytest.raises(ValidationError, match="invalid utf-8 JSON text"):
        _parse_json_text(b"\xff", field_name="payload")

    with pytest.raises(ValidationError, match="invalid JSON value"):
        _parse_json_text("not-json", field_name="payload")


def test_normalize_json_compatible_handles_dataclasses_mappings_sequences_and_errors() -> None:
    @dataclass
    class Demo:
        count: int
        ratio: Decimal

    assert _normalize_json_compatible(Demo(count=2, ratio=Decimal("1.5")), path="payload") == {
        "count": 2,
        "ratio": 1.5,
    }
    assert _normalize_json_compatible((1, 2, 3), path="payload") == [1, 2, 3]
    assert _normalize_json_compatible(Decimal("5"), path="payload") == 5

    with pytest.raises(ValidationError, match="non-finite JSON number"):
        _normalize_json_compatible(float("inf"), path="payload")

    with pytest.raises(ValidationError, match="non-finite JSON number"):
        _normalize_json_compatible(Decimal("NaN"), path="payload")

    with pytest.raises(ValidationError, match="non-JSON value"):
        _normalize_json_compatible(b"abc", path="payload")

    with pytest.raises(ValidationError, match="non-string JSON key"):
        _normalize_json_compatible({1: "bad"}, path="payload")

    with pytest.raises(ValidationError, match="non-JSON value"):
        _normalize_json_compatible(object(), path="payload")


@pytest.mark.parametrize(
    ("storage_type", "value", "message"),
    [
        ("N", True, "requires a numeric value"),
        ("BOOL", 1, "requires a boolean value"),
        ("NULL", 1, "requires null"),
        ("L", {"a": 1}, "requires a list value"),
        ("M", [1], "requires an object value"),
    ],
)
def test_validate_json_value_matches_storage_rejects_mismatches(
    storage_type: str, value: object, message: str
) -> None:
    with pytest.raises(ValidationError, match=message):
        _validate_json_value_matches_storage(value, storage_type=storage_type, field_name="payload")
