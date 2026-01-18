from __future__ import annotations

import json
from pathlib import Path

import theorydb_py


def test_version_matches_version_json() -> None:
    version_file = Path(__file__).resolve().parents[2] / "src" / "theorydb_py" / "version.json"
    data = json.loads(version_file.read_text(encoding="utf-8"))
    assert theorydb_py.__repo_version__ == data["version"]
    if "-rc." in data["version"]:
        assert "-rc." not in theorydb_py.__version__
        assert "rc" in theorydb_py.__version__
    else:
        assert theorydb_py.__version__ == data["version"]
