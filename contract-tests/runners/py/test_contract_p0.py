from __future__ import annotations

import os
import time
from dataclasses import asdict, dataclass, is_dataclass
from decimal import Decimal
from pathlib import Path
from typing import Any, cast

import boto3
import pytest
import yaml
from boto3.dynamodb.types import TypeSerializer
from botocore.exceptions import ClientError, EndpointConnectionError

from theorydb_py import (
    ConditionFailedError,
    FilterCondition,
    FilterGroup,
    ImmutableModelMutationError,
    ModelDefinition,
    NotFoundError,
    Projection,
    ProtectedFieldMutationError,
    RejectedDeployAuthorityEvidenceError,
    SortKeyCondition,
    Table,
    ValidationError,
    WritePolicy,
    gsi,
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


def _load_models() -> dict[str, dict[str, Any]]:
    out: dict[str, dict[str, Any]] = {}
    for path in sorted((_repo_root() / "contract-tests" / "dms" / "v0.1" / "models").glob("*.yml")):
        doc = _load_yaml(path)
        for model in doc.get("models", []):
            out[model["name"]] = model
    assert out
    return out


def _load_scenarios() -> list[dict[str, Any]]:
    scenarios = _load_scenarios_from("p0")
    assert len(scenarios) >= 9
    return scenarios


def _load_scenarios_from(name: str) -> list[dict[str, Any]]:
    scenario_dir = _repo_root() / "contract-tests" / "scenarios" / name
    scenarios = [_load_yaml(path) for path in sorted(scenario_dir.glob("*.yml"))]
    assert scenarios
    return scenarios


@dataclass(frozen=True)
class _User:
    PK: str = theorydb_field(name="PK", roles=["pk"])
    SK: str = theorydb_field(name="SK", roles=["sk"])
    emailHash: str | None = theorydb_field(omitempty=True, default=None)
    nickname: str | None = theorydb_field(omitempty=True, default=None)
    tags: set[str] | None = theorydb_field(set_=True, omitempty=True, default=None)
    createdAt: str = theorydb_field(roles=["created_at"], default="")
    updatedAt: str = theorydb_field(roles=["updated_at"], default="")
    version: int | None = theorydb_field(roles=["version"], default=None)
    ttl: int | None = theorydb_field(roles=["ttl"], omitempty=True, default=None)


@dataclass(frozen=True)
class _Order:
    PK: str = theorydb_field(name="PK", roles=["pk"])
    SK: str = theorydb_field(name="SK", roles=["sk"])
    status: str | None = theorydb_field(omitempty=True, default=None)
    amount: int | None = theorydb_field(omitempty=True, default=None)
    createdAt: str = theorydb_field(roles=["created_at"], default="")
    updatedAt: str = theorydb_field(roles=["updated_at"], default="")
    version: int | None = theorydb_field(roles=["version"], default=None)
    ttl: int | None = theorydb_field(roles=["ttl"], omitempty=True, default=None)


@dataclass(frozen=True)
class _ReleaseStateActual:
    PK: str = theorydb_field(name="PK", roles=["pk"])
    SK: str = theorydb_field(name="SK", roles=["sk"])
    status: str | None = theorydb_field(omitempty=True, default=None)
    pinnedReleaseId: str | None = theorydb_field(omitempty=True, default=None)
    previousReleaseId: str | None = theorydb_field(omitempty=True, default=None)
    provenance: dict[str, Any] | None = theorydb_field(json=True, omitempty=True, default=None)
    confidence: dict[str, Any] | None = theorydb_field(json=True, omitempty=True, default=None)
    updatedAt: str = theorydb_field(roles=["updated_at"], default="")
    version: int | None = theorydb_field(roles=["version"], default=None)


@dataclass(frozen=True)
class _ReleaseStateEvent:
    PK: str = theorydb_field(name="PK", roles=["pk"])
    SK: str = theorydb_field(name="SK", roles=["sk"])
    releaseId: str | None = theorydb_field(omitempty=True, default=None)
    eventType: str | None = theorydb_field(omitempty=True, default=None)
    provenance: dict[str, Any] | None = theorydb_field(json=True, omitempty=True, default=None)
    confidence: dict[str, Any] | None = theorydb_field(json=True, omitempty=True, default=None)
    recordedAt: str | None = theorydb_field(omitempty=True, default=None)
    ttl: int | None = theorydb_field(roles=["ttl"], omitempty=True, default=None)


class _TheorydbPyDriver:
    def __init__(self, client: Any) -> None:
        self.client = client
        self.tables: dict[str, Table[Any]] = {
            "User": Table(
                ModelDefinition.from_dataclass(
                    _User,
                    table_name="users_contract",
                    indexes=[gsi("gsi-email", partition="emailHash", projection=Projection.all())],
                ),
                client=client,
            ),
            "Order": Table(
                ModelDefinition.from_dataclass(
                    _Order,
                    table_name="orders_contract",
                    indexes=[
                        gsi("gsi-status", partition="status", sort="createdAt", projection=Projection.all())
                    ],
                ),
                client=client,
            ),
            "ReleaseStateActual": Table(
                ModelDefinition.from_dataclass(
                    _ReleaseStateActual,
                    table_name="release_state_contract",
                    write_policy=WritePolicy(mode="mutable", protected_attributes=["pinnedReleaseId"]),
                ),
                client=client,
            ),
            "ReleaseStateEvent": Table(
                ModelDefinition.from_dataclass(
                    _ReleaseStateEvent,
                    table_name="release_state_contract",
                    write_policy=WritePolicy(mode="write_once"),
                ),
                client=client,
            ),
        }

    def create(self, model: str, item: dict[str, Any], *, if_not_exists: bool = False) -> None:
        self._validate_release_state_metadata_if_present(model, item)
        table = self.tables[model]
        kwargs: dict[str, Any] = {}
        if if_not_exists:
            kwargs = {
                "condition_expression": "attribute_not_exists(#pk)",
                "expression_attribute_names": {"#pk": table._model.pk.attribute_name},
            }
        table.put(self._item(model, item), **kwargs)

    def get(self, model: str, key: dict[str, Any]) -> dict[str, Any]:
        table = self.tables[model]
        return _contract_item(table.get(key["PK"], key["SK"], consistent_read=True))

    def update(
        self,
        model: str,
        item: dict[str, Any],
        fields: list[str],
        *,
        protected_attributes: list[str] | None = None,
    ) -> None:
        table = self.tables[model]
        updates = {field: item[field] for field in fields if field in item}
        expected_version = _expected_version(table, item)
        table.update(
            item["PK"],
            item["SK"],
            updates,
            expected_version=expected_version,
            protected_attributes=protected_attributes or [],
        )

    def save(self, model: str, item: dict[str, Any]) -> None:
        self._validate_release_state_metadata_if_present(model, item)
        self.tables[model].save(self._item(model, item))

    def delete(self, model: str, key: dict[str, Any]) -> None:
        self.tables[model].delete(key["PK"], key["SK"])

    def query(self, model: str, request: dict[str, Any]) -> dict[str, Any]:
        partition = request.get("partition")
        if partition is None:
            raise ValidationError("query partition is required")
        page = self.tables[model].query(
            _condition_value(partition),
            sort=_sort_condition(request.get("sort")),
            index_name=request.get("index"),
            limit=request.get("limit"),
            cursor=request.get("cursor"),
            scan_forward=_scan_forward(request.get("sort_direction")),
            consistent_read=bool(request.get("consistent_read", False)),
            projection=request.get("projection"),
            filter=_filter_expression(request.get("filter", [])),
        )
        return {"items": [_contract_item(item) for item in page.items], "cursor": page.next_cursor}

    def scan(self, model: str, request: dict[str, Any]) -> dict[str, Any]:
        page = self.tables[model].scan(
            index_name=request.get("index"),
            limit=request.get("limit"),
            cursor=request.get("cursor"),
            consistent_read=bool(request.get("consistent_read", False)),
            projection=request.get("projection"),
            filter=_filter_expression(request.get("filter", [])),
        )
        return {"items": [_contract_item(item) for item in page.items], "cursor": page.next_cursor}

    def transition_append_event(self, actual: dict[str, Any], event: dict[str, Any]) -> None:
        transition_release_state(
            self.tables[actual["model"]],
            self.tables[event["model"]],
            actual_key=actual["key"],
            expected_version=actual.get("expected_version"),
            set_values=actual["set"],
            event_item=self._item(event["model"], event["item"]),
        )

    def validate_provenance(self, model: str, item: dict[str, Any]) -> None:
        if model != "ReleaseStateActual":
            raise ValidationError(f"validate_provenance unsupported for {model}")
        validate_deploy_authority_metadata(item)

    @staticmethod
    def _validate_release_state_metadata_if_present(model: str, item: dict[str, Any]) -> None:
        if model != "ReleaseStateActual":
            return
        if "provenance" in item or "confidence" in item:
            validate_deploy_authority_metadata(item)

    @staticmethod
    def _item(model: str, item: dict[str, Any]) -> Any:
        kwargs = dict(item)
        if model == "User":
            if "tags" in kwargs and kwargs["tags"] is not None:
                kwargs["tags"] = set(cast(list[str], kwargs["tags"]))
            return _User(**kwargs)
        if model == "Order":
            return _Order(**kwargs)
        if model == "ReleaseStateActual":
            return _ReleaseStateActual(**kwargs)
        if model == "ReleaseStateEvent":
            return _ReleaseStateEvent(**kwargs)
        raise AssertionError(f"unknown model: {model}")


@pytest.mark.parametrize("scenario", _load_scenarios(), ids=lambda scenario: str(scenario["name"]))
def test_p0_contract_scenarios_execute_for_python(scenario: dict[str, Any]) -> None:
    models = _load_models()
    missing = _missing_capabilities(scenario, _supported_capabilities())
    if missing:
        pytest.skip(f"scenario requires unsupported capabilities: {', '.join(missing)}")

    client = _dynamodb_client()
    try:
        client.list_tables(Limit=1)
    except EndpointConnectionError:
        if os.environ.get("SKIP_INTEGRATION") in {"1", "true"}:
            pytest.skip("DynamoDB Local not reachable and SKIP_INTEGRATION is set")
        raise

    driver = _TheorydbPyDriver(client)
    _run_scenario(client, driver, scenario, models)


@pytest.mark.parametrize(
    "scenario",
    _load_scenarios_from("interop"),
    ids=lambda scenario: str(scenario["name"]),
)
def test_cross_runtime_interop_scenarios_execute_for_python(scenario: dict[str, Any]) -> None:
    if os.environ.get("CONTRACT_RUN_INTEROP") != "1":
        pytest.skip("set CONTRACT_RUN_INTEROP=1 to run cross-runtime interop scenarios")

    models = _load_models()
    missing = _missing_capabilities(scenario, _supported_capabilities())
    if missing:
        pytest.skip(f"scenario requires unsupported capabilities: {', '.join(missing)}")

    client = _dynamodb_client()
    client.list_tables(Limit=1)

    driver = _TheorydbPyDriver(client)
    _run_scenario(client, driver, scenario, models)


def _supported_capabilities() -> list[str]:
    return [
        "crud",
        "omitempty",
        "lifecycle.timestamps",
        "optimistic_lock.version",
        "ttl.epoch_seconds",
        "query.basic",
        "scan.basic",
        "release_state.write_policy",
        "release_state.transactional_transition",
        "release_state.provenance_confidence",
    ]


def _missing_capabilities(scenario: dict[str, Any], supported: list[str]) -> list[str]:
    supported_set = set(supported)
    return [
        capability
        for capability in scenario.get("requires_capabilities", [])
        if capability not in supported_set
    ]


def _condition_value(condition: dict[str, Any]) -> Any:
    if "values" in condition:
        return condition["values"]
    return condition.get("value")


def _condition_values(condition: dict[str, Any]) -> tuple[Any, ...]:
    if "values" in condition:
        return tuple(condition["values"])
    return (condition.get("value"),)


def _sort_condition(condition: dict[str, Any] | None) -> SortKeyCondition | None:
    if condition is None:
        return None
    values = _condition_values(condition)
    op = str(condition["operator"]).lower()
    if op == "=":
        return SortKeyCondition.eq(values[0])
    if op == "<":
        return SortKeyCondition.lt(values[0])
    if op == "<=":
        return SortKeyCondition.lte(values[0])
    if op == ">":
        return SortKeyCondition.gt(values[0])
    if op == ">=":
        return SortKeyCondition.gte(values[0])
    if op == "between":
        return SortKeyCondition.between(values[0], values[1])
    if op == "begins_with":
        return SortKeyCondition.begins_with(values[0])
    raise ValidationError(f"unsupported sort operator: {condition['operator']}")


def _filter_expression(conditions: list[dict[str, Any]]) -> Any:
    filters = [_filter_condition(condition) for condition in conditions]
    if not filters:
        return None
    if len(filters) == 1:
        return filters[0]
    return FilterGroup.and_(*filters)


def _filter_condition(condition: dict[str, Any]) -> FilterCondition:
    values = _condition_values(condition)
    field = condition["attribute"]
    op = str(condition["operator"]).lower()
    if op == "=":
        return FilterCondition.eq(field, values[0])
    if op in {"!=", "<>"}:
        return FilterCondition.ne(field, values[0])
    if op == "<":
        return FilterCondition.lt(field, values[0])
    if op == "<=":
        return FilterCondition.lte(field, values[0])
    if op == ">":
        return FilterCondition.gt(field, values[0])
    if op == ">=":
        return FilterCondition.gte(field, values[0])
    if op == "between":
        return FilterCondition.between(field, values[0], values[1])
    if op == "begins_with":
        return FilterCondition.begins_with(field, values[0])
    if op == "contains":
        return FilterCondition.contains(field, values[0])
    if op == "in":
        return FilterCondition.in_(field, list(values[0]))
    if op in {"exists", "attribute_exists"}:
        return FilterCondition.exists(field)
    if op in {"not_exists", "attribute_not_exists"}:
        return FilterCondition.not_exists(field)
    raise ValidationError(f"unsupported filter operator: {condition['operator']}")


def _scan_forward(sort_direction: str | None) -> bool:
    return str(sort_direction or "ASC").upper() != "DESC"


def _dynamodb_client() -> Any:
    endpoint = os.environ.get("DYNAMODB_ENDPOINT", "http://localhost:8000")
    region = os.environ.get("AWS_REGION") or os.environ.get("AWS_DEFAULT_REGION") or "us-east-1"
    return boto3.client(
        "dynamodb",
        endpoint_url=endpoint,
        region_name=region,
        aws_access_key_id=os.environ.get("AWS_ACCESS_KEY_ID", "dummy"),
        aws_secret_access_key=os.environ.get("AWS_SECRET_ACCESS_KEY", "dummy"),
    )


def _run_scenario(
    client: Any,
    driver: _TheorydbPyDriver,
    scenario: dict[str, Any],
    models: dict[str, dict[str, Any]],
) -> None:
    model = models[scenario["model"]]
    table_name = scenario.get("table", {}).get("name") or model["table"]["name"]
    steps, recreate = _steps_for_runtime(scenario, "py")
    if recreate:
        _recreate_table(client, table_name, model)
    variables: dict[str, Any] = {}

    for step in steps:
        _run_step(client, driver, step, scenario, models, table_name, variables)


def _steps_for_runtime(scenario: dict[str, Any], runtime_name: str) -> tuple[list[dict[str, Any]], bool]:
    seed_runtime = scenario.get("seed_runtime")
    if not seed_runtime:
        return cast(list[dict[str, Any]], scenario["steps"]), True
    if str(seed_runtime).lower() == runtime_name:
        return (
            cast(list[dict[str, Any]], scenario.get("seed_steps", []))
            + cast(list[dict[str, Any]], scenario.get("read_steps", [])),
            True,
        )
    return cast(list[dict[str, Any]], scenario.get("read_steps", [])), False


def _run_step(
    client: Any,
    driver: _TheorydbPyDriver,
    step: dict[str, Any],
    scenario: dict[str, Any],
    models: dict[str, dict[str, Any]],
    table_name: str,
    variables: dict[str, Any],
) -> None:
    if step["op"] == "sleep":
        if step.get("ms", 0) > 0:
            time.sleep(step["ms"] / 1000.0)
        return

    model_name = step.get("model") or scenario["model"]
    model = models[model_name]

    if step["op"] == "create":
        error = _capture_error(
            lambda: driver.create(model_name, step["item"], if_not_exists=step.get("if_not_exists", False))
        )
        _assert_expectation(step.get("expect", {}), error=error, model=model, variables=variables)
        return

    if step["op"] == "update":
        error = _capture_error(
            lambda: driver.update(
                model_name,
                step["item"],
                step.get("fields", []),
                protected_attributes=step.get("protected_attributes", []),
            )
        )
        _assert_expectation(step.get("expect", {}), error=error, model=model, variables=variables)
        return

    if step["op"] == "save":
        error = _capture_error(lambda: driver.save(model_name, step["item"]))
        _assert_expectation(step.get("expect", {}), error=error, model=model, variables=variables)
        return

    if step["op"] == "delete":
        error = _capture_error(lambda: driver.delete(model_name, step["key"]))
        _assert_expectation(step.get("expect", {}), error=error, model=model, variables=variables)
        return

    if step["op"] == "get":
        item, error = _capture_result(lambda: driver.get(model_name, step["key"]))
        raw = None if error is not None else _get_raw_item(client, table_name, model, step["key"])
        if error is None:
            for variable_name, attr in step.get("save", {}).items():
                variables[variable_name] = item[attr]
        _assert_expectation(
            step.get("expect", {}), error=error, item=item, raw=raw, model=model, variables=variables
        )
        return

    if step["op"] == "query":
        result, error = _capture_result(lambda: driver.query(model_name, step["query"]))
        _assert_read_expectation(step.get("expect", {}), error=error, result=result, model=model)
        return

    if step["op"] == "scan":
        result, error = _capture_result(lambda: driver.scan(model_name, step["scan"]))
        _assert_read_expectation(step.get("expect", {}), error=error, result=result, model=model)
        return

    if step["op"] == "transition_append_event":
        error = _capture_error(lambda: driver.transition_append_event(step["actual"], step["event"]))
        _assert_expectation(step.get("expect", {}), error=error, model=model, variables=variables)
        return

    if step["op"] == "validate_provenance":
        error = _capture_error(lambda: driver.validate_provenance(model_name, step["item"]))
        _assert_expectation(step.get("expect", {}), error=error, model=model, variables=variables)
        return

    raise AssertionError(f"unsupported op: {step['op']}")


def _capture_error(fn: Any) -> Exception | None:
    try:
        fn()
    except Exception as err:  # noqa: BLE001 - scenario runner maps expected runtime errors.
        return err
    return None


def _capture_result(fn: Any) -> tuple[dict[str, Any], Exception | None]:
    try:
        return fn(), None
    except Exception as err:  # noqa: BLE001 - scenario runner maps expected runtime errors.
        return {}, err


def _assert_expectation(
    expect: dict[str, Any],
    *,
    error: Exception | None,
    model: dict[str, Any],
    variables: dict[str, Any],
    item: dict[str, Any] | None = None,
    raw: dict[str, Any] | None = None,
) -> None:
    has_item_assertion = _expectation_has_any_key(
        expect,
        {
            "item_contains",
            "item_equals",
            "item_has_fields",
            "item_missing_fields",
            "raw_attribute_types",
            "item_field_equals_var",
            "item_field_not_equals_var",
        },
    )
    has_raw_assertion = _expectation_has_any_key(expect, {"item_missing_fields", "raw_attribute_types"})

    if "error" in expect:
        assert error is not None
        assert _map_error(error) == expect["error"]
        assert not has_item_assertion, "item assertions cannot be combined with error expectations"
        return

    if has_item_assertion:
        assert error is None, "expected successful operation for item assertions"
        assert item is not None, "expected item for item assertions"
    if has_raw_assertion:
        assert raw is not None, "expected raw item for raw assertions"

    if "ok" in expect:
        if expect["ok"]:
            assert error is None
        else:
            assert error is not None
    if error is not None:
        return

    if "item_contains" in expect:
        assert item is not None
        for attr, want in expect["item_contains"].items():
            assert attr in item
            attr_def = _attribute_by_name(model, attr)
            assert attr_def is not None
            raw_value = None if raw is None else raw.get(attr)
            _assert_value_matches(attr_def["type"], want, item[attr], raw_value)

    if "item_equals" in expect:
        assert item is not None
        _assert_item_equals(expect["item_equals"], item, raw, model)

    if "item_has_fields" in expect:
        assert item is not None
        for attr in expect["item_has_fields"]:
            assert attr in item

    if "item_missing_fields" in expect:
        assert raw is not None
        for attr in expect["item_missing_fields"]:
            assert attr not in raw

    if "raw_attribute_types" in expect:
        assert raw is not None
        for attr, want_type in expect["raw_attribute_types"].items():
            assert attr in raw
            assert _attribute_value_type_name(raw[attr]) == want_type

    if "item_field_equals_var" in expect:
        assert item is not None
        for attr, variable_name in expect["item_field_equals_var"].items():
            assert item[attr] == variables[variable_name]

    if "item_field_not_equals_var" in expect:
        assert item is not None
        for attr, variable_name in expect["item_field_not_equals_var"].items():
            assert item[attr] != variables[variable_name]


def _assert_read_expectation(
    expect: dict[str, Any],
    *,
    error: Exception | None,
    result: dict[str, Any],
    model: dict[str, Any],
) -> None:
    has_read_assertion = _expectation_has_any_key(expect, {"item_count", "items_contains", "cursor_equals"})

    if "error" in expect:
        assert error is not None
        assert _map_error(error) == expect["error"]
        assert not has_read_assertion, "read assertions cannot be combined with error expectations"
        return

    if has_read_assertion:
        assert error is None, "expected successful read for read assertions"
        assert result is not None, "expected read result for read assertions"

    if "ok" in expect:
        if expect["ok"]:
            assert error is None
        else:
            assert error is not None
    if error is not None:
        return

    items = result.get("items", [])

    if "item_count" in expect:
        assert len(items) == expect["item_count"]

    if "items_contains" in expect:
        assert len(items) >= len(expect["items_contains"])
        for index, want_item in enumerate(expect["items_contains"]):
            have_item = items[index]
            for attr, want in want_item.items():
                assert attr in have_item
                attr_def = _attribute_by_name(model, attr)
                assert attr_def is not None
                _assert_value_matches(attr_def["type"], want, have_item[attr])

    if "cursor_equals" in expect:
        assert (result.get("cursor") or "") == expect["cursor_equals"]


def _expectation_has_any_key(expect: dict[str, Any], keys: set[str]) -> bool:
    return any(key in expect for key in keys)


def test_assert_expectation_fails_closed_on_item_assertion_error_without_ok() -> None:
    with pytest.raises(AssertionError, match="expected successful operation for item assertions"):
        _assert_expectation(
            {"item_equals": {"PK": "USER#fail-closed"}},
            error=RuntimeError("missing seeded item"),
            item=None,
            raw=None,
            model=_minimal_user_model(),
            variables={},
        )


def test_assert_read_expectation_fails_closed_on_read_assertion_error_without_ok() -> None:
    with pytest.raises(AssertionError, match="expected successful read for read assertions"):
        _assert_read_expectation(
            {"items_contains": [{"PK": "USER#fail-closed"}]},
            error=RuntimeError("read failed"),
            result={},
            model=_minimal_user_model(),
        )


def _minimal_user_model() -> dict[str, Any]:
    return {
        "name": "User",
        "attributes": [
            {"attribute": "PK", "type": "S"},
            {"attribute": "SK", "type": "S"},
        ],
    }


def _assert_item_equals(
    want: dict[str, Any],
    item: dict[str, Any],
    raw: dict[str, Any] | None,
    model: dict[str, Any],
) -> None:
    if raw is not None:
        assert sorted(raw.keys()) == sorted(want.keys())
        for attr, want_value in want.items():
            assert attr in raw
            attr_def = _attribute_by_name(model, attr)
            assert attr_def is not None
            _assert_value_matches(attr_def["type"], want_value, item.get(attr), raw[attr])
        return

    assert sorted(item.keys()) == sorted(want.keys())
    for attr, want_value in want.items():
        assert attr in item
        attr_def = _attribute_by_name(model, attr)
        assert attr_def is not None
        _assert_value_matches(attr_def["type"], want_value, item[attr])


def _map_error(error: Exception) -> str:
    if isinstance(error, NotFoundError):
        return "ErrItemNotFound"
    if isinstance(error, ConditionFailedError):
        return "ErrConditionFailed"
    if isinstance(error, ImmutableModelMutationError):
        return "ErrImmutableModelMutation"
    if isinstance(error, ProtectedFieldMutationError):
        return "ErrProtectedFieldMutation"
    if isinstance(error, RejectedDeployAuthorityEvidenceError):
        return "ErrRejectedDeployAuthorityEvidence"
    if isinstance(error, ValidationError):
        return "ErrInvalidModel"
    return ""


def _expected_version(table: Table[Any], item: dict[str, Any]) -> int | None:
    for attr in table._model.attributes.values():
        if "version" in attr.roles:
            value = item.get(attr.python_name, item.get(attr.attribute_name))
            return None if value is None else int(value)
    return None


def _contract_item(value: Any) -> dict[str, Any]:
    if is_dataclass(value):
        return _normalize_contract_value(asdict(value))
    assert isinstance(value, dict)
    return _normalize_contract_value(value)


def _normalize_contract_value(value: Any) -> Any:
    if isinstance(value, set):
        return sorted(str(item) for item in value)
    if isinstance(value, Decimal):
        return int(value) if value == value.to_integral_value() else float(value)
    if isinstance(value, dict):
        return {key: _normalize_contract_value(val) for key, val in value.items()}
    if isinstance(value, list):
        return [_normalize_contract_value(item) for item in value]
    return value


def _assert_value_matches(attr_type: str, want: Any, have: Any, raw: dict[str, Any] | None = None) -> None:
    if attr_type == "S":
        if raw is not None:
            assert "S" in raw
            assert raw["S"] == str(want)
            return
        assert str(have) == str(want)
        return
    if attr_type == "N":
        if raw is not None:
            assert "N" in raw
            assert raw["N"] == _canonical_decimal_string(want)
            return
        assert _canonical_decimal_string(have) == _canonical_decimal_string(want)
        return
    if attr_type == "SS":
        if raw is not None:
            assert "SS" in raw
            assert sorted(str(item) for item in raw["SS"]) == sorted(str(item) for item in want)
            return
        assert sorted(str(item) for item in have) == sorted(str(item) for item in want)
        return
    assert have == want


def _canonical_decimal_string(value: Any) -> str:
    if isinstance(value, str):
        return value
    if isinstance(value, bool):
        raise AssertionError("DynamoDB N expectations must not be boolean")
    if isinstance(value, int):
        return str(value)
    if isinstance(value, Decimal):
        return format(value, "f")
    if isinstance(value, float):
        if not Decimal(str(value)).is_finite():
            raise AssertionError("DynamoDB N expectations must be finite")
        text = format(Decimal(str(value)), "f")
        if "E" in text or "e" in text:
            raise AssertionError("DynamoDB N expectations must use canonical decimal strings")
        return text
    raise AssertionError(f"expected DynamoDB canonical decimal string, got {type(value).__name__}")


def _attribute_by_name(model: dict[str, Any], name: str) -> dict[str, Any] | None:
    for attr in model.get("attributes", []):
        if attr["attribute"] == name:
            return cast(dict[str, Any], attr)
    return None


def _get_raw_item(
    client: Any,
    table_name: str,
    model: dict[str, Any],
    key: dict[str, Any],
) -> dict[str, Any]:
    out = client.get_item(TableName=table_name, Key=_marshal_key(model, key), ConsistentRead=True)
    item = out.get("Item")
    if item is None:
        raise AssertionError("raw GetItem returned no Item")
    return cast(dict[str, Any], item)


def _marshal_key(model: dict[str, Any], key: dict[str, Any]) -> dict[str, Any]:
    serializer = TypeSerializer()
    out: dict[str, Any] = {}
    partition = model["keys"]["partition"]
    out[partition["attribute"]] = serializer.serialize(key[partition["attribute"]])
    sort = model["keys"].get("sort")
    if sort is not None:
        out[sort["attribute"]] = serializer.serialize(key[sort["attribute"]])
    return out


def _attribute_value_type_name(value: dict[str, Any]) -> str:
    for name in ("S", "N", "B", "BOOL", "NULL", "SS", "NS", "BS", "L", "M"):
        if name in value:
            return name
    return "UNKNOWN"


def _recreate_table(client: Any, table_name: str, model: dict[str, Any]) -> None:
    try:
        client.delete_table(TableName=table_name)
    except ClientError as err:
        if err.response.get("Error", {}).get("Code") != "ResourceNotFoundException":
            raise
    _wait_table_not_exists(client, table_name)
    client.create_table(**_create_table_input(table_name, model))
    _wait_table_exists(client, table_name)


def _wait_table_exists(client: Any, table_name: str) -> None:
    for _ in range(60):
        try:
            resp = client.describe_table(TableName=table_name)
        except ClientError as err:
            if err.response.get("Error", {}).get("Code") != "ResourceNotFoundException":
                raise
        else:
            if resp.get("Table", {}).get("TableStatus") == "ACTIVE":
                return
        time.sleep(0.5)
    raise AssertionError(f"timeout waiting for table exists: {table_name}")


def _wait_table_not_exists(client: Any, table_name: str) -> None:
    for _ in range(40):
        try:
            client.describe_table(TableName=table_name)
        except ClientError as err:
            if err.response.get("Error", {}).get("Code") == "ResourceNotFoundException":
                return
            raise
        time.sleep(0.25)


def _create_table_input(table_name: str, model: dict[str, Any]) -> dict[str, Any]:
    definitions: dict[str, str] = {}

    def add_def(attr: dict[str, str]) -> None:
        definitions[attr["attribute"]] = attr["type"]

    add_def(model["keys"]["partition"])
    if model["keys"].get("sort") is not None:
        add_def(model["keys"]["sort"])
    for index in model.get("indexes", []):
        add_def(index["partition"])
        if index.get("sort") is not None:
            add_def(index["sort"])

    key_schema = [{"AttributeName": model["keys"]["partition"]["attribute"], "KeyType": "HASH"}]
    if model["keys"].get("sort") is not None:
        key_schema.append({"AttributeName": model["keys"]["sort"]["attribute"], "KeyType": "RANGE"})

    input_value: dict[str, Any] = {
        "TableName": table_name,
        "AttributeDefinitions": [
            {"AttributeName": name, "AttributeType": type_} for name, type_ in sorted(definitions.items())
        ],
        "KeySchema": key_schema,
        "BillingMode": "PAY_PER_REQUEST",
    }

    global_secondary_indexes: list[dict[str, Any]] = []
    local_secondary_indexes: list[dict[str, Any]] = []
    for index in model.get("indexes", []):
        index_key_schema = [{"AttributeName": index["partition"]["attribute"], "KeyType": "HASH"}]
        if index.get("sort") is not None:
            index_key_schema.append({"AttributeName": index["sort"]["attribute"], "KeyType": "RANGE"})
        projection = {
            "ProjectionType": index.get("projection", {}).get("type", "ALL"),
        }
        fields = index.get("projection", {}).get("fields")
        if fields:
            projection["NonKeyAttributes"] = fields
        index_input = {
            "IndexName": index["name"],
            "KeySchema": index_key_schema,
            "Projection": projection,
        }
        if index["type"] == "LSI":
            local_secondary_indexes.append(index_input)
        else:
            global_secondary_indexes.append(index_input)

    if global_secondary_indexes:
        input_value["GlobalSecondaryIndexes"] = global_secondary_indexes
    if local_secondary_indexes:
        input_value["LocalSecondaryIndexes"] = local_secondary_indexes
    return input_value
