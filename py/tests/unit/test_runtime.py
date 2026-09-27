from __future__ import annotations

import re
import threading
from dataclasses import dataclass
from datetime import UTC, datetime

import pytest

import tabletheory_py.runtime as runtime
from tabletheory_py.mocks import FakeDynamoDBClient
from tabletheory_py.model import ModelDefinition, theorydb_field
from tabletheory_py.runtime import (
    DEFAULT_LAMBDA_TIMEOUT_BUFFER_SECONDS,
    LambdaTimeoutConfig,
    _reset_lambda_clients_for_tests,
    check_lambda_timeout,
    create_lambda_boto3_config,
    get_lambda_boto3_client,
    instrument_boto3_client,
    is_lambda_environment,
    with_lambda_timeout,
)
from tabletheory_py.table import Table


@dataclass(frozen=True)
class RuntimeItem:
    pk: str = theorydb_field(name="PK", roles=["pk"])
    sk: str = theorydb_field(name="SK", roles=["sk"])
    value: int = theorydb_field(name="value")


class _LambdaContext:
    def __init__(self, remaining_ms: int | None) -> None:
        self.remaining_ms = remaining_ms

    def get_remaining_time_in_millis(self) -> int | None:
        return self.remaining_ms


class _CamelLambdaContext:
    def __init__(self, remaining_ms: int) -> None:
        self.remaining_ms = remaining_ms

    def getRemainingTimeInMillis(self) -> int:
        return self.remaining_ms


def test_is_lambda_environment() -> None:
    assert is_lambda_environment({}) is False
    assert is_lambda_environment({"AWS_LAMBDA_FUNCTION_NAME": "fn"}) is True
    assert is_lambda_environment({"AWS_EXECUTION_ENV": "AWS_Lambda_python3.14"}) is True


def test_create_lambda_boto3_config() -> None:
    cfg = create_lambda_boto3_config(connect_timeout=2.0, read_timeout=4.0, max_attempts=3)
    assert cfg.connect_timeout == 2.0
    assert cfg.read_timeout == 4.0
    assert cfg.retries["max_attempts"] == 3


def test_check_lambda_timeout_ignores_contexts_without_deadlines() -> None:
    check_lambda_timeout(None)
    check_lambda_timeout(object())
    check_lambda_timeout(_LambdaContext(None))


def test_check_lambda_timeout_respects_default_and_custom_buffers() -> None:
    assert LambdaTimeoutConfig().buffer_seconds == DEFAULT_LAMBDA_TIMEOUT_BUFFER_SECONDS

    check_lambda_timeout(_LambdaContext(1_001))
    check_lambda_timeout(_CamelLambdaContext(600), buffer_seconds=0.5)

    with pytest.raises(TimeoutError, match="lambda timeout imminent: only 1000ms remaining"):
        check_lambda_timeout(_LambdaContext(1_000))

    with pytest.raises(TimeoutError, match="lambda timeout imminent: only 500ms remaining"):
        check_lambda_timeout(_CamelLambdaContext(500), buffer_seconds=0.5)


def test_with_lambda_timeout_wraps_client_when_deadline_is_available() -> None:
    client = FakeDynamoDBClient()
    assert with_lambda_timeout(client, None) is client
    assert with_lambda_timeout(client, object()) is client

    context = _LambdaContext(750)
    wrapped = with_lambda_timeout(client, context, buffer_seconds=0.5)
    assert wrapped is not client

    client.expect("get_item", response={})
    wrapped.get_item(TableName="tbl", Key={})
    client.assert_no_pending()

    context.remaining_ms = 500
    with pytest.raises(TimeoutError, match="lambda timeout imminent"):
        wrapped.get_item(TableName="tbl", Key={})
    assert len(client.calls) == 1


def test_with_lambda_timeout_clamps_negative_buffer_seconds() -> None:
    client = FakeDynamoDBClient()
    wrapped = with_lambda_timeout(client, _LambdaContext(0), buffer_seconds=-5)

    with pytest.raises(TimeoutError, match="lambda timeout imminent: only 0ms remaining"):
        wrapped.get_item(TableName="tbl", Key={})
    assert client.calls == []


def test_instrument_boto3_client_records_calls() -> None:
    metrics: list[object] = []

    client = FakeDynamoDBClient()
    client.expect("put_item", response={})
    wrapped = instrument_boto3_client(client, service="dynamodb", on_call=metrics.append)
    wrapped.put_item(TableName="t", Item={})
    assert len(metrics) == 1

    client2 = FakeDynamoDBClient()
    client2.expect("get_item", error=RuntimeError("boom"), response=None)
    wrapped2 = instrument_boto3_client(client2, service="dynamodb", on_call=metrics.append)
    with pytest.raises(RuntimeError, match="boom"):
        wrapped2.get_item(TableName="t", Key={})
    assert len(metrics) == 2


def test_get_lambda_boto3_client_caches() -> None:
    _reset_lambda_clients_for_tests()

    class FakeSession:
        def __init__(self) -> None:
            self.calls = 0

        def client(self, service_name: str, **kwargs: object) -> object:
            _ = service_name, kwargs
            self.calls += 1
            return {"created_at": datetime.now(tz=UTC).isoformat()}

    sess = FakeSession()
    c1 = get_lambda_boto3_client("dynamodb", region="us-east-1", session=sess)
    c2 = get_lambda_boto3_client("dynamodb", region="us-east-1", session=sess)
    assert c1 is c2
    assert sess.calls == 1


def test_table_with_lambda_timeout_preserves_table_state_and_guards_calls() -> None:
    model = ModelDefinition.from_dataclass(RuntimeItem, table_name="runtime_items")
    client = FakeDynamoDBClient()
    table: Table[RuntimeItem] = Table(model, client=client)

    assert table.with_lambda_timeout(None) is table

    context = _LambdaContext(1_500)
    wrapped = table.with_lambda_timeout(context, buffer_seconds=0.5)
    assert wrapped is not table
    assert wrapped._model is table._model
    assert wrapped._table_name == table._table_name
    assert wrapped._kms_key_arn == table._kms_key_arn
    assert wrapped._kms_client is table._kms_client
    assert wrapped._rand_bytes is table._rand_bytes

    client.expect(
        "get_item",
        response={"Item": {"PK": {"S": "A"}, "SK": {"S": "1"}, "value": {"N": "7"}}},
    )
    assert wrapped.get("A", "1") == RuntimeItem(pk="A", sk="1", value=7)
    client.assert_no_pending()

    context.remaining_ms = 500
    with pytest.raises(TimeoutError, match="lambda timeout imminent"):
        wrapped.get("A", "1")
    assert len(client.calls) == 1


def _non_main_threads() -> list[str]:
    return [t.name for t in threading.enumerate() if t is not threading.main_thread()]


def test_runtime_helpers_leave_no_background_threads() -> None:
    # The Python runtime is synchronous. Lambda freezes the execution environment
    # when the handler returns, so a helper must not leave a thread running after
    # it returns.
    assert _non_main_threads() == []

    _reset_lambda_clients_for_tests()
    try:
        get_lambda_boto3_client("dynamodb", region="us-east-1", config=create_lambda_boto3_config())
        get_lambda_boto3_client("kms", region="us-east-1")
        assert _non_main_threads() == []
    finally:
        _reset_lambda_clients_for_tests()


def test_scan_all_segments_joins_its_worker_threads() -> None:
    model = ModelDefinition.from_dataclass(RuntimeItem, table_name="runtime_items")
    client = FakeDynamoDBClient()
    table: Table[RuntimeItem] = Table(model, client=client)

    empty_page = {"Items": [], "Count": 0, "ScannedCount": 0}
    for _ in range(2):
        client.expect("scan", response=empty_page)

    assert table.scan_all_segments(total_segments=2, max_workers=2) == []
    client.assert_no_pending()

    # The parallel segment workers are joined before the call returns, so no
    # thread outlives the invocation that started it.
    assert _non_main_threads() == []


def test_runtime_exposes_no_cold_start_prewarm() -> None:
    # TableTheory's Python Lambda surface deliberately has no pre-warm that runs
    # detached from the init that started it. A new one must be reviewed against
    # the same invariant as Go's removed pre-warm, so this test fails on purpose.
    pattern = re.compile(r"pre_?warm|warm_?up|cold_?start|optimi[sz]e", re.IGNORECASE)
    offenders = sorted(name for name in dir(runtime) if pattern.search(name))
    assert offenders == []
