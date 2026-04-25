from __future__ import annotations

import re
from copy import deepcopy
from dataclasses import asdict, dataclass, is_dataclass
from decimal import Decimal
from pathlib import Path
from typing import Any, cast

import yaml

from theorydb_py import (
    ConditionFailedError,
    ImmutableModelMutationError,
    ModelDefinition,
    ProtectedFieldMutationError,
    RejectedDeployAuthorityEvidenceError,
    Table,
    WritePolicy,
    theorydb_field,
    transition_release_state,
    validate_deploy_authority_metadata,
)


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


def test_release_state_scenarios_execute_for_python() -> None:
    root = _repo_root()
    scenario_dir = root / "contract-tests" / "scenarios" / "p0"
    scenarios = [
        _load_yaml(scenario_dir / "06-release-state-write-policy.yml"),
        _load_yaml(scenario_dir / "07-release-state-protected-fields.yml"),
        _load_yaml(scenario_dir / "08-release-state-transactional-transition.yml"),
        _load_yaml(scenario_dir / "09-release-state-provenance-confidence.yml"),
    ]

    for scenario in scenarios:
        assert scenario.get("requires_capabilities", [])
        driver = _ReleaseStatePyDriver()
        for step in scenario["steps"]:
            result, error = _capture_contract_result(
                lambda driver=driver, scenario=scenario, step=step: driver.run_step(scenario, step)
            )
            _assert_contract_expectation(scenario["name"], step, result, error)


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


@dataclass(frozen=True)
class _ReleaseStateActual:
    PK: str = theorydb_field(name="PK", roles=["pk"])
    SK: str = theorydb_field(name="SK", roles=["sk"])
    status: str | None = theorydb_field(default=None)
    pinnedReleaseId: str | None = theorydb_field(default=None)
    previousReleaseId: str | None = theorydb_field(default=None)
    provenance: dict[str, Any] | None = theorydb_field(json=True, default=None)
    confidence: dict[str, Any] | None = theorydb_field(json=True, default=None)
    version: int = theorydb_field(roles=["version"], default=0)


@dataclass(frozen=True)
class _ReleaseStateEvent:
    PK: str = theorydb_field(name="PK", roles=["pk"])
    SK: str = theorydb_field(name="SK", roles=["sk"])
    releaseId: str | None = theorydb_field(default=None)
    eventType: str | None = theorydb_field(default=None)
    provenance: dict[str, Any] | None = theorydb_field(json=True, default=None)
    confidence: dict[str, Any] | None = theorydb_field(json=True, default=None)
    recordedAt: str | None = theorydb_field(default=None)
    ttl: int | None = theorydb_field(roles=["ttl"], omitempty=True, default=None)


class _ReleaseStatePyDriver:
    def __init__(self) -> None:
        self.client = _InMemoryDynamoDBClient()
        self.tables: dict[str, Table[Any]] = {
            "ReleaseStateActual": Table(
                ModelDefinition.from_dataclass(
                    _ReleaseStateActual,
                    table_name="release_state_contract",
                    write_policy=WritePolicy(mode="mutable", protected_attributes=["pinnedReleaseId"]),
                ),
                client=self.client,
            ),
            "ReleaseStateEvent": Table(
                ModelDefinition.from_dataclass(
                    _ReleaseStateEvent,
                    table_name="release_state_contract",
                    write_policy=WritePolicy(mode="write_once"),
                ),
                client=self.client,
            ),
        }

    def run_step(self, scenario: dict[str, Any], step: dict[str, Any]) -> Any:
        model_name = step.get("model") or scenario["model"]
        table = self.tables[model_name]

        if step["op"] == "create":
            self._validate_metadata_if_present(model_name, step["item"])
            kwargs: dict[str, Any] = {}
            if step.get("if_not_exists"):
                kwargs = {
                    "condition_expression": "attribute_not_exists(#pk)",
                    "expression_attribute_names": {"#pk": table._model.pk.attribute_name},
                }
            table.put(self._item(model_name, step["item"]), **kwargs)
            return

        if step["op"] == "save":
            self._validate_metadata_if_present(model_name, step["item"])
            table.save(self._item(model_name, step["item"]))
            return

        if step["op"] == "update":
            item = step["item"]
            updates = {field: item[field] for field in step.get("fields", []) if field in item}
            table.update(
                item["PK"],
                item["SK"],
                updates,
                protected_attributes=step.get("protected_attributes", []),
            )
            return

        if step["op"] == "delete":
            key = step["key"]
            table.delete(key["PK"], key["SK"])
            return

        if step["op"] == "get":
            key = step["key"]
            return _contract_item(table.get(key["PK"], key["SK"], consistent_read=True))

        if step["op"] == "transition_append_event":
            actual = step["actual"]
            event = step["event"]
            transition_release_state(
                self.tables[actual["model"]],
                self.tables[event["model"]],
                actual_key=actual["key"],
                expected_version=actual.get("expected_version"),
                set_values=actual["set"],
                event_item=self._item(event["model"], event["item"]),
            )
            return None

        if step["op"] == "validate_provenance":
            if model_name != "ReleaseStateActual":
                raise AssertionError(f"unsupported validate_provenance model: {model_name}")
            validate_deploy_authority_metadata(step["item"])
            return None

        raise AssertionError(f"unsupported executable op: {step['op']}")

    @staticmethod
    def _validate_metadata_if_present(model_name: str, item: dict[str, Any]) -> None:
        if model_name != "ReleaseStateActual":
            return
        if "provenance" in item or "confidence" in item:
            validate_deploy_authority_metadata(item)

    @staticmethod
    def _item(model_name: str, item: dict[str, Any]) -> Any:
        if model_name == "ReleaseStateActual":
            return _ReleaseStateActual(**item)
        if model_name == "ReleaseStateEvent":
            return _ReleaseStateEvent(**item)
        raise AssertionError(f"unknown model: {model_name}")


class _InMemoryDynamoDBClient:
    def __init__(self) -> None:
        self.items: dict[tuple[str, str, str], dict[str, Any]] = {}

    def put_item(self, **req: Any) -> dict[str, Any]:
        key = self._key(req["TableName"], req["Item"])
        if "attribute_not_exists" in req.get("ConditionExpression", "") and key in self.items:
            raise ConditionFailedError("conditional write failed")
        self.items[key] = dict(req["Item"])
        return {}

    def get_item(self, **req: Any) -> dict[str, Any]:
        item = self.items.get(self._key(req["TableName"], req["Key"]))
        return {"Item": dict(item)} if item is not None else {}

    def delete_item(self, **req: Any) -> dict[str, Any]:
        self.items.pop(self._key(req["TableName"], req["Key"]), None)
        return {}

    def update_item(self, **req: Any) -> dict[str, Any]:
        return self._apply_update(req, self.items)

    def transact_write_items(self, **req: Any) -> dict[str, Any]:
        draft = deepcopy(self.items)
        for action in req["TransactItems"]:
            if "Put" in action:
                put = action["Put"]
                key = self._key(put["TableName"], put["Item"])
                if not self._condition_matches(put, draft.get(key)):
                    raise ConditionFailedError("conditional write failed")
                draft[key] = dict(put["Item"])
                continue
            if "Update" in action:
                self._apply_update(action["Update"], draft)
                continue
            raise AssertionError(f"unsupported transact action: {action!r}")
        self.items = draft
        return {}

    def _apply_update(
        self,
        req: dict[str, Any],
        items: dict[tuple[str, str, str], dict[str, Any]],
    ) -> dict[str, Any]:
        key = self._key(req["TableName"], req["Key"])
        item = dict(items[key])
        if not self._condition_matches(req, item):
            raise ConditionFailedError("conditional write failed")
        names = req.get("ExpressionAttributeNames", {})
        values = req.get("ExpressionAttributeValues", {})
        for assignment in _set_assignments(req["UpdateExpression"]):
            left, right = [part.strip() for part in assignment.split("=", 1)]
            item[names[left]] = values[right]
        for assignment in _add_assignments(req["UpdateExpression"]):
            left, right = [part.strip() for part in assignment.split(None, 1)]
            attr = names[left]
            item[attr] = _add_number(item.get(attr), values[right])
        items[key] = item
        return {"Attributes": dict(item)}

    def _condition_matches(self, req: dict[str, Any], item: dict[str, Any] | None) -> bool:
        condition = req.get("ConditionExpression", "")
        if not condition:
            return True
        names = req.get("ExpressionAttributeNames", {})
        values = req.get("ExpressionAttributeValues", {})
        if "attribute_not_exists" in condition:
            if item is not None:
                return False
        equality = re.search(r"(#[A-Za-z0-9_]+)\s*=\s*(:[A-Za-z0-9_]+)", condition)
        if equality:
            if item is None:
                return False
            left, right = equality.groups()
            if item.get(names[left]) != values[right]:
                return False
        return True

    @staticmethod
    def _key(table_name: str, attrs: dict[str, Any]) -> tuple[str, str, str]:
        return (table_name, _av_to_string(attrs["PK"]), _av_to_string(attrs["SK"]))


def _set_assignments(update_expression: str) -> list[str]:
    match = re.search(r"\bSET\b([\s\S]*?)(?=\b(?:REMOVE|ADD|DELETE)\b|$)", update_expression)
    if not match:
        return []
    return [part.strip() for part in match.group(1).split(",") if part.strip()]


def _add_assignments(update_expression: str) -> list[str]:
    match = re.search(r"\bADD\b([\s\S]*?)(?=\b(?:SET|REMOVE|DELETE)\b|$)", update_expression)
    if not match:
        return []
    return [part.strip() for part in match.group(1).split(",") if part.strip()]


def _add_number(current: dict[str, Any] | None, increment: dict[str, Any]) -> dict[str, str]:
    current_value = Decimal(str((current or {"N": "0"})["N"]))
    increment_value = Decimal(str(increment["N"]))
    result = current_value + increment_value
    if result == result.to_integral_value():
        text = str(int(result))
    else:
        text = format(result, "f")
    return {"N": text}


def _av_to_string(value: dict[str, Any]) -> str:
    if "S" in value:
        return str(value["S"])
    if "N" in value:
        return str(value["N"])
    raise AssertionError(f"unsupported key attribute value: {value!r}")


def _contract_item(value: Any) -> dict[str, Any]:
    if is_dataclass(value):
        return asdict(value)
    assert isinstance(value, dict)
    return value


def _capture_contract_result(fn: Any) -> tuple[Any, str | None]:
    try:
        return fn(), None
    except ImmutableModelMutationError:
        return None, "ErrImmutableModelMutation"
    except ProtectedFieldMutationError:
        return None, "ErrProtectedFieldMutation"
    except ConditionFailedError:
        return None, "ErrConditionFailed"
    except RejectedDeployAuthorityEvidenceError:
        return None, "ErrRejectedDeployAuthorityEvidence"


def _assert_contract_expectation(
    scenario_name: str,
    step: dict[str, Any],
    result: Any,
    error: str | None,
) -> None:
    expected = step.get("expect", {})
    if expected.get("ok") is True:
        assert error is None, f"{scenario_name} step {step['op']} expected ok, got {error}"
    elif "error" in expected:
        assert error == expected["error"], (
            f"{scenario_name} step {step['op']} expected {expected['error']}, got {error}"
        )
        return
    else:
        assert error is None

    if "item_contains" in expected:
        assert isinstance(result, dict), f"{scenario_name} step {step['op']} did not return an item"
        for key, value in expected["item_contains"].items():
            assert result.get(key) == value, (
                f"{scenario_name} step {step['op']} expected {key}={value!r}, got {result.get(key)!r}"
            )
