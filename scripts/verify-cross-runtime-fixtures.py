#!/usr/bin/env python3
"""Pin the two shared, language-neutral cross-runtime contract fixtures.

The nil/empty serialization table and the transaction-cancellation fixture are
consumed by the Go, TypeScript and Python suites. Each runtime asserts its own
column, so a divergence in a runtime already fails that runtime's own test. What
nothing asserted until now is the fixture wiring itself: a fixture row or case
could be dropped, renamed, or a runtime could quietly stop consuming its
fixture, and no single gate would notice.

This validator is that gate. It fails when:

* a fixture file is missing or changes shape (declared runtimes/writer paths,
  representation vocabulary, per-row/per-case required keys);
* a row or case disappears or is renamed (both sets are pinned by name);
* a runtime's consumer file is deleted or stops referencing its fixture.

It intentionally does not re-run the runtime assertions: it pins the shared
surface they depend on. `scripts/verify-cross-runtime-contracts.sh` runs both.
"""

from __future__ import annotations

import sys
from pathlib import Path
from typing import Any

import yaml

REPO_ROOT = Path(__file__).resolve().parents[1]

TABLE_PATH = REPO_ROOT / "contract-tests" / "serialization" / "nil-empty-collections.yml"
CANCELLATION_PATH = (
    REPO_ROOT
    / "contract-tests"
    / "scenarios"
    / "cancellation"
    / "01-transaction-cancellation.yml"
)
LIFECYCLE_PATH = REPO_ROOT / "contract-tests" / "serialization" / "lifecycle-clocks.yml"

# The vocabulary and shape both fixtures must keep. These are the contract the
# three runtimes are written against; widening them is a deliberate change that
# must update every runtime and this pin together.
EXPECTED_RUNTIMES = ["go", "ts", "py"]
EXPECTED_WRITER_PATHS = ["put", "transact_write"]
EXPECTED_REPRESENTATIONS = {"NULL", "L", "M", "BOOL", "N", "S", "ABSENT"}
EXPECTED_ROW_FIELDS = [
    "nil_list",
    "empty_list",
    "nil_map",
    "empty_map",
    "nil_list_omit",
    "empty_list_omit",
    "nil_map_omit",
    "empty_map_omit",
    "false_bool",
    "zero_number",
    "empty_set",
    "nested_nil_list",
    "nested_empty_list",
    "nested_nil_map",
    "nested_empty_map",
]
EXPECTED_ROW_KEYS = {"field", "container", "omitempty", "expect"}
EXPECTED_CASE_NAMES = [
    "none_then_conditional_check_failed",
    "none_then_conflict_then_conditional",
    "all_none",
    "empty_reason_list",
    "throttling_error",
    "provisioned_throughput_exceeded",
    "unknown_code_is_generic",
    "missing_code_keeps_position",
    "transport_error_not_reclassified",
]
EXPECTED_CASE_KEYS = {"name", "kind", "expect"}
EXPECTED_CASE_KINDS = {"cancellation", "transport"}

# The lifecycle-clock table pins stamping per writer path. Widening any of these
# lists is a deliberate contract change that must update every runtime and this
# pin together.
EXPECTED_LIFECYCLE_NAME = "lifecycle-clocks"
EXPECTED_LIFECYCLE_MODES = ["fixed", "advancing", "missing"]
EXPECTED_LIFECYCLE_STAMPS = ["advancing_first", "advancing_second", "fixed"]
EXPECTED_LIFECYCLE_EXPECTATIONS = {
    "stamp_fixed",
    "stamp_first",
    "stamp_second",
    "wall_clock",
    "unstamped",
}
EXPECTED_LIFECYCLE_ROW_KEYS = {"mode", "expect", "exact_equality_supported"}
OPTIONAL_LIFECYCLE_ROW_KEYS = {"put_equals_transact_write"}

# Every runtime's consumer of each fixture, and the fixture name it must
# reference. A runtime that stops reading its fixture drops out of the contract
# silently; this pin makes that a hard failure.
CONSUMERS = [
    ("go", TABLE_PATH.name, "pkg/marshal/preflight_conformance_test.go"),
    ("ts", TABLE_PATH.name, "ts/test/unit/preflight-conformance.test.ts"),
    ("py", TABLE_PATH.name, "py/tests/unit/test_preflight_conformance.py"),
    (
        "go",
        CANCELLATION_PATH.name,
        "contract-tests/runners/go/transaction_cancellation_contract_test.go",
    ),
    (
        "ts",
        CANCELLATION_PATH.name,
        "contract-tests/runners/ts/test/contract.cancellation.test.ts",
    ),
    (
        "py",
        CANCELLATION_PATH.name,
        "contract-tests/runners/py/test_contract_cancellation.py",
    ),
    (
        "go",
        LIFECYCLE_PATH.name,
        "pkg/marshal/preflight_lifecycle_conformance_test.go",
    ),
    (
        "ts",
        LIFECYCLE_PATH.name,
        "ts/test/unit/lifecycle-clock-conformance.test.ts",
    ),
    (
        "py",
        LIFECYCLE_PATH.name,
        "py/tests/unit/test_lifecycle_clock_conformance.py",
    ),
]


def load_yaml(path: Path) -> Any:
    if not path.is_file():
        raise SystemExit(f"cross-runtime-fixtures: FAIL (missing fixture {path})")
    return yaml.safe_load(path.read_text(encoding="utf-8"))


def check_serialization_table(problems: list[str]) -> None:
    table = load_yaml(TABLE_PATH)
    if not isinstance(table, dict):
        problems.append(f"{TABLE_PATH.name}: not a mapping")
        return

    if table.get("version") != 1:
        problems.append(f"{TABLE_PATH.name}: version must be 1")
    if table.get("runtimes") != EXPECTED_RUNTIMES:
        problems.append(
            f"{TABLE_PATH.name}: runtimes must be {EXPECTED_RUNTIMES}, "
            f"got {table.get('runtimes')!r}"
        )
    if table.get("writer_paths") != EXPECTED_WRITER_PATHS:
        problems.append(
            f"{TABLE_PATH.name}: writer_paths must be {EXPECTED_WRITER_PATHS}, "
            f"got {table.get('writer_paths')!r}"
        )
    declared_reps = table.get("representations")
    if not isinstance(declared_reps, list) or set(declared_reps) != EXPECTED_REPRESENTATIONS:
        problems.append(
            f"{TABLE_PATH.name}: representations must be "
            f"{sorted(EXPECTED_REPRESENTATIONS)}, got {declared_reps!r}"
        )

    rows = table.get("rows")
    if not isinstance(rows, list):
        problems.append(f"{TABLE_PATH.name}: rows must be a list")
        return

    row_fields = [row.get("field") for row in rows if isinstance(row, dict)]
    if row_fields != EXPECTED_ROW_FIELDS:
        problems.append(
            f"{TABLE_PATH.name}: rows changed; expected {EXPECTED_ROW_FIELDS}, "
            f"got {row_fields}"
        )

    for row in rows:
        if not isinstance(row, dict):
            problems.append(f"{TABLE_PATH.name}: row is not a mapping")
            continue
        field = row.get("field", "<unnamed>")
        if set(row) != EXPECTED_ROW_KEYS:
            problems.append(f"{TABLE_PATH.name}/{field}: keys must be {sorted(EXPECTED_ROW_KEYS)}")
        expect = row.get("expect")
        if not isinstance(expect, dict):
            problems.append(f"{TABLE_PATH.name}/{field}: expect must be a mapping")
            continue
        if set(expect) != set(EXPECTED_WRITER_PATHS):
            problems.append(
                f"{TABLE_PATH.name}/{field}: expect must cover {EXPECTED_WRITER_PATHS}"
            )
            continue
        for writer_path in EXPECTED_WRITER_PATHS:
            column = expect[writer_path]
            if not isinstance(column, dict) or list(column) != EXPECTED_RUNTIMES:
                problems.append(
                    f"{TABLE_PATH.name}/{field}/{writer_path}: must have exactly "
                    f"{EXPECTED_RUNTIMES}"
                )
                continue
            for runtime in EXPECTED_RUNTIMES:
                value = column[runtime]
                if value not in EXPECTED_REPRESENTATIONS:
                    problems.append(
                        f"{TABLE_PATH.name}/{field}/{writer_path}/{runtime}: "
                        f"{value!r} is not a representation"
                    )

    cells_per_runtime = len(row_fields) * len(EXPECTED_WRITER_PATHS)
    print(
        f"cross-runtime-fixtures: table rows={len(row_fields)} "
        f"cells/runtime={cells_per_runtime} "
        f"({len(EXPECTED_WRITER_PATHS)} writer paths x {len(EXPECTED_RUNTIMES)} runtimes)"
    )


def check_cancellation_fixture(problems: list[str]) -> None:
    fixture = load_yaml(CANCELLATION_PATH)
    if not isinstance(fixture, dict):
        problems.append(f"{CANCELLATION_PATH.name}: not a mapping")
        return
    if not fixture.get("name"):
        problems.append(f"{CANCELLATION_PATH.name}: missing name")

    cases = fixture.get("cases")
    if not isinstance(cases, list) or not cases:
        problems.append(f"{CANCELLATION_PATH.name}: cases must be a non-empty list")
        return

    case_names = [case.get("name") for case in cases if isinstance(case, dict)]
    if case_names != EXPECTED_CASE_NAMES:
        problems.append(
            f"{CANCELLATION_PATH.name}: cases changed; expected {EXPECTED_CASE_NAMES}, "
            f"got {case_names}"
        )

    for case in cases:
        if not isinstance(case, dict):
            problems.append(f"{CANCELLATION_PATH.name}: case is not a mapping")
            continue
        name = case.get("name", "<unnamed>")
        if not EXPECTED_CASE_KEYS.issubset(case):
            problems.append(
                f"{CANCELLATION_PATH.name}/{name}: must declare {sorted(EXPECTED_CASE_KEYS)}"
            )
        if case.get("kind") not in EXPECTED_CASE_KINDS:
            problems.append(
                f"{CANCELLATION_PATH.name}/{name}: kind must be one of "
                f"{sorted(EXPECTED_CASE_KINDS)}"
            )
        expect = case.get("expect")
        if not isinstance(expect, dict) or "code" not in expect or "cause_recoverable" not in expect:
            problems.append(
                f"{CANCELLATION_PATH.name}/{name}: expect must declare code and cause_recoverable"
            )

    print(f"cross-runtime-fixtures: cancellation cases={len(case_names)}")


def check_lifecycle_table(problems: list[str]) -> None:
    fixture = load_yaml(LIFECYCLE_PATH)
    if not isinstance(fixture, dict):
        problems.append(f"{LIFECYCLE_PATH.name}: not a mapping")
        return

    if fixture.get("version") != 1:
        problems.append(f"{LIFECYCLE_PATH.name}: version must be 1")
    if fixture.get("name") != EXPECTED_LIFECYCLE_NAME:
        problems.append(f"{LIFECYCLE_PATH.name}: name must be {EXPECTED_LIFECYCLE_NAME}")
    if fixture.get("runtimes") != EXPECTED_RUNTIMES:
        problems.append(f"{LIFECYCLE_PATH.name}: runtimes must be {EXPECTED_RUNTIMES}")
    if fixture.get("writer_paths") != EXPECTED_WRITER_PATHS:
        problems.append(f"{LIFECYCLE_PATH.name}: writer_paths must be {EXPECTED_WRITER_PATHS}")
    if fixture.get("clock_modes") != EXPECTED_LIFECYCLE_MODES:
        problems.append(
            f"{LIFECYCLE_PATH.name}: clock_modes must be {EXPECTED_LIFECYCLE_MODES}, "
            f"got {fixture.get('clock_modes')!r}"
        )

    stamps = fixture.get("stamps")
    if not isinstance(stamps, dict) or sorted(stamps) != EXPECTED_LIFECYCLE_STAMPS:
        problems.append(
            f"{LIFECYCLE_PATH.name}: stamps must be {EXPECTED_LIFECYCLE_STAMPS}, got {stamps!r}"
        )

    expectations = fixture.get("expectations")
    if not isinstance(expectations, list) or set(expectations) != EXPECTED_LIFECYCLE_EXPECTATIONS:
        problems.append(
            f"{LIFECYCLE_PATH.name}: expectations must be "
            f"{sorted(EXPECTED_LIFECYCLE_EXPECTATIONS)}, got {expectations!r}"
        )

    rows = fixture.get("rows")
    if not isinstance(rows, list):
        problems.append(f"{LIFECYCLE_PATH.name}: rows must be a list")
        return
    modes = [row.get("mode") for row in rows if isinstance(row, dict)]
    if modes != EXPECTED_LIFECYCLE_MODES:
        problems.append(
            f"{LIFECYCLE_PATH.name}: rows changed; expected one per "
            f"{EXPECTED_LIFECYCLE_MODES}, got {modes}"
        )

    for row in rows:
        if not isinstance(row, dict):
            problems.append(f"{LIFECYCLE_PATH.name}: row is not a mapping")
            continue
        mode = row.get("mode", "<unnamed>")
        keys = set(row)
        if not EXPECTED_LIFECYCLE_ROW_KEYS.issubset(keys) or keys - (
            EXPECTED_LIFECYCLE_ROW_KEYS | OPTIONAL_LIFECYCLE_ROW_KEYS
        ):
            problems.append(
                f"{LIFECYCLE_PATH.name}/{mode}: keys must be "
                f"{sorted(EXPECTED_LIFECYCLE_ROW_KEYS)} plus optional "
                f"{sorted(OPTIONAL_LIFECYCLE_ROW_KEYS)}"
            )
        expect = row.get("expect")
        if not isinstance(expect, dict) or set(expect) != set(EXPECTED_WRITER_PATHS):
            problems.append(f"{LIFECYCLE_PATH.name}/{mode}: expect must cover {EXPECTED_WRITER_PATHS}")
            continue
        for writer_path in EXPECTED_WRITER_PATHS:
            column = expect[writer_path]
            if not isinstance(column, dict) or list(column) != EXPECTED_RUNTIMES:
                problems.append(
                    f"{LIFECYCLE_PATH.name}/{mode}/{writer_path}: must have exactly "
                    f"{EXPECTED_RUNTIMES}"
                )
                continue
            for runtime in EXPECTED_RUNTIMES:
                value = column[runtime]
                if value not in EXPECTED_LIFECYCLE_EXPECTATIONS:
                    problems.append(
                        f"{LIFECYCLE_PATH.name}/{mode}/{writer_path}/{runtime}: "
                        f"{value!r} is not an expectation"
                    )
        for key in EXPECTED_LIFECYCLE_ROW_KEYS | OPTIONAL_LIFECYCLE_ROW_KEYS:
            if key == "mode" or key == "expect" or key not in row:
                continue
            column = row[key]
            if not isinstance(column, dict) or sorted(column) != sorted(EXPECTED_RUNTIMES):
                problems.append(
                    f"{LIFECYCLE_PATH.name}/{mode}/{key}: must have exactly {EXPECTED_RUNTIMES}"
                )
                continue
            if not all(isinstance(column[runtime], bool) for runtime in EXPECTED_RUNTIMES):
                problems.append(f"{LIFECYCLE_PATH.name}/{mode}/{key}: values must be booleans")

    print(f"cross-runtime-fixtures: lifecycle modes={len(modes)}")


def check_consumers(problems: list[str]) -> None:
    for runtime, fixture_name, rel_path in CONSUMERS:
        path = REPO_ROOT / rel_path
        if not path.is_file():
            problems.append(f"{runtime} consumer missing: {rel_path}")
            continue
        if fixture_name not in path.read_text(encoding="utf-8"):
            problems.append(
                f"{runtime} consumer {rel_path} no longer references {fixture_name}"
            )
    print(f"cross-runtime-fixtures: runtime consumers pinned={len(CONSUMERS)}")


def main() -> int:
    problems: list[str] = []
    check_serialization_table(problems)
    check_cancellation_fixture(problems)
    check_lifecycle_table(problems)
    check_consumers(problems)

    if problems:
        print("cross-runtime-fixtures: FAIL", file=sys.stderr)
        for problem in problems:
            print(f"  - {problem}", file=sys.stderr)
        return 1

    print("cross-runtime-fixtures: PASS")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
