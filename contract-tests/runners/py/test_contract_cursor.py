from __future__ import annotations

import base64
import json
from pathlib import Path
from typing import Any, cast

from tabletheory_py.query import decode_cursor, encode_cursor


def _repo_root() -> Path:
    # contract-tests/runners/py/test_*.py -> repo root is 4 levels up
    return Path(__file__).resolve().parents[3]


def test_cursor_golden_v0_1_corpus_round_trips() -> None:
    cursor_dir = _repo_root() / "contract-tests" / "golden" / "cursor"
    json_paths = sorted(cursor_dir.glob("cursor_v0.1_*.json"))
    assert json_paths

    for json_path in json_paths:
        stem = json_path.stem
        cursor = (cursor_dir / f"{stem}.cursor").read_text(encoding="utf-8").strip()
        expected_json = json_path.read_text(encoding="utf-8").strip()
        payload = cast(dict[str, Any], json.loads(expected_json))
        last_key = _last_key_from_dynamo_json(cast(dict[str, Any], payload["lastKey"]))

        assert encode_cursor(last_key, index=payload.get("index"), sort=payload.get("sort")) == cursor
        if cursor == "":
            continue

        padding = "=" * (-len(cursor) % 4)
        decoded_json = base64.urlsafe_b64decode(cursor + padding).decode("utf-8")
        assert decoded_json == expected_json

        decoded = decode_cursor(cursor)
        reencoded = encode_cursor(decoded.last_key, index=decoded.index, sort=decoded.sort)
        assert reencoded == cursor

        assert decoded.last_key == last_key
        assert decoded.index == payload.get("index")
        assert decoded.sort == payload.get("sort")


def _last_key_from_dynamo_json(value: dict[str, Any]) -> dict[str, Any]:
    return {key: _av_from_dynamo_json(cast(dict[str, Any], av)) for key, av in value.items()}


def _av_from_dynamo_json(value: dict[str, Any]) -> dict[str, Any]:
    assert len(value) == 1
    kind, raw = next(iter(value.items()))
    if kind == "B":
        return {"B": base64.b64decode(cast(str, raw))}
    if kind == "BS":
        return {"BS": [base64.b64decode(item) for item in cast(list[str], raw)]}
    if kind == "L":
        return {"L": [_av_from_dynamo_json(cast(dict[str, Any], item)) for item in cast(list[Any], raw)]}
    if kind == "M":
        return {
            "M": {
                key: _av_from_dynamo_json(cast(dict[str, Any], item))
                for key, item in cast(dict[str, Any], raw).items()
            }
        }
    return {kind: raw}
