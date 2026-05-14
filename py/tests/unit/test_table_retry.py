from __future__ import annotations

from dataclasses import dataclass

import pytest

from theorydb_py import ModelDefinition, Table, theorydb_field
from theorydb_py.mocks import FakeDynamoDBClient


@dataclass(frozen=True)
class PKOnly:
    pk: str = theorydb_field(roles=["pk"])


class _LambdaContext:
    def __init__(self, remaining_ms: int) -> None:
        self.remaining_ms = remaining_ms

    def get_remaining_time_in_millis(self) -> int:
        return self.remaining_ms


def test_query_with_retry_retries_on_empty_page() -> None:
    client = FakeDynamoDBClient()
    client.expect("query", response={"Items": [], "LastEvaluatedKey": None})
    client.expect("query", response={"Items": [{"pk": {"S": "P1"}}], "LastEvaluatedKey": None})

    model = ModelDefinition.from_dataclass(PKOnly, table_name="tbl")
    table: Table[PKOnly] = Table(model, client=client)

    page = table.query_with_retry("P1", max_retries=1, initial_delay_seconds=0, sleep=lambda _: None)
    assert [r.pk for r in page.items] == ["P1"]
    client.assert_no_pending()


def test_query_with_retry_does_not_retry_lambda_timeout_guard() -> None:
    client = FakeDynamoDBClient()

    model = ModelDefinition.from_dataclass(PKOnly, table_name="tbl")
    table: Table[PKOnly] = Table(model, client=client).with_lambda_timeout(
        _LambdaContext(500), buffer_seconds=0.5
    )
    sleeps: list[float] = []

    with pytest.raises(TimeoutError, match="lambda timeout imminent"):
        table.query_with_retry(
            "P1",
            max_retries=3,
            initial_delay_seconds=0.1,
            sleep=sleeps.append,
        )

    assert sleeps == []
    assert client.calls == []


def test_scan_with_retry_retries_on_empty_page() -> None:
    client = FakeDynamoDBClient()
    client.expect("scan", response={"Items": [], "LastEvaluatedKey": None})
    client.expect("scan", response={"Items": [{"pk": {"S": "P1"}}], "LastEvaluatedKey": None})

    model = ModelDefinition.from_dataclass(PKOnly, table_name="tbl")
    table: Table[PKOnly] = Table(model, client=client)

    page = table.scan_with_retry(max_retries=1, initial_delay_seconds=0, sleep=lambda _: None)
    assert [r.pk for r in page.items] == ["P1"]
    client.assert_no_pending()


def test_scan_with_retry_does_not_retry_lambda_timeout_guard() -> None:
    client = FakeDynamoDBClient()

    model = ModelDefinition.from_dataclass(PKOnly, table_name="tbl")
    table: Table[PKOnly] = Table(model, client=client).with_lambda_timeout(
        _LambdaContext(500), buffer_seconds=0.5
    )
    sleeps: list[float] = []

    with pytest.raises(TimeoutError, match="lambda timeout imminent"):
        table.scan_with_retry(max_retries=3, initial_delay_seconds=0.1, sleep=sleeps.append)

    assert sleeps == []
    assert client.calls == []
