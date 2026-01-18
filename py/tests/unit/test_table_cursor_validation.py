from __future__ import annotations

from dataclasses import dataclass

import pytest

from theorydb_py import ModelDefinition, Table, ValidationError, theorydb_field


@dataclass(frozen=True)
class Thing:
    pk: str = theorydb_field(roles=["pk"])
    sk: str = theorydb_field(roles=["sk"])
    value: int = theorydb_field(default=0)


def test_query_invalid_cursor_raises_validation_error() -> None:
    model = ModelDefinition.from_dataclass(Thing, table_name="tbl")
    table: Table[Thing] = Table(model, client=object())

    with pytest.raises(ValidationError):
        table.query("A", cursor="not-a-valid-cursor")


def test_scan_invalid_cursor_raises_validation_error() -> None:
    model = ModelDefinition.from_dataclass(Thing, table_name="tbl")
    table: Table[Thing] = Table(model, client=object())

    with pytest.raises(ValidationError):
        table.scan(cursor="not-a-valid-cursor")
