from __future__ import annotations

from collections.abc import Callable
from dataclasses import dataclass
from pathlib import Path

import pytest
import yaml

from tabletheory_py import (
    ItemWriterPath,
    ModelDefinition,
    StatefulDynamoDBClient,
    Table,
    TransactPut,
    ValidationError,
    marshal_item_for_writer_path,
    theorydb_field,
)

_TABLE_NAME = "lifecycle_clocks"

# The shared lifecycle-clock table the Go, TypeScript and Python suites all
# consume. Repo root is three levels up from py/tests/unit.
_LIFECYCLE_TABLE_PATH = (
    Path(__file__).resolve().parents[3] / "contract-tests" / "serialization" / "lifecycle-clocks.yml"
)


@dataclass(frozen=True)
class LifecycleClocksItem:
    pk: str = theorydb_field(name="PK", roles=["pk"])
    sk: str = theorydb_field(name="SK", roles=["sk"])
    created_at: str = theorydb_field(name="createdAt", roles=["created_at"], default="")
    updated_at: str = theorydb_field(name="updatedAt", roles=["updated_at"], default="")


def _load_fixture() -> dict[str, object]:
    assert _LIFECYCLE_TABLE_PATH.is_file(), (
        f"the shared lifecycle table must exist at {_LIFECYCLE_TABLE_PATH}"
    )
    fixture = yaml.safe_load(_LIFECYCLE_TABLE_PATH.read_text(encoding="utf-8"))
    assert isinstance(fixture, dict)
    assert fixture.get("version") == 1
    rows = fixture.get("rows")
    assert isinstance(rows, list) and rows
    return fixture


def _scenario_clock(mode: str, stamps: dict[str, str]) -> Callable[[], str] | None:
    if mode == "fixed":
        return lambda: stamps["fixed"]
    if mode == "missing":
        return None
    assert mode == "advancing", f"unknown clock mode {mode}"
    calls = 0

    def advancing() -> str:
        nonlocal calls
        value = stamps["advancing_first"] if calls == 0 else stamps["advancing_second"]
        calls += 1
        return value

    return advancing


def _table(client: StatefulDynamoDBClient, clock: Callable[[], str] | None) -> Table[LifecycleClocksItem]:
    model = ModelDefinition.from_dataclass(LifecycleClocksItem, table_name=_TABLE_NAME)
    return Table(model, client=client, now=clock)


def _capture_put(clock: Callable[[], str] | None) -> dict[str, object]:
    client = StatefulDynamoDBClient()
    _table(client, clock).put(LifecycleClocksItem(pk="p1", sk="s1"))
    items = client.items(_TABLE_NAME)
    assert len(items) == 1, "the fake must have stored exactly one put item"
    return items[0]


def _capture_transact(clock: Callable[[], str] | None) -> dict[str, object]:
    client = StatefulDynamoDBClient()
    _table(client, clock).transact_write([TransactPut(LifecycleClocksItem(pk="p1", sk="s1"))])
    items = client.items(_TABLE_NAME)
    assert len(items) == 1, "the fake must have stored exactly one transacted item"
    return items[0]


def _stamp_token(item: dict[str, object], stamps: dict[str, str]) -> str:
    value = item.get("createdAt")
    if not isinstance(value, dict):
        return "unstamped"
    text = value.get("S")
    if not isinstance(text, str):
        return "unstamped"
    if text == stamps["fixed"]:
        return "stamp_fixed"
    if text == stamps["advancing_first"]:
        return "stamp_first"
    if text == stamps["advancing_second"]:
        return "stamp_second"
    return "wall_clock"


def test_shared_lifecycle_table_pins_the_py_column_on_both_writer_paths() -> None:
    fixture = _load_fixture()
    stamps = fixture["stamps"]
    assert isinstance(stamps, dict)
    assert "py" in fixture["runtimes"]
    assert fixture["writer_paths"] == ["put", "transact_write"]
    assert fixture["clock_modes"] == ["fixed", "advancing", "missing"]

    rows = fixture["rows"]
    assert isinstance(rows, list)
    assert len(rows) == len(fixture["clock_modes"])

    for row in rows:
        assert isinstance(row, dict)
        mode = row["mode"]
        clock = _scenario_clock(mode, stamps)

        put_item = _capture_put(clock)
        transact_item = _capture_transact(clock)

        assert _stamp_token(put_item, stamps) == row["expect"]["put"]["py"], f"put createdAt for {mode}"
        assert _stamp_token(transact_item, stamps) == row["expect"]["transact_write"]["py"], (
            f"transact_write createdAt for {mode}"
        )
        assert put_item["createdAt"] == put_item["updatedAt"], (
            "both lifecycle fields of one put must come from the same clock read"
        )

        declared = row.get("put_equals_transact_write")
        if isinstance(declared, dict):
            assert declared["py"] == (put_item == transact_item), (
                f"the py writer paths disagree about equality under the {mode} clock"
            )

        # A preflight can only reproduce a stamped item when it reads the same
        # clock as the writer, which the shared table only allows for the fixed
        # row on a runtime whose two paths both stamp.
        table = _table(StatefulDynamoDBClient(), clock)
        item = LifecycleClocksItem(pk="p1", sk="s1")
        if row["exact_equality_supported"]["py"]:
            preflight = marshal_item_for_writer_path(table, item, ItemWriterPath.PUT, now=stamps["fixed"])
            assert preflight["createdAt"] == put_item["createdAt"]
            assert preflight["updatedAt"] == put_item["updatedAt"]
        else:
            # Unsupported: the preflight must refuse rather than invent a stamp.
            for path in (ItemWriterPath.PUT, ItemWriterPath.TRANSACT_WRITE):
                with pytest.raises(ValidationError, match="lifecycle clock"):
                    marshal_item_for_writer_path(table, item, path)
