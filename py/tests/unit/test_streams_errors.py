from __future__ import annotations

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
class Note:
    pk: str = theorydb_field(roles=["pk"])
    sk: str = theorydb_field(roles=["sk"])
    value: int = theorydb_field(default=0)


def test_unmarshal_stream_record_validation_errors() -> None:
    model = ModelDefinition.from_dataclass(Note)

    with pytest.raises(ValidationError):
        unmarshal_stream_record(model, "not-a-map")

    with pytest.raises(ValidationError):
        unmarshal_stream_record(model, {"dynamodb": "not-a-map"})

    assert unmarshal_stream_record(model, {"dynamodb": {}}, image="NewImage") is None


def test_unmarshal_stream_image_decoding_errors() -> None:
    model = ModelDefinition.from_dataclass(Note)

    with pytest.raises(ValidationError):
        unmarshal_stream_image(model, "not-a-map")

    with pytest.raises(ValidationError):
        unmarshal_stream_image(model, {"pk": {"S": "A", "N": "1"}})

    with pytest.raises(ValidationError):
        unmarshal_stream_image(model, {"pk": {"B": 1}})

    with pytest.raises(ValidationError):
        unmarshal_stream_image(model, {"pk": {"BS": "nope"}})

    with pytest.raises(ValidationError):
        unmarshal_stream_image(model, {"pk": {"BS": ["not-base64"]}})

    with pytest.raises(ValidationError):
        unmarshal_stream_image(model, {"pk": {"M": "nope"}})

    with pytest.raises(ValidationError):
        unmarshal_stream_image(model, {"pk": {"L": "nope"}})

    with pytest.raises(ValidationError):
        unmarshal_stream_image(model, {"pk": {"S": 1}})

    with pytest.raises(ValidationError):
        unmarshal_stream_image(model, {"pk": {"SS": "nope"}})

    with pytest.raises(ValidationError):
        unmarshal_stream_image(model, {"pk": {"SS": ["A", 1]}})

    with pytest.raises(ValidationError):
        unmarshal_stream_image(model, {"pk": {"BOOL": "true"}})

    with pytest.raises(ValidationError):
        unmarshal_stream_image(model, {"pk": {"NULL": False}})

    with pytest.raises(ValidationError):
        unmarshal_stream_image(model, {"pk": {"X": "nope"}})
