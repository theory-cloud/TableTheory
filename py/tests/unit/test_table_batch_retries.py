from __future__ import annotations

from dataclasses import dataclass

import pytest

from theorydb_py import BatchRetryExceededError, ModelDefinition, Table, theorydb_field


@dataclass(frozen=True)
class Note:
    pk: str = theorydb_field(roles=["pk"])
    sk: str = theorydb_field(roles=["sk"])
    value: int = theorydb_field()


class _StubBatchClient:
    def __init__(self, *, table_name: str, table: Table[Note], items: list[Note]) -> None:
        self._table_name = table_name
        self._table = table
        self._items = items
        self.batch_get_calls = 0
        self.batch_write_calls = 0

    def batch_get_item(self, *, RequestItems):  # noqa: N803
        self.batch_get_calls += 1
        if self.batch_get_calls == 1:
            return {"Responses": {self._table_name: []}, "UnprocessedKeys": RequestItems}

        return {
            "Responses": {self._table_name: [self._table._to_item(i) for i in self._items]},
            "UnprocessedKeys": {},
        }

    def batch_write_item(self, *, RequestItems):  # noqa: N803
        self.batch_write_calls += 1
        if self.batch_write_calls == 1:
            return {"UnprocessedItems": RequestItems}
        return {"UnprocessedItems": {}}


class _AlwaysUnprocessedClient:
    def __init__(self, *, table_name: str) -> None:
        self._table_name = table_name

    def batch_get_item(self, *, RequestItems):  # noqa: N803
        return {"Responses": {self._table_name: []}, "UnprocessedKeys": RequestItems}

    def batch_write_item(self, *, RequestItems):  # noqa: N803
        return {"UnprocessedItems": RequestItems}


def test_batch_get_retries_unprocessed_keys() -> None:
    model = ModelDefinition.from_dataclass(Note, table_name="tbl")
    table: Table[Note] = Table(model, client=object())

    expected = [Note(pk="A", sk="1", value=1), Note(pk="B", sk="2", value=2)]
    stub = _StubBatchClient(table_name="tbl", table=table, items=expected)
    table = Table(model, client=stub)

    got = table.batch_get([("A", "1"), ("B", "2")], sleep=lambda _: None)
    assert set(got) == set(expected)
    assert stub.batch_get_calls == 2


def test_batch_write_retries_unprocessed_items() -> None:
    model = ModelDefinition.from_dataclass(Note, table_name="tbl")
    stub = _StubBatchClient(table_name="tbl", table=Table(model, client=object()), items=[])
    table: Table[Note] = Table(model, client=stub)

    table.batch_write(puts=[Note(pk="A", sk="1", value=1)], sleep=lambda _: None)
    assert stub.batch_write_calls == 2


def test_batch_get_retry_limit_exceeded() -> None:
    model = ModelDefinition.from_dataclass(Note, table_name="tbl")
    table: Table[Note] = Table(model, client=_AlwaysUnprocessedClient(table_name="tbl"))

    with pytest.raises(BatchRetryExceededError):
        table.batch_get([("A", "1")], max_retries=0, sleep=lambda _: None)


def test_batch_write_retry_limit_exceeded() -> None:
    model = ModelDefinition.from_dataclass(Note, table_name="tbl")
    table: Table[Note] = Table(model, client=_AlwaysUnprocessedClient(table_name="tbl"))

    with pytest.raises(BatchRetryExceededError):
        table.batch_write(puts=[Note(pk="A", sk="1", value=1)], max_retries=0, sleep=lambda _: None)
