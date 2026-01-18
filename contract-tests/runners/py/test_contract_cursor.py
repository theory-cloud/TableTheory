from __future__ import annotations

import base64
import json
from pathlib import Path

from theorydb_py.query import decode_cursor, encode_cursor


def _repo_root() -> Path:
    # contract-tests/runners/py/test_*.py -> repo root is 4 levels up
    return Path(__file__).resolve().parents[3]


def test_cursor_golden_v0_1_round_trip() -> None:
    root = _repo_root()
    cursor_path = root / "contract-tests" / "golden" / "cursor" / "cursor_v0.1_basic.cursor"
    json_path = root / "contract-tests" / "golden" / "cursor" / "cursor_v0.1_basic.json"

    cursor = cursor_path.read_text(encoding="utf-8").strip()
    expected_json = json_path.read_text(encoding="utf-8").strip()

    padding = "=" * (-len(cursor) % 4)
    decoded_json = base64.urlsafe_b64decode(cursor + padding).decode("utf-8")
    assert decoded_json == expected_json

    decoded = decode_cursor(cursor)
    reencoded = encode_cursor(decoded.last_key, index=decoded.index, sort=decoded.sort)
    assert reencoded == cursor

    payload = json.loads(expected_json)
    assert decoded.last_key == payload["lastKey"]
    assert decoded.index == payload["index"]
    assert decoded.sort == payload["sort"]

