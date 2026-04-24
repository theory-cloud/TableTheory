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
        _validate_scenario_steps(path.name, scenario)


def _validate_scenario_steps(name: str, scenario: dict[str, Any]) -> None:
    steps = scenario.get("steps")
    assert isinstance(steps, list) and steps, f"{name} must declare steps[]"

    for index, step in enumerate(steps):
        assert isinstance(step, dict), f"{name} step {index} must be an object"
        op = step.get("op")
        assert isinstance(op, str) and op, f"{name} step {index} must declare op"

        if op == "sleep":
            continue
        if op in {"create", "update", "save"}:
            assert isinstance(step.get("item"), dict) and step["item"], f"{name} step {index} {op} needs item"
            continue
        if op in {"get", "delete"}:
            assert isinstance(step.get("key"), dict) and step["key"], f"{name} step {index} {op} needs key"
            continue
        if op == "transition_append_event":
            _validate_transition_actual(name, index, step.get("actual"))
            _validate_transition_event(name, index, step.get("event"))
            continue
        if op == "validate_provenance":
            assert isinstance(step.get("item"), dict) and step["item"], (
                f"{name} step {index} validate_provenance needs item"
            )
            continue

        raise AssertionError(f"{name} step {index} has unsupported op {op!r}")


def _validate_transition_actual(name: str, index: int, value: Any) -> None:
    assert isinstance(value, dict), f"{name} step {index} transition actual must be an object"
    assert isinstance(value.get("model"), str) and value["model"], (
        f"{name} step {index} actual.model required"
    )
    assert isinstance(value.get("key"), dict) and value["key"], f"{name} step {index} actual.key required"
    assert isinstance(value.get("set"), dict) and value["set"], f"{name} step {index} actual.set required"


def _validate_transition_event(name: str, index: int, value: Any) -> None:
    assert isinstance(value, dict), f"{name} step {index} transition event must be an object"
    assert isinstance(value.get("model"), str) and value["model"], f"{name} step {index} event.model required"
    assert isinstance(value.get("item"), dict) and value["item"], f"{name} step {index} event.item required"
