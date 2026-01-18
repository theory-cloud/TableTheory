from __future__ import annotations

from dataclasses import dataclass

from theorydb_py import ModelDefinition, Table, theorydb_field
from theorydb_py.mocks import FakeDynamoDBClient


@dataclass(frozen=True)
class PKOnly:
    pk: str = theorydb_field(roles=["pk"])


def test_query_with_retry_retries_on_empty_page() -> None:
    client = FakeDynamoDBClient()
    client.expect("query", response={"Items": [], "LastEvaluatedKey": None})
    client.expect("query", response={"Items": [{"pk": {"S": "P1"}}], "LastEvaluatedKey": None})

    model = ModelDefinition.from_dataclass(PKOnly, table_name="tbl")
    table: Table[PKOnly] = Table(model, client=client)

    page = table.query_with_retry("P1", max_retries=1, initial_delay_seconds=0, sleep=lambda _: None)
    assert [r.pk for r in page.items] == ["P1"]
    client.assert_no_pending()


def test_scan_with_retry_retries_on_empty_page() -> None:
    client = FakeDynamoDBClient()
    client.expect("scan", response={"Items": [], "LastEvaluatedKey": None})
    client.expect("scan", response={"Items": [{"pk": {"S": "P1"}}], "LastEvaluatedKey": None})

    model = ModelDefinition.from_dataclass(PKOnly, table_name="tbl")
    table: Table[PKOnly] = Table(model, client=client)

    page = table.scan_with_retry(max_retries=1, initial_delay_seconds=0, sleep=lambda _: None)
    assert [r.pk for r in page.items] == ["P1"]
    client.assert_no_pending()
