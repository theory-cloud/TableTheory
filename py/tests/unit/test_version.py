from __future__ import annotations

import json
from pathlib import Path

import tabletheory_py


def test_version_matches_version_json() -> None:
    version_file = Path(__file__).resolve().parents[2] / "src" / "tabletheory_py" / "version.json"
    data = json.loads(version_file.read_text(encoding="utf-8"))
    assert tabletheory_py.__repo_version__ == data["version"]
    if "-rc." in data["version"]:
        assert "-rc." not in tabletheory_py.__version__
        assert "rc" in tabletheory_py.__version__
    else:
        assert tabletheory_py.__version__ == data["version"]
