from __future__ import annotations

import re
import threading
import time
from dataclasses import dataclass
from datetime import UTC, datetime
from typing import Any

import pytest

from tabletheory_py import runtime
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


class _SegmentErrorClient(FakeDynamoDBClient):
    """Fails segment 0 and holds every other segment open for a moment.

    That window is exactly where an unjoined parallel scan would return while
    its remaining segment workers were still running.
    """

    def __init__(self) -> None:
        super().__init__()
        self.segment_zero_failed = threading.Event()
        self.finished: list[int | None] = []

    def scan(self, **kwargs: Any) -> dict[str, Any]:
        segment = kwargs.get("Segment")
        if segment == 0:
            self.segment_zero_failed.set()
            raise RuntimeError("segment scan failed")
        assert self.segment_zero_failed.wait(timeout=5)
        time.sleep(0.05)
        self.finished.append(segment)
        return {"Items": [], "Count": 0, "ScannedCount": 0}


def test_scan_all_segments_joins_its_worker_threads_on_segment_error() -> None:
    model = ModelDefinition.from_dataclass(RuntimeItem, table_name="runtime_items")
    client = _SegmentErrorClient()
    table: Table[RuntimeItem] = Table(model, client=client)

    with pytest.raises(RuntimeError, match="segment scan failed"):
        table.scan_all_segments(total_segments=3, max_workers=3)

    # The first segment error must not let the call return while the other
    # segment workers are still running, and the exception the caller sees must
    # be the original segment failure rather than a secondary error.
    assert sorted(client.finished) == [1, 2]
    assert _non_main_threads() == []


class _CutoffContext:
    """A thread-safe Lambda context whose remaining time can reach the buffer."""

    def __init__(self, remaining_ms: int = 60_000, cutoff_ms: int = 100) -> None:
        self._lock = threading.Lock()
        self._remaining_ms = remaining_ms
        self._cutoff_ms = cutoff_ms

    def get_remaining_time_in_millis(self) -> int:
        with self._lock:
            return self._remaining_ms

    def reach_cutoff(self) -> None:
        with self._lock:
            self._remaining_ms = self._cutoff_ms


class _SegmentedScanClient:
    """Deterministic, thread-safe scan double for parallel segment scans.

    Pages are scripted per segment and claimed atomically, so a worker always
    receives the next page for its own segment whatever the thread interleaving
    is. Nothing here sleeps or depends on call ordering across segments.
    """

    def __init__(
        self,
        pages: dict[int, list[dict[str, Any]]],
        *,
        cutoff_context: _CutoffContext | None = None,
        cutoff_after_segments: int = 0,
        first_page_barrier: threading.Barrier | None = None,
    ) -> None:
        self._pages = pages
        self._cutoff_context = cutoff_context
        self._cutoff_after_segments = cutoff_after_segments
        self._first_page_barrier = first_page_barrier
        self._lock = threading.Lock()
        self._served: dict[int, int] = {}
        self.calls: list[dict[str, Any]] = []

    def scan(self, **req: Any) -> dict[str, Any]:
        segment = int(req["Segment"])
        with self._lock:
            self.calls.append(dict(req))
            index = self._served.get(segment, 0)
            self._served[segment] = index + 1
            started_segments = len(self._served)
            if (
                index == 0
                and self._cutoff_context is not None
                and started_segments >= self._cutoff_after_segments
            ):
                self._cutoff_context.reach_cutoff()
        if index == 0 and self._first_page_barrier is not None:
            # Both segments must finish their first page before either one is
            # allowed to ask for its next page, so the cutoff is observed by
            # both workers rather than by whichever one loses a race.
            self._first_page_barrier.wait(timeout=5)
        pages = self._pages[segment]
        return dict(pages[index] if index < len(pages) else {"Items": []})


def _segment_pages(segments: int) -> dict[int, list[dict[str, Any]]]:
    scripted: dict[int, list[dict[str, Any]]] = {}
    for segment in range(segments):
        pages: list[dict[str, Any]] = []
        for page in range(2):
            item = {
                "PK": {"S": f"seg{segment}"},
                "SK": {"S": f"s{page}"},
                "value": {"N": str(segment * 10 + page)},
            }
            body: dict[str, Any] = {"Items": [item], "Count": 1, "ScannedCount": 1}
            if page == 0:
                body["LastEvaluatedKey"] = {
                    "PK": {"S": f"seg{segment}"},
                    "SK": {"S": f"s{page}"},
                }
            pages.append(body)
        scripted[segment] = pages
    return scripted


def test_scan_all_segments_stops_before_raw_calls_when_already_expired() -> None:
    model = ModelDefinition.from_dataclass(RuntimeItem, table_name="runtime_items")
    context = _CutoffContext()
    context.reach_cutoff()
    client = _SegmentedScanClient(_segment_pages(2))
    table = Table(model, client=client).with_lambda_timeout(context, buffer_seconds=1.0)

    with pytest.raises(TimeoutError) as info:
        table.scan_all_segments(total_segments=2, max_workers=2)

    assert getattr(info.value, "_theorydb_lambda_timeout_imminent", False) is True
    # The worker clone must submit nothing to the raw client, because the
    # deadline guard has to run before every SDK operation.
    assert client.calls == []
    assert _non_main_threads() == []


def test_scan_all_segments_blocks_continuation_after_cutoff() -> None:
    model = ModelDefinition.from_dataclass(RuntimeItem, table_name="runtime_items")
    context = _CutoffContext()
    client = _SegmentedScanClient(
        _segment_pages(2),
        cutoff_context=context,
        cutoff_after_segments=2,
        first_page_barrier=threading.Barrier(2),
    )
    table = Table(model, client=client).with_lambda_timeout(context, buffer_seconds=1.0)

    with pytest.raises(TimeoutError) as info:
        table.scan_all_segments(total_segments=2, max_workers=2)

    assert getattr(info.value, "_theorydb_lambda_timeout_imminent", False) is True
    # Exactly the two first pages, and no continuation carrying a cursor.
    assert len(client.calls) == 2
    assert [call["Segment"] for call in client.calls].count(0) == 1
    assert [call["Segment"] for call in client.calls].count(1) == 1
    assert all("ExclusiveStartKey" not in call for call in client.calls)
    assert _non_main_threads() == []


def test_scan_all_segments_returns_every_page_in_segment_order() -> None:
    model = ModelDefinition.from_dataclass(RuntimeItem, table_name="runtime_items")
    client = _SegmentedScanClient(_segment_pages(2))
    table: Table[RuntimeItem] = Table(model, client=client)

    items = table.scan_all_segments(total_segments=2, max_workers=2)

    assert [(item.pk, item.sk, item.value) for item in items] == [
        ("seg0", "s0", 0),
        ("seg0", "s1", 1),
        ("seg1", "s0", 10),
        ("seg1", "s1", 11),
    ]
    assert len(client.calls) == 4
    assert {call["TotalSegments"] for call in client.calls} == {2}
    for segment in (0, 1):
        segment_calls = [call for call in client.calls if call["Segment"] == segment]
        assert len(segment_calls) == 2
        assert "ExclusiveStartKey" not in segment_calls[0]
        assert segment_calls[1]["ExclusiveStartKey"] == {
            "PK": {"S": f"seg{segment}"},
            "SK": {"S": "s0"},
        }
    assert _non_main_threads() == []


def test_segment_worker_inherits_operational_chain_and_clock() -> None:
    model = ModelDefinition.from_dataclass(RuntimeItem, table_name="runtime_items")

    def fixed_clock() -> str:
        return "2026-01-02T03:04:05.000000000Z"

    client = FakeDynamoDBClient()
    table: Table[RuntimeItem] = Table(model, client=client, now=fixed_clock)
    context = _LambdaContext(60_000)
    derived = table.with_lambda_timeout(context, buffer_seconds=1.0)

    worker = derived._segment_worker()

    assert worker is not derived
    # Canonical identity stays the caller's raw client.
    assert worker._raw_client is client
    # The complete operational chain is reused as-is: no raw fallback and no
    # second timeout/redaction wrap.
    assert worker._client is derived._client
    assert worker._client is not client
    # Every other piece of state the writer and readers depend on is inherited.
    assert worker._model is derived._model
    assert worker._table_name == derived._table_name
    assert worker._kms_key_arn == derived._kms_key_arn
    assert worker._kms_client is derived._kms_client
    assert worker._rand_bytes is derived._rand_bytes
    assert worker._now is fixed_clock
    assert worker._redact_item_images == derived._redact_item_images


def test_runtime_exposes_no_cold_start_prewarm() -> None:
    # TableTheory's Python Lambda surface deliberately has no pre-warm that runs
    # detached from the init that started it. A new one must be reviewed against
    # the same invariant as Go's removed pre-warm, so this test fails on purpose.
    pattern = re.compile(r"pre_?warm|warm_?up|cold_?start|optimi[sz]e", re.IGNORECASE)
    offenders = sorted(name for name in dir(runtime) if pattern.search(name))
    assert offenders == []
