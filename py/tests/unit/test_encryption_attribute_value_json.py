from __future__ import annotations

import base64

import pytest

from theorydb_py.encryption import marshal_attribute_value_json, unmarshal_attribute_value_json
from theorydb_py.errors import ValidationError


def test_marshal_attribute_value_json_supported_types() -> None:
    assert marshal_attribute_value_json({"S": "x"}) == {"t": "S", "s": "x"}
    assert marshal_attribute_value_json({"N": "1"}) == {"t": "N", "n": "1"}
    assert marshal_attribute_value_json({"BOOL": True}) == {"t": "BOOL", "bool": True}
    assert marshal_attribute_value_json({"NULL": True}) == {"t": "NULL", "null": True}
    assert marshal_attribute_value_json({"SS": ["a", "b"]}) == {"t": "SS", "ss": ["a", "b"]}
    assert marshal_attribute_value_json({"NS": ["1", "2"]}) == {"t": "NS", "ns": ["1", "2"]}

    assert marshal_attribute_value_json({"B": b"hi"}) == {
        "t": "B",
        "b": base64.b64encode(b"hi").decode("ascii"),
    }
    assert marshal_attribute_value_json({"BS": [b"a", b"b"]}) == {
        "t": "BS",
        "bs": [base64.b64encode(b"a").decode("ascii"), base64.b64encode(b"b").decode("ascii")],
    }

    assert marshal_attribute_value_json({"L": [{"S": "x"}, {"N": "1"}]}) == {
        "t": "L",
        "l": [{"t": "S", "s": "x"}, {"t": "N", "n": "1"}],
    }
    assert marshal_attribute_value_json({"M": {"a": {"S": "x"}}}) == {
        "t": "M",
        "m": {"a": {"t": "S", "s": "x"}},
    }


def test_marshal_attribute_value_json_validation_errors() -> None:
    with pytest.raises(ValidationError):
        marshal_attribute_value_json("not-a-map")
    with pytest.raises(ValidationError):
        marshal_attribute_value_json({"S": "x", "N": "1"})

    with pytest.raises(ValidationError):
        marshal_attribute_value_json({"S": 1})
    with pytest.raises(ValidationError):
        marshal_attribute_value_json({"N": 1})
    with pytest.raises(ValidationError):
        marshal_attribute_value_json({"B": "not-bytes"})
    with pytest.raises(ValidationError):
        marshal_attribute_value_json({"BOOL": "true"})
    with pytest.raises(ValidationError):
        marshal_attribute_value_json({"NULL": False})
    with pytest.raises(ValidationError):
        marshal_attribute_value_json({"SS": ["a", 1]})
    with pytest.raises(ValidationError):
        marshal_attribute_value_json({"NS": [1]})
    with pytest.raises(ValidationError):
        marshal_attribute_value_json({"BS": ["not-bytes"]})
    with pytest.raises(ValidationError):
        marshal_attribute_value_json({"L": "not-a-list"})
    with pytest.raises(ValidationError):
        marshal_attribute_value_json({"M": "not-a-map"})
    with pytest.raises(ValidationError):
        marshal_attribute_value_json({"X": "nope"})


def test_unmarshal_attribute_value_json_supported_types() -> None:
    assert unmarshal_attribute_value_json({"t": "S", "s": "x"}) == {"S": "x"}
    assert unmarshal_attribute_value_json({"t": "N", "n": "1"}) == {"N": "1"}
    assert unmarshal_attribute_value_json({"t": "BOOL", "bool": False}) == {"BOOL": False}
    assert unmarshal_attribute_value_json({"t": "NULL", "null": True}) == {"NULL": True}
    assert unmarshal_attribute_value_json({"t": "SS", "ss": ["a"]}) == {"SS": ["a"]}
    assert unmarshal_attribute_value_json({"t": "NS", "ns": ["1"]}) == {"NS": ["1"]}

    b64 = base64.b64encode(b"hi").decode("ascii")
    assert unmarshal_attribute_value_json({"t": "B", "b": b64}) == {"B": b"hi"}
    assert unmarshal_attribute_value_json({"t": "BS", "bs": [b64]}) == {"BS": [b"hi"]}

    assert unmarshal_attribute_value_json({"t": "L", "l": [{"t": "S", "s": "x"}]}) == {"L": [{"S": "x"}]}
    assert unmarshal_attribute_value_json({"t": "M", "m": {"a": {"t": "N", "n": "1"}}}) == {
        "M": {"a": {"N": "1"}}
    }


def test_unmarshal_attribute_value_json_validation_errors() -> None:
    with pytest.raises(ValidationError):
        unmarshal_attribute_value_json("not-a-map")
    with pytest.raises(ValidationError):
        unmarshal_attribute_value_json({"S": "x"})

    with pytest.raises(ValidationError):
        unmarshal_attribute_value_json({"t": "S", "s": 1})
    with pytest.raises(ValidationError):
        unmarshal_attribute_value_json({"t": "N", "n": 1})
    with pytest.raises(ValidationError):
        unmarshal_attribute_value_json({"t": "BOOL", "bool": "true"})
    with pytest.raises(ValidationError):
        unmarshal_attribute_value_json({"t": "NULL", "null": False})
    with pytest.raises(ValidationError):
        unmarshal_attribute_value_json({"t": "SS", "ss": ["a", 1]})
    with pytest.raises(ValidationError):
        unmarshal_attribute_value_json({"t": "NS", "ns": [1]})
    with pytest.raises(ValidationError):
        unmarshal_attribute_value_json({"t": "B", "b": "not-base64"})
    with pytest.raises(ValidationError):
        unmarshal_attribute_value_json({"t": "BS", "bs": ["not-base64"]})
    with pytest.raises(ValidationError):
        unmarshal_attribute_value_json({"t": "L", "l": "not-a-list"})
    with pytest.raises(ValidationError):
        unmarshal_attribute_value_json({"t": "M", "m": "not-a-map"})
    with pytest.raises(ValidationError):
        unmarshal_attribute_value_json({"t": "X"})
