from __future__ import annotations

import base64
from dataclasses import dataclass

import pytest

from theorydb_py import (
    ModelDefinition,
    ValidationError,
    theorydb_field,
    unmarshal_stream_image,
    unmarshal_stream_record,
)


@dataclass(frozen=True)
class StreamThing:
    pk: str = theorydb_field(roles=["pk"])
    sk: str = theorydb_field(roles=["sk"])
    data: bytes = theorydb_field(binary=True)
    payload: dict[str, int] = theorydb_field(json=True)


def test_unmarshal_stream_image_binary_and_json() -> None:
    model = ModelDefinition.from_dataclass(StreamThing)
    raw = b"hello"

    image = {
        "pk": {"S": "A"},
        "sk": {"S": "B"},
        "data": {"B": base64.b64encode(raw).decode("ascii")},
        "payload": {"S": '{"a":1}'},
    }

    got = unmarshal_stream_image(model, image)
    assert got.pk == "A"
    assert got.sk == "B"
    assert got.data == raw
    assert got.payload == {"a": 1}


def test_unmarshal_stream_record_missing_image_returns_none() -> None:
    model = ModelDefinition.from_dataclass(StreamThing)
    image = {
        "pk": {"S": "A"},
        "sk": {"S": "B"},
        "data": {"B": base64.b64encode(b"x").decode("ascii")},
        "payload": {"S": '{"a":1}'},
    }

    record = {"dynamodb": {"NewImage": image}}
    assert unmarshal_stream_record(model, record) == unmarshal_stream_image(model, image)
    assert unmarshal_stream_record(model, record, image="OldImage") is None


def test_unmarshal_stream_image_invalid_binary_is_validation_error() -> None:
    model = ModelDefinition.from_dataclass(StreamThing)
    image = {"pk": {"S": "A"}, "sk": {"S": "B"}, "data": {"B": "not-base64"}, "payload": {"S": "{}"}}

    with pytest.raises(ValidationError):
        unmarshal_stream_image(model, image)


def test_unmarshal_stream_image_invalid_attribute_value_shape_is_validation_error() -> None:
    model = ModelDefinition.from_dataclass(StreamThing)
    image = {"pk": {"S": "A", "N": "1"}, "sk": {"S": "B"}, "data": {"B": ""}, "payload": {"S": "{}"}}

    with pytest.raises(ValidationError):
        unmarshal_stream_image(model, image)
