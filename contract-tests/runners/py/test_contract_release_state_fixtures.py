from __future__ import annotations

from pathlib import Path
from typing import Any, cast

import yaml


def _repo_root() -> Path:
    # contract-tests/runners/py/test_*.py -> repo root is 4 levels up
    return Path(__file__).resolve().parents[3]


def _load_yaml(path: Path) -> dict[str, Any]:
    data = yaml.safe_load(path.read_text(encoding="utf-8"))
    assert isinstance(data, dict)
    return cast(dict[str, Any], data)


def test_release_state_dms_write_policy_fixture() -> None:
    root = _repo_root()
    doc = _load_yaml(root / "contract-tests" / "dms" / "v0.1" / "models" / "release-state.yml")
    assert doc["dms_version"] == "0.1"

    models = {model["name"]: model for model in doc["models"]}
    actual = models["ReleaseStateActual"]
    event = models["ReleaseStateEvent"]

    assert actual["write_policy"] == {
        "mode": "mutable",
        "protected_attributes": ["pinnedReleaseId"],
    }
    assert event["write_policy"] == {"mode": "write_once", "protected_attributes": []}

    for model in (actual, event):
        attributes = {attr["attribute"] for attr in model["attributes"]}
        protected = model["write_policy"].get("protected_attributes", [])
        assert set(protected).issubset(attributes)


def test_release_state_p0_scenarios_are_capability_gated() -> None:
    root = _repo_root()
    scenario_dir = root / "contract-tests" / "scenarios" / "p0"
    scenarios = sorted(scenario_dir.glob("*release-state*.yml"))
    assert scenarios

    for path in scenarios:
        scenario = _load_yaml(path)
        assert scenario["dms_version"] == "0.1"
        assert scenario["name"].startswith("p0.release_state.")
        required = scenario.get("requires_capabilities")
        assert isinstance(required, list) and required, f"{path.name} must declare required capabilities"
