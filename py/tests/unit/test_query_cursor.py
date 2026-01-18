from __future__ import annotations

import base64
import json

import pytest

from theorydb_py.query import decode_cursor, encode_cursor


def test_cursor_round_trip_with_bytes_and_nested_structures() -> None:
    key = {
        "PK": {"S": "A"},
        "SK": {"S": "B"},
        "blob": {"B": b"hi"},
        "nested": {"M": {"x": {"B": b"bye"}}},
        "list": {"L": [{"B": b"x"}, {"S": "y"}]},
    }
    cursor = encode_cursor(key, index="gsi-email", sort="ASC")
    decoded = decode_cursor(cursor)
    assert decoded.last_key == key
    assert decoded.index == "gsi-email"
    assert decoded.sort == "ASC"


def test_cursor_round_trip_all_attribute_value_types() -> None:
    key = {
        "PK": {"S": "A"},
        "N": {"N": "123"},
        "flag": {"BOOL": True},
        "nil": {"NULL": True},
        "ss": {"SS": ["a", "b"]},
        "ns": {"NS": ["1", "2"]},
        "bs": {"BS": [b"x", b"y"]},
        "list": {"L": [{"S": "z"}, {"N": "9"}]},
        "map": {"M": {"x": {"S": "y"}}},
    }

    cursor = encode_cursor(key, index="gsi", sort="DESC")
    decoded = decode_cursor(cursor)
    assert decoded.last_key == key
    assert decoded.index == "gsi"
    assert decoded.sort == "DESC"


def test_decode_cursor_invalid_base64_raises() -> None:
    with pytest.raises(ValueError):
        decode_cursor("bm90LWpzb24")  # base64url("not-json")


def test_decode_cursor_empty_raises() -> None:
    with pytest.raises(ValueError, match="cursor is empty"):
        decode_cursor("")


def test_encode_cursor_empty_returns_empty_string() -> None:
    assert encode_cursor({}) == ""


def test_encode_cursor_rejects_non_map() -> None:
    with pytest.raises(ValueError, match="last_key must be a map"):
        encode_cursor(["not-a-map"])  # type: ignore[arg-type]


@pytest.mark.parametrize(
    "av",
    [
        {"S": 1},
        {"N": 1},
        {"B": "not-bytes"},
        {"BOOL": "true"},
        {"NULL": False},
        {"SS": ["a", 1]},
        {"NS": ["1", 2]},
        {"BS": ["x"]},
        {"L": "not-list"},
        {"M": "not-map"},
        {"Z": "nope"},
        {"S": "x", "N": "1"},
        "not-a-map",
    ],
)
def test_encode_cursor_rejects_invalid_attribute_values(av: object) -> None:
    with pytest.raises(ValueError):
        encode_cursor({"PK": av})


def test_decode_cursor_rejects_non_object_json() -> None:
    payload = json.dumps(["nope"]).encode("utf-8")
    cursor = base64.urlsafe_b64encode(payload).decode("ascii")
    with pytest.raises(ValueError, match="cursor must decode to an object"):
        decode_cursor(cursor)


def test_decode_cursor_rejects_invalid_last_key_shape() -> None:
    payload = json.dumps({"lastKey": 1}).encode("utf-8")
    cursor = base64.urlsafe_b64encode(payload).decode("ascii")
    with pytest.raises(ValueError, match="cursor lastKey is invalid"):
        decode_cursor(cursor)


@pytest.mark.parametrize(
    "av_json",
    [
        {"S": 1},
        {"N": 1},
        {"B": 123},
        {"BOOL": "nope"},
        {"BS": [123]},
        {"SS": ["a", 1]},
        {"NS": ["1", 2]},
        {"L": "nope"},
        {"M": "nope"},
        {"NULL": "nope"},
        {"Z": "nope"},
        {"S": "x", "N": "1"},
        "nope",
    ],
)
def test_decode_cursor_rejects_invalid_attribute_values(av_json: object) -> None:
    payload = json.dumps({"lastKey": {"PK": av_json}}).encode("utf-8")
    cursor = base64.urlsafe_b64encode(payload).decode("ascii")
    with pytest.raises(ValueError):
        decode_cursor(cursor)


def test_decode_cursor_coerces_index_and_sort_fields() -> None:
    key = {"PK": {"S": "A"}}
    payload = json.dumps({"lastKey": key, "index": 1, "sort": "NOPE"}).encode("utf-8")
    cursor = base64.urlsafe_b64encode(payload).decode("ascii")
    decoded = decode_cursor(cursor)
    assert decoded.last_key == key
    assert decoded.index is None
    assert decoded.sort is None
