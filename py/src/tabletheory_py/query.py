from __future__ import annotations

import base64
import json
from dataclasses import dataclass
from typing import Any, Literal


@dataclass(frozen=True)
class SortKeyCondition:
    op: str
    values: tuple[Any, ...]

    @staticmethod
    def eq(value: Any) -> SortKeyCondition:
        return SortKeyCondition(op="=", values=(value,))

    @staticmethod
    def lt(value: Any) -> SortKeyCondition:
        return SortKeyCondition(op="<", values=(value,))

    @staticmethod
    def lte(value: Any) -> SortKeyCondition:
        return SortKeyCondition(op="<=", values=(value,))

    @staticmethod
    def gt(value: Any) -> SortKeyCondition:
        return SortKeyCondition(op=">", values=(value,))

    @staticmethod
    def gte(value: Any) -> SortKeyCondition:
        return SortKeyCondition(op=">=", values=(value,))

    @staticmethod
    def between(low: Any, high: Any) -> SortKeyCondition:
        return SortKeyCondition(op="between", values=(low, high))

    @staticmethod
    def begins_with(prefix: Any) -> SortKeyCondition:
        return SortKeyCondition(op="begins_with", values=(prefix,))


@dataclass(frozen=True)
class Page[T]:
    items: list[T]
    next_cursor: str | None


type LogicalOp = Literal["AND", "OR"]


@dataclass(frozen=True)
class FilterCondition:
    field: str
    op: str
    values: tuple[Any, ...] = ()

    @staticmethod
    def eq(field: str, value: Any) -> FilterCondition:
        return FilterCondition(field=field, op="=", values=(value,))

    @staticmethod
    def ne(field: str, value: Any) -> FilterCondition:
        return FilterCondition(field=field, op="!=", values=(value,))

    @staticmethod
    def lt(field: str, value: Any) -> FilterCondition:
        return FilterCondition(field=field, op="<", values=(value,))

    @staticmethod
    def lte(field: str, value: Any) -> FilterCondition:
        return FilterCondition(field=field, op="<=", values=(value,))

    @staticmethod
    def gt(field: str, value: Any) -> FilterCondition:
        return FilterCondition(field=field, op=">", values=(value,))

    @staticmethod
    def gte(field: str, value: Any) -> FilterCondition:
        return FilterCondition(field=field, op=">=", values=(value,))

    @staticmethod
    def between(field: str, low: Any, high: Any) -> FilterCondition:
        return FilterCondition(field=field, op="between", values=(low, high))

    @staticmethod
    def begins_with(field: str, prefix: Any) -> FilterCondition:
        return FilterCondition(field=field, op="begins_with", values=(prefix,))

    @staticmethod
    def contains(field: str, value: Any) -> FilterCondition:
        return FilterCondition(field=field, op="contains", values=(value,))

    @staticmethod
    def in_(field: str, values: list[Any]) -> FilterCondition:
        return FilterCondition(field=field, op="in", values=(list(values),))

    @staticmethod
    def exists(field: str) -> FilterCondition:
        return FilterCondition(field=field, op="exists")

    @staticmethod
    def not_exists(field: str) -> FilterCondition:
        return FilterCondition(field=field, op="not_exists")


@dataclass(frozen=True)
class FilterGroup:
    op: LogicalOp
    filters: tuple[FilterExpression, ...]

    @staticmethod
    def and_(*filters: FilterExpression) -> FilterGroup:
        return FilterGroup(op="AND", filters=tuple(filters))

    @staticmethod
    def or_(*filters: FilterExpression) -> FilterGroup:
        return FilterGroup(op="OR", filters=tuple(filters))


type FilterExpression = FilterCondition | FilterGroup


@dataclass(frozen=True)
class Cursor:
    last_key: dict[str, Any]
    index: str | None = None
    sort: str | None = None


def _ensure_single_key_map(value: Any) -> tuple[str, Any]:
    if not isinstance(value, dict) or len(value) != 1:
        raise ValueError("attribute value must be a single-key map")
    (key, inner), *_ = value.items()
    return str(key), inner


def _av_to_json(av: Any) -> dict[str, Any]:
    kind, value = _ensure_single_key_map(av)

    if kind in {"S", "N"}:
        if not isinstance(value, str):
            raise ValueError(f"{kind} value must be a string")
        return {kind: value}

    if kind == "B":
        if not isinstance(value, (bytes, bytearray)):
            raise ValueError("B value must be bytes")
        return {"B": base64.b64encode(bytes(value)).decode("ascii")}

    if kind == "BOOL":
        if not isinstance(value, bool):
            raise ValueError("BOOL value must be a boolean")
        return {"BOOL": value}

    if kind == "NULL":
        if value is not True:
            raise ValueError("NULL value must be true")
        return {"NULL": True}

    if kind in {"SS", "NS"}:
        if not isinstance(value, list) or not all(isinstance(v, str) for v in value):
            raise ValueError(f"{kind} value must be a list of strings")
        return {kind: value}

    if kind == "BS":
        if not isinstance(value, list) or not all(isinstance(v, (bytes, bytearray)) for v in value):
            raise ValueError("BS value must be a list of bytes")
        return {"BS": [base64.b64encode(bytes(v)).decode("ascii") for v in value]}

    if kind == "L":
        if not isinstance(value, list):
            raise ValueError("L value must be a list")
        return {"L": [_av_to_json(v) for v in value]}

    if kind == "M":
        if not isinstance(value, dict):
            raise ValueError("M value must be a map")
        out: dict[str, Any] = {}
        for k in sorted(value.keys()):
            out[str(k)] = _av_to_json(value[k])
        return {"M": out}

    raise ValueError(f"unsupported attribute value type: {kind}")


def _av_from_json(enc: Any) -> dict[str, Any]:
    kind, value = _ensure_single_key_map(enc)

    if kind in {"S", "N"}:
        if not isinstance(value, str):
            raise ValueError(f"{kind} value must be a string")
        return {kind: value}

    if kind == "B":
        if not isinstance(value, str):
            raise ValueError("B value must be a base64 string")
        return {"B": base64.b64decode(value)}

    if kind == "BOOL":
        if not isinstance(value, bool):
            raise ValueError("BOOL value must be a boolean")
        return {"BOOL": value}

    if kind == "NULL":
        if value is not True:
            raise ValueError("NULL value must be true")
        return {"NULL": True}

    if kind in {"SS", "NS"}:
        if not isinstance(value, list) or not all(isinstance(v, str) for v in value):
            raise ValueError(f"{kind} value must be a list of strings")
        return {kind: value}

    if kind == "BS":
        if not isinstance(value, list) or not all(isinstance(v, str) for v in value):
            raise ValueError("BS value must be a list of base64 strings")
        return {"BS": [base64.b64decode(v) for v in value]}

    if kind == "L":
        if not isinstance(value, list):
            raise ValueError("L value must be a list")
        return {"L": [_av_from_json(v) for v in value]}

    if kind == "M":
        if not isinstance(value, dict):
            raise ValueError("M value must be a map")
        out: dict[str, Any] = {}
        for k in sorted(value.keys()):
            out[str(k)] = _av_from_json(value[k])
        return {"M": out}

    raise ValueError(f"unsupported attribute value type: {kind}")


def encode_cursor(last_key: Any, *, index: str | None = None, sort: str | None = None) -> str:
    if not last_key:
        return ""
    if not isinstance(last_key, dict):
        raise ValueError("last_key must be a map")

    last_key_json: dict[str, Any] = {}
    for k in sorted(last_key.keys()):
        last_key_json[str(k)] = _av_to_json(last_key[k])

    parts: list[str] = []
    parts.append('"lastKey":' + json.dumps(last_key_json, separators=(",", ":"), ensure_ascii=False))
    if index is not None:
        parts.append('"index":' + json.dumps(index, separators=(",", ":"), ensure_ascii=False))
    if sort is not None:
        parts.append('"sort":' + json.dumps(sort, separators=(",", ":"), ensure_ascii=False))

    payload = ("{" + ",".join(parts) + "}").encode("utf-8")
    return base64.urlsafe_b64encode(payload).decode("ascii")


def decode_cursor(cursor: str) -> Cursor:
    raw = str(cursor or "").strip()
    if not raw:
        raise ValueError("cursor is empty")

    padding = "=" * (-len(raw) % 4)
    data = base64.urlsafe_b64decode(raw + padding).decode("utf-8")
    parsed = json.loads(data)
    if not isinstance(parsed, dict):
        raise ValueError("cursor must decode to an object")

    last_key_raw = parsed.get("lastKey")
    if not isinstance(last_key_raw, dict):
        raise ValueError("cursor lastKey is invalid")

    last_key: dict[str, Any] = {}
    for k in sorted(last_key_raw.keys()):
        last_key[str(k)] = _av_from_json(last_key_raw[k])

    index = parsed.get("index")
    sort = parsed.get("sort")
    return Cursor(
        last_key=last_key,
        index=index if isinstance(index, str) else None,
        sort=sort if sort in {"ASC", "DESC"} else None,
    )
