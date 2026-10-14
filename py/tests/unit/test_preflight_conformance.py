from __future__ import annotations

from collections.abc import Callable
from dataclasses import dataclass
from pathlib import Path
from typing import Any

import yaml

from tabletheory_py import (
    ItemWriterPath,
    ModelDefinition,
    StatefulDynamoDBClient,
    Table,
    TransactPut,
    theorydb_field,
)

_TABLE_NAME = "preflight_conformance"

# The one language-neutral table the Go, TypeScript and Python suites all
# consume. It lives beside the contract test fixtures so a single file states
# these semantics for every runtime. Repo root is three levels up from
# py/tests/unit.
_CONFORMANCE_TABLE_PATH = (
    Path(__file__).resolve().parents[3] / "contract-tests" / "serialization" / "nil-empty-collections.yml"
)


@dataclass(frozen=True)
class ConformanceModel:
    pk: str = theorydb_field(name="PK", roles=["pk"])
    sk: str = theorydb_field(name="SK", roles=["sk"])
    nil_list: list[str] | None = theorydb_field(name="nilList", default=None)
    empty_list: list[str] = theorydb_field(name="emptyList", default_factory=list)
    nil_map: dict[str, str] | None = theorydb_field(name="nilMap", default=None)
    empty_map: dict[str, str] = theorydb_field(name="emptyMap", default_factory=dict)
    nil_list_omit: list[str] | None = theorydb_field(name="nilListOmit", omitempty=True, default=None)
    empty_list_omit: list[str] = theorydb_field(name="emptyListOmit", omitempty=True, default_factory=list)
    nil_map_omit: dict[str, str] | None = theorydb_field(name="nilMapOmit", omitempty=True, default=None)
    empty_map_omit: dict[str, str] = theorydb_field(name="emptyMapOmit", omitempty=True, default_factory=dict)
    false_bool: bool = theorydb_field(name="falseBool", default=False)
    zero_int: int = theorydb_field(name="zeroInt", default=0)
    empty_set: set[str] = theorydb_field(name="emptySet", set_=True, default_factory=set)
    nested: dict[str, Any] = theorydb_field(name="nested", default_factory=dict)


def _conformance_item() -> ConformanceModel:
    return ConformanceModel(
        pk="p1",
        sk="s1",
        empty_list=[],
        empty_map={},
        empty_list_omit=[],
        empty_map_omit={},
        nested={
            "nestedNilList": None,
            "nestedEmptyList": [],
            "nestedNilMap": None,
            "nestedEmptyMap": {},
        },
    )


def _table(client: StatefulDynamoDBClient) -> Table[ConformanceModel]:
    model = ModelDefinition.from_dataclass(ConformanceModel, table_name=_TABLE_NAME)
    return Table(model, client=client)


@dataclass(frozen=True)
class _Row:
    field: str
    container: str
    omitempty: bool
    expect: dict[str, dict[str, str]]


@dataclass(frozen=True)
class _ConformanceTable:
    version: int
    writer_paths: list[str]
    runtimes: list[str]
    representations: list[str]
    rows: list[_Row]


def _load_conformance_table() -> _ConformanceTable:
    assert _CONFORMANCE_TABLE_PATH.is_file(), (
        f"the shared conformance table must exist at {_CONFORMANCE_TABLE_PATH}"
    )

    raw: Any = yaml.safe_load(_CONFORMANCE_TABLE_PATH.read_text(encoding="utf-8"))
    assert isinstance(raw, dict), "the shared conformance table must be a mapping"

    version = raw["version"]
    assert version == 1, f"unsupported conformance table version: {version!r}"

    writer_paths = [str(path) for path in raw["writer_paths"]]
    runtimes = [str(runtime) for runtime in raw["runtimes"]]
    representations = [str(rep) for rep in raw["representations"]]

    rows: list[_Row] = []
    for entry in raw["rows"]:
        expect: dict[str, dict[str, str]] = {
            str(path): {str(runtime): str(rep) for runtime, rep in cells.items()}
            for path, cells in entry["expect"].items()
        }
        rows.append(
            _Row(
                field=str(entry["field"]),
                container=str(entry["container"]),
                omitempty=bool(entry["omitempty"]),
                expect=expect,
            )
        )
    assert rows, "the shared conformance table must declare rows"
    return _ConformanceTable(
        version=version,
        writer_paths=writer_paths,
        runtimes=runtimes,
        representations=representations,
        rows=rows,
    )


def _root(name: str) -> Callable[[dict[str, Any]], str]:
    def read(item: dict[str, Any]) -> str:
        if name not in item:
            return "ABSENT"
        return next(iter(item[name]))

    return read


def _nested(name: str) -> Callable[[dict[str, Any]], str]:
    def read(item: dict[str, Any]) -> str:
        container = item.get("nested")
        if not isinstance(container, dict):
            return "ABSENT"
        values = container.get("M")
        if not isinstance(values, dict) or name not in values:
            return "ABSENT"
        return next(iter(values[name]))

    return read


# conformance_field_readers maps each logical field name in the shared table to
# the Python attribute it is stored as, and to a reader over a stored item.
def _conformance_field_readers() -> dict[str, Callable[[dict[str, Any]], str]]:
    return {
        "nil_list": _root("nilList"),
        "empty_list": _root("emptyList"),
        "nil_map": _root("nilMap"),
        "empty_map": _root("emptyMap"),
        "nil_list_omit": _root("nilListOmit"),
        "empty_list_omit": _root("emptyListOmit"),
        "nil_map_omit": _root("nilMapOmit"),
        "empty_map_omit": _root("emptyMapOmit"),
        "false_bool": _root("falseBool"),
        "zero_number": _root("zeroInt"),
        "empty_set": _root("emptySet"),
        "nested_nil_list": _nested("nestedNilList"),
        "nested_empty_list": _nested("nestedEmptyList"),
        "nested_nil_map": _nested("nestedNilMap"),
        "nested_empty_map": _nested("nestedEmptyMap"),
    }


def _capture(path: str) -> dict[str, Any]:
    client = StatefulDynamoDBClient()
    table = _table(client)
    item = _conformance_item()

    if path == "put":
        table.put(item)
    else:
        table.transact_write([TransactPut(item)])

    items = client.items(_TABLE_NAME)
    assert len(items) == 1, f"the fake must have stored exactly one {path} item"
    return items[0]


# TestSharedConformanceTablePythonColumn asserts the Python column of the shared
# table against both the captured writer payloads and the preflight answers.
# Every row must be mapped and every cell counted, so a dropped or renamed row
# fails the build instead of silently shrinking the pin.
def test_shared_conformance_table_python_column() -> None:
    table = _load_conformance_table()
    readers = _conformance_field_readers()

    assert len(table.rows) == len(readers), (
        "the shared table must declare exactly the fields the Python runtime maps"
    )
    assert "py" in table.runtimes
    assert table.writer_paths

    captured = {path: _capture(path) for path in table.writer_paths}
    preflight = {
        path: _table(StatefulDynamoDBClient()).marshal_item_for_writer_path(
            _conformance_item(), ItemWriterPath(path)
        )
        for path in table.writer_paths
    }

    seen: set[str] = set()
    captured_cells = 0
    preflight_cells = 0

    for row in table.rows:
        assert row.field in readers, (
            f"the shared table names a field the Python runtime does not map: {row.field}"
        )
        assert row.field not in seen, f"the shared table repeats the field {row.field}"
        seen.add(row.field)

        for path in table.writer_paths:
            expectations = row.expect[path]
            want = expectations["py"]
            assert want in table.representations, f"{want} is not a declared representation"

            got_captured = readers[row.field](captured[path])
            assert got_captured == want, (
                f"captured {path} payload for {row.field}: got {got_captured}, want {want}"
            )
            got_preflight = readers[row.field](preflight[path])
            assert got_preflight == want, (
                f"preflight {path} answer for {row.field}: got {got_preflight}, want {want}"
            )

            captured_cells += 1
            preflight_cells += 1

    derived_cells = len(table.rows) * len(table.writer_paths)
    assert captured_cells == derived_cells, "every row/writer-path cell must be checked against a capture"
    assert preflight_cells == derived_cells, (
        "every row/writer-path cell must be checked against the preflight"
    )
