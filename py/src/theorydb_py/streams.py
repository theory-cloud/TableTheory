from __future__ import annotations

import base64
import json
from dataclasses import fields, is_dataclass
from decimal import Decimal
from typing import Any, cast, get_args, get_origin

from boto3.dynamodb.types import TypeDeserializer

from .errors import ValidationError
from .model import ModelDefinition


def _coerce_value(value: Any, annotation: Any) -> Any:
    if value is None:
        return None

    if annotation is int and isinstance(value, Decimal):
        return int(value)
    if annotation is float and isinstance(value, Decimal):
        return float(value)

    origin = get_origin(annotation)
    if origin is set and isinstance(value, set):
        (elem_type,) = get_args(annotation) or (Any,)
        return {_coerce_value(v, elem_type) for v in value}

    return value


def _decode_stream_av(av: Any) -> dict[str, Any]:
    if not isinstance(av, dict):
        raise ValidationError("stream attribute value must be a map")
    if len(av) != 1:
        raise ValidationError("stream attribute value must have exactly one type key")

    (kind, value), *_ = av.items()

    if kind == "B":
        if not isinstance(value, str):
            raise ValidationError("stream binary value must be base64 string")
        try:
            return {"B": base64.b64decode(value)}
        except Exception as err:
            raise ValidationError("stream binary value is not valid base64") from err

    if kind == "BS":
        if not isinstance(value, list) or not all(isinstance(v, str) for v in value):
            raise ValidationError("stream binary set must be list of base64 strings")
        try:
            return {"BS": [base64.b64decode(v) for v in value]}
        except Exception as err:
            raise ValidationError("stream binary set contains invalid base64") from err

    if kind == "M":
        if not isinstance(value, dict):
            raise ValidationError("stream map value must be a map")
        return {"M": {k: _decode_stream_av(v) for k, v in value.items()}}

    if kind == "L":
        if not isinstance(value, list):
            raise ValidationError("stream list value must be a list")
        return {"L": [_decode_stream_av(v) for v in value]}

    if kind in {"S", "N"}:
        if not isinstance(value, str):
            raise ValidationError(f"stream {kind} value must be a string")
        return {kind: value}

    if kind in {"SS", "NS"}:
        if not isinstance(value, list) or not all(isinstance(v, str) for v in value):
            raise ValidationError(f"stream {kind} value must be a list of strings")
        return {kind: value}

    if kind == "BOOL":
        if not isinstance(value, bool):
            raise ValidationError("stream BOOL value must be a boolean")
        return {"BOOL": value}

    if kind == "NULL":
        if value is not True:
            raise ValidationError("stream NULL value must be true")
        return {"NULL": True}

    raise ValidationError(f"unsupported stream attribute value type: {kind}")


def _decode_stream_image(image: Any) -> dict[str, Any]:
    if not isinstance(image, dict):
        raise ValidationError("stream image must be a map")
    return {k: _decode_stream_av(v) for k, v in image.items()}


def unmarshal_stream_image[T](model: ModelDefinition[T], stream_image: Any) -> T:
    if not is_dataclass(model.model_type):
        raise ValidationError("model_type must be a dataclass")

    image = _decode_stream_image(stream_image)
    deserializer = TypeDeserializer()

    model_cls = model.model_type
    model_annotations = getattr(model_cls, "__annotations__", {})

    kwargs: dict[str, Any] = {}
    for dc_field in fields(cast(Any, model_cls)):
        if dc_field.name not in model.attributes:
            continue

        attr_def = model.attributes[dc_field.name]
        if attr_def.attribute_name not in image:
            continue

        raw = deserializer.deserialize(image[attr_def.attribute_name])
        if attr_def.json and isinstance(raw, str):
            raw = json.loads(raw)

        kwargs[dc_field.name] = _coerce_value(raw, model_annotations.get(dc_field.name, Any))

    try:
        return model_cls(**kwargs)
    except TypeError as err:
        raise ValidationError(str(err)) from err


def unmarshal_stream_record[T](
    model: ModelDefinition[T], record: Any, *, image: str = "NewImage"
) -> T | None:
    if not isinstance(record, dict):
        raise ValidationError("record must be a map")
    dynamodb = record.get("dynamodb")
    if not isinstance(dynamodb, dict):
        raise ValidationError("record.dynamodb must be a map")
    stream_image = dynamodb.get(image)
    if stream_image is None:
        return None
    return unmarshal_stream_image(model, stream_image)
