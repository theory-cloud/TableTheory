from __future__ import annotations

import base64
import hashlib
import hmac
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
    EncryptionNotConfiguredError,
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
    VersionConflictError,
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
class _NumberPrecision:
    PK: str = theorydb_field(name="PK", roles=["pk"])
    SK: str = theorydb_field(name="SK", roles=["sk"])
    largeInteger: Decimal = Decimal(0)
    preciseDecimal: Decimal = Decimal(0)


@dataclass(frozen=True)
class _TypeMatrix:
    PK: str = theorydb_field(name="PK", roles=["pk"])
    SK: str = theorydb_field(name="SK", roles=["sk"])
    numberSet: set[Decimal] | None = theorydb_field(set_=True, default=None)
    binaryBlob: bytes | None = theorydb_field(default=None)
    binarySet: set[bytes] | None = theorydb_field(set_=True, default=None)
    flag: bool | None = None
    nothing: Any = None
    items: list[Any] | None = None
    metadata: dict[str, Any] | None = None
    emptyNumberSet: set[Decimal] | None = theorydb_field(set_=True, default=None)
    emptyBinarySet: set[bytes] | None = theorydb_field(set_=True, default=None)
    optionalString: str | None = None


@dataclass(frozen=True)
class _SnakeCaseRecord:
    pk: str = theorydb_field(roles=["pk"])
    sk: str = theorydb_field(roles=["sk"])
    display_name: str | None = None
    email_hash: str | None = None


@dataclass(frozen=True)
class _EncryptedRecord:
    PK: str = theorydb_field(name="PK", roles=["pk"])
    SK: str = theorydb_field(name="SK", roles=["sk"])
    secret: str | None = theorydb_field(encrypted=True, default=None)


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


class _DeterministicEncryptionMaterial:
    def __init__(self, seed: str, *, model: str, attribute: str) -> None:
        self._master = hashlib.sha256(seed.encode("utf-8")).digest()
        self._model = model
        self._attribute = attribute
        self._counter = 0

    def next_bytes(self, label: str, n: int) -> bytes:
        self._counter += 1
        digest = hmac.new(self._master, f"{label}|{self._counter}".encode("utf-8"), hashlib.sha256).digest()
        return digest[:n]

    def data_key(self, edk: bytes) -> bytes:
        return hmac.new(self._master, edk, hashlib.sha256).digest()

    def generate_data_key(self, *, KeyId: str, KeySpec: str) -> dict[str, bytes | str]:
        del KeyId, KeySpec
        edk = self.next_bytes(f"edk|{self._model}|{self._attribute}", 32)
        return {"Plaintext": self.data_key(edk), "CiphertextBlob": edk}

    def decrypt(self, *, CiphertextBlob: bytes, KeyId: str | None = None) -> dict[str, bytes]:
        del KeyId
        return {"Plaintext": self.data_key(bytes(CiphertextBlob))}

    def rand_bytes(self, n: int) -> bytes:
        return self.next_bytes(f"nonce|{self._model}|{self._attribute}", n)


def _deterministic_encryption_material(
    config: dict[str, Any] | None,
) -> _DeterministicEncryptionMaterial | None:
    if not config:
        return None
    if config.get("provider") != "deterministic":
        raise ValidationError(f"unsupported scenario encryption provider: {config.get('provider')!r}")
    seed = config.get("seed")
    if not isinstance(seed, str) or not seed:
        raise ValidationError("deterministic scenario encryption requires seed")
    return _DeterministicEncryptionMaterial(seed, model="EncryptedRecord", attribute="secret")


class _TheorydbPyDriver:
    def __init__(self, client: Any, *, encryption: dict[str, Any] | None = None) -> None:
        self.client = client
        self.encryption = _deterministic_encryption_material(encryption)
        self.tables: dict[str, Table[Any]] = {
            "User": Table(
                ModelDefinition.from_dataclass(
                    _User,
                    table_name="users_contract",
                    indexes=[gsi("gsi-email", partition="emailHash", projection=Projection.keys_only())],
                ),
                client=client,
            ),
            "Order": Table(
                ModelDefinition.from_dataclass(
                    _Order,
                    table_name="orders_contract",
                    indexes=[
                        gsi(
                            "gsi-status",
                            partition="status",
                            sort="createdAt",
                            projection=Projection.include("amount"),
                        )
                    ],
                ),
                client=client,
            ),
            "NumberPrecision": Table(
                ModelDefinition.from_dataclass(
                    _NumberPrecision,
                    table_name="number_precision_contract",
                ),
                client=client,
            ),
            "TypeMatrix": Table(
                ModelDefinition.from_dataclass(
                    _TypeMatrix,
                    table_name="type_matrix_contract",
                ),
                client=client,
            ),
            "SnakeCaseRecord": Table(
                ModelDefinition.from_dataclass(
                    _SnakeCaseRecord,
                    table_name="snake_case_contract",
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

    def _table(self, model: str) -> Table[Any]:
        if model == "EncryptedRecord":
            kwargs: dict[str, Any] = {}
            if self.encryption is not None:
                kwargs = {
                    "kms_key_arn": "arn:aws:kms:us-east-1:111111111111:key/contract-deterministic",
                    "kms_client": self.encryption,
                    "rand_bytes": self.encryption.rand_bytes,
                }
            return Table(
                ModelDefinition.from_dataclass(
                    _EncryptedRecord,
                    table_name="encrypted_records_contract",
                ),
                client=self.client,
                **kwargs,
            )
        return self.tables[model]

    def create(self, model: str, item: dict[str, Any], *, if_not_exists: bool = False) -> None:
        self._validate_release_state_metadata_if_present(model, item)
        table = self._table(model)
        kwargs: dict[str, Any] = {}
        if if_not_exists:
            kwargs = {
                "condition_expression": "attribute_not_exists(#pk)",
                "expression_attribute_names": {"#pk": table._model.pk.attribute_name},
            }
        table.put(self._item(model, item), **kwargs)

    def get(self, model: str, key: dict[str, Any]) -> dict[str, Any]:
        table = self._table(model)
        return _contract_item(table.get(_pk_value(key), _sk_value(key), consistent_read=True))

    def get_optional(self, model: str, key: dict[str, Any]) -> dict[str, Any] | None:
        del model, key
        raise ValidationError("optional get is not advertised")

    def update(
        self,
        model: str,
        item: dict[str, Any],
        fields: list[str],
        *,
        protected_attributes: list[str] | None = None,
    ) -> None:
        table = self._table(model)
        updates = {field: item[field] for field in fields if field in item}
        expected_version = _expected_version(table, item)
        table.update(
            _pk_value(item),
            _sk_value(item),
            updates,
            expected_version=expected_version,
            protected_attributes=protected_attributes or [],
        )

    def save(self, model: str, item: dict[str, Any]) -> None:
        self._validate_release_state_metadata_if_present(model, item)
        self._table(model).save(self._item(model, item))

    def delete(self, model: str, key: dict[str, Any]) -> None:
        self._table(model).delete(_pk_value(key), _sk_value(key))

    def query(self, model: str, request: dict[str, Any]) -> dict[str, Any]:
        partition = request.get("partition")
        if partition is None:
            raise ValidationError("query partition is required")
        page = self._table(model).query(
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
        page = self._table(model).scan(
            index_name=request.get("index"),
            limit=request.get("limit"),
            cursor=request.get("cursor"),
            consistent_read=bool(request.get("consistent_read", False)),
            projection=request.get("projection"),
            filter=_filter_expression(request.get("filter", [])),
        )
        return {"items": [_contract_item(item) for item in page.items], "cursor": page.next_cursor}

    def count_query(self, model: str, request: dict[str, Any]) -> dict[str, Any]:
        partition = request.get("partition")
        if partition is None:
            raise ValidationError("query partition is required")
        count = self._table(model).query_count(
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
        return {"items": [], "count": count}

    def count_scan(self, model: str, request: dict[str, Any]) -> dict[str, Any]:
        count = self._table(model).scan_count(
            index_name=request.get("index"),
            limit=request.get("limit"),
            cursor=request.get("cursor"),
            consistent_read=bool(request.get("consistent_read", False)),
            projection=request.get("projection"),
            filter=_filter_expression(request.get("filter", [])),
        )
        return {"items": [], "count": count}

    def transition_append_event(self, actual: dict[str, Any], event: dict[str, Any]) -> None:
        transition_release_state(
            self._table(actual["model"]),
            self._table(event["model"]),
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
        if model == "NumberPrecision":
            if "largeInteger" in kwargs and kwargs["largeInteger"] is not None:
                kwargs["largeInteger"] = Decimal(str(kwargs["largeInteger"]))
            if "preciseDecimal" in kwargs and kwargs["preciseDecimal"] is not None:
                kwargs["preciseDecimal"] = Decimal(str(kwargs["preciseDecimal"]))
            return _NumberPrecision(**kwargs)
        if model == "TypeMatrix":
            if "numberSet" in kwargs and kwargs["numberSet"] is not None:
                kwargs["numberSet"] = {Decimal(str(item)) for item in cast(list[Any], kwargs["numberSet"])}
            if "binaryBlob" in kwargs and kwargs["binaryBlob"] is not None:
                kwargs["binaryBlob"] = base64.b64decode(cast(str, kwargs["binaryBlob"]))
            if "binarySet" in kwargs and kwargs["binarySet"] is not None:
                kwargs["binarySet"] = {
                    base64.b64decode(cast(str, item)) for item in cast(list[Any], kwargs["binarySet"])
                }
            if "emptyNumberSet" in kwargs and kwargs["emptyNumberSet"] is not None:
                kwargs["emptyNumberSet"] = {
                    Decimal(str(item)) for item in cast(list[Any], kwargs["emptyNumberSet"])
                }
            if "emptyBinarySet" in kwargs and kwargs["emptyBinarySet"] is not None:
                kwargs["emptyBinarySet"] = {
                    base64.b64decode(cast(str, item)) for item in cast(list[Any], kwargs["emptyBinarySet"])
                }
            return _TypeMatrix(**kwargs)
        if model == "SnakeCaseRecord":
            return _SnakeCaseRecord(**kwargs)
        if model == "EncryptedRecord":
            return _EncryptedRecord(**kwargs)
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

    driver = _TheorydbPyDriver(client, encryption=scenario.get("encryption"))
    _run_scenario(client, driver, scenario, models)


@pytest.mark.parametrize("scenario", _load_scenarios_from("p1"), ids=lambda scenario: str(scenario["name"]))
def test_p1_contract_scenarios_execute_for_python(scenario: dict[str, Any]) -> None:
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

    driver = _TheorydbPyDriver(client, encryption=scenario.get("encryption"))
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

    driver = _TheorydbPyDriver(client, encryption=scenario.get("encryption"))
    _run_scenario(client, driver, scenario, models)


def _supported_capabilities() -> list[str]:
    return [
        "crud",
        "omitempty",
        "lifecycle.timestamps",
        "optimistic_lock.version",
        "error.version_conflict",
        "ttl.epoch_seconds",
        "number.precision.exact",
        "type.matrix",
        "query.basic",
        "scan.basic",
        "count.native",
        "release_state.write_policy",
        "release_state.transactional_transition",
        "release_state.provenance_confidence",
        "encryption.fail_closed",
        "encryption.deterministic_interop",
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


def _pk_value(values: dict[str, Any]) -> Any:
    if "PK" in values:
        return values["PK"]
    return values["pk"]


def _sk_value(values: dict[str, Any]) -> Any:
    if "SK" in values:
        return values["SK"]
    return values["sk"]


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

    if step["op"] == "get_optional":
        item, error = _capture_result(lambda: driver.get_optional(model_name, step["key"]))
        raw = (
            None
            if error is not None or item is None
            else _get_raw_item(client, table_name, model, step["key"])
        )
        if error is None and item is not None:
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

    if step["op"] == "count":
        count_request = step["count"]
        if "query" in count_request:
            result, error = _capture_result(lambda: driver.count_query(model_name, count_request["query"]))
        else:
            result, error = _capture_result(lambda: driver.count_scan(model_name, count_request["scan"]))
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
            "item_absent",
            "item_has_fields",
            "item_missing_fields",
            "raw_attribute_types",
            "raw_item_contains",
            "item_field_equals_var",
            "item_field_not_equals_var",
        },
    )
    has_raw_assertion = _expectation_has_any_key(
        expect, {"item_missing_fields", "raw_attribute_types", "raw_item_contains"}
    )

    if "error" in expect:
        assert error is not None
        assert _map_error(error) == expect["error"]
        assert not has_item_assertion, "item assertions cannot be combined with error expectations"
        return
    if "errors" in expect:
        assert error is not None
        assert sorted(_map_errors(error)) == sorted(expect["errors"])
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

    if "item_absent" in expect:
        if expect["item_absent"]:
            assert item is None
        else:
            assert item is not None

    if "item_contains" in expect:
        assert item is not None
        for attr, want in expect["item_contains"].items():
            assert attr in item
            attr_def = _attribute_by_name(model, attr)
            assert attr_def is not None
            raw_value = None if raw is None else raw.get(attr)
            if attr_def.get("encryption") is not None:
                raw_value = None
            _assert_value_matches(attr_def["type"], want, item[attr], raw_value)

    if "item_equals" in expect:
        assert item is not None
        _assert_item_equals(expect["item_equals"], item, raw, model)

    if "raw_item_contains" in expect:
        assert raw is not None
        for attr, want in expect["raw_item_contains"].items():
            assert attr in raw
            assert _attribute_value_to_contract(raw[attr]) == _normalize_document(want)

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
    has_read_assertion = _expectation_has_any_key(
        expect, {"item_count", "count", "items_contains", "items_missing_fields", "cursor_equals"}
    )

    if "error" in expect:
        assert error is not None
        assert _map_error(error) == expect["error"]
        assert not has_read_assertion, "read assertions cannot be combined with error expectations"
        return
    if "errors" in expect:
        assert error is not None
        assert sorted(_map_errors(error)) == sorted(expect["errors"])
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

    if "count" in expect:
        assert result.get("count") == expect["count"]

    if "items_contains" in expect:
        assert len(items) >= len(expect["items_contains"])
        for index, want_item in enumerate(expect["items_contains"]):
            have_item = items[index]
            for attr, want in want_item.items():
                assert attr in have_item
                attr_def = _attribute_by_name(model, attr)
                assert attr_def is not None
                _assert_value_matches(attr_def["type"], want, have_item[attr])

    if "items_missing_fields" in expect:
        for index, item in enumerate(items):
            for attr in expect["items_missing_fields"]:
                assert _semantically_missing_read_field(item.get(attr), attr in item), (
                    f"expected missing attr {attr} in result {index}"
                )

    if "cursor_equals" in expect:
        assert (result.get("cursor") or "") == expect["cursor_equals"]


def _semantically_missing_read_field(value: Any, present: bool) -> bool:
    if not present or value is None:
        return True
    if isinstance(value, str):
        return value == ""
    if isinstance(value, bool):
        return value is False
    if isinstance(value, int | float | Decimal):
        return value == 0
    if isinstance(value, bytes):
        return len(value) == 0
    if isinstance(value, (list, dict, set, tuple)):
        return len(value) == 0
    return False


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
            raw_value = None if attr_def.get("encryption") is not None else raw[attr]
            _assert_value_matches(attr_def["type"], want_value, item.get(attr), raw_value)
        return

    assert sorted(item.keys()) == sorted(want.keys())
    for attr, want_value in want.items():
        assert attr in item
        attr_def = _attribute_by_name(model, attr)
        assert attr_def is not None
        _assert_value_matches(attr_def["type"], want_value, item[attr])


def _map_error(error: Exception) -> str:
    errors = _map_errors(error)
    return errors[0] if errors else ""


def _map_errors(error: Exception) -> list[str]:
    if isinstance(error, NotFoundError):
        return ["ErrItemNotFound"]
    if isinstance(error, EncryptionNotConfiguredError):
        return ["ErrEncryptionNotConfigured"]
    if isinstance(error, VersionConflictError):
        return ["ErrConditionFailed", "ErrVersionConflict"]
    if isinstance(error, ConditionFailedError):
        return ["ErrConditionFailed"]
    if isinstance(error, ImmutableModelMutationError):
        return ["ErrImmutableModelMutation"]
    if isinstance(error, ProtectedFieldMutationError):
        return ["ErrProtectedFieldMutation"]
    if isinstance(error, RejectedDeployAuthorityEvidenceError):
        return ["ErrRejectedDeployAuthorityEvidence"]
    if isinstance(error, ValidationError):
        if (
            "consistent_read" in str(error)
            or "unsupported sort operator" in str(error)
            or "unsupported filter operator" in str(error)
        ):
            return ["ErrInvalidOperator"]
        return ["ErrInvalidModel"]
    return []


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
    if _is_binary_value(value):
        return _base64_string(value)
    if isinstance(value, set):
        if all(_is_binary_value(item) for item in value):
            return sorted(_base64_string(item) for item in value)
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
    if attr_type == "B":
        want_b = _base64_string(want)
        if raw is not None:
            assert "B" in raw
            assert _base64_string(raw["B"]) == want_b
            return
        assert _base64_string(have) == want_b
        return
    if attr_type == "BOOL":
        if raw is not None:
            assert "BOOL" in raw
            assert raw["BOOL"] is want
            return
        assert have is want
        return
    if attr_type == "NULL":
        if raw is not None:
            assert raw.get("NULL") is True
            return
        assert have is None
        return
    if attr_type == "SS":
        if raw is not None:
            assert "SS" in raw
            assert sorted(str(item) for item in raw["SS"]) == sorted(str(item) for item in want)
            return
        assert sorted(str(item) for item in have) == sorted(str(item) for item in want)
        return
    if attr_type == "NS":
        want_ns = sorted(_canonical_decimal_string(item) for item in (want or []))
        if raw is not None:
            if raw.get("NULL") is True:
                assert want_ns == []
                return
            assert "NS" in raw
            assert sorted(str(item) for item in raw["NS"]) == want_ns
            return
        assert sorted(_canonical_decimal_string(item) for item in (have or [])) == want_ns
        return
    if attr_type == "BS":
        want_bs = sorted(_base64_string(item) for item in (want or []))
        if raw is not None:
            if raw.get("NULL") is True:
                assert want_bs == []
                return
            assert "BS" in raw
            assert sorted(_base64_string(item) for item in raw["BS"]) == want_bs
            return
        assert sorted(_base64_string(item) for item in (have or [])) == want_bs
        return
    if attr_type in {"L", "M"}:
        expected = _normalize_document(want)
        actual = _attribute_value_to_contract(raw) if raw is not None else _normalize_document(have)
        assert actual == expected
        return
    assert have == want


def _is_binary_value(value: Any) -> bool:
    return isinstance(value, bytes) or (hasattr(value, "value") and isinstance(value.value, bytes))


def _base64_string(value: Any) -> str:
    if isinstance(value, str):
        return value
    if isinstance(value, bytes):
        return base64.b64encode(value).decode("ascii")
    if hasattr(value, "value") and isinstance(value.value, bytes):
        return base64.b64encode(value.value).decode("ascii")
    raise AssertionError(f"expected base64 string or bytes, got {type(value).__name__}")


def _normalize_document(value: Any) -> Any:
    if isinstance(value, bytes):
        return _base64_string(value)
    if isinstance(value, Decimal):
        return _canonical_decimal_string(value)
    if isinstance(value, dict):
        return {str(key): _normalize_document(val) for key, val in value.items()}
    if isinstance(value, list):
        return [_normalize_document(item) for item in value]
    return value


def _attribute_value_to_contract(value: dict[str, Any]) -> Any:
    if "S" in value:
        return value["S"]
    if "N" in value:
        return value["N"]
    if "B" in value:
        return _base64_string(value["B"])
    if "BOOL" in value:
        return value["BOOL"]
    if value.get("NULL") is True:
        return None
    if "SS" in value:
        return sorted(str(item) for item in value["SS"])
    if "NS" in value:
        return sorted(str(item) for item in value["NS"])
    if "BS" in value:
        return sorted(_base64_string(item) for item in value["BS"])
    if "L" in value:
        return [_attribute_value_to_contract(cast(dict[str, Any], item)) for item in value["L"]]
    if "M" in value:
        return {
            str(key): _attribute_value_to_contract(cast(dict[str, Any], item))
            for key, item in value["M"].items()
        }
    raise AssertionError(f"unsupported AttributeValue: {value}")


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
