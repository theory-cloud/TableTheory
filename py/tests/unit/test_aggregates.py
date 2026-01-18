from __future__ import annotations

from dataclasses import dataclass

import pytest

from theorydb_py.aggregates import (
    aggregate_field,
    average_field,
    count_distinct,
    group_by,
    max_field,
    min_field,
    sum_field,
)


def test_aggregate_helpers_on_mappings() -> None:
    items = [
        {"n": 1, "k": "a"},
        {"n": 2, "k": "a"},
        {"n": 0, "k": 0},
        {"k": "b"},
        {},
    ]

    assert sum_field(items, "n") == 3.0
    assert average_field(items, "n") == 1.0
    assert min_field(items, "n") == 1
    assert max_field(items, "n") == 2

    agg = aggregate_field(items, "n")
    assert agg.count == 5
    assert agg.sum == 3.0
    assert agg.average == 1.0
    assert agg.min == 1
    assert agg.max == 2

    assert count_distinct(items, "k") == 2


def test_group_by_query_aggregates_and_having() -> None:
    items = [
        {"g": "a", "n": 1},
        {"g": "a", "n": 2},
        {"g": "b", "n": 10},
        {"g": "", "n": 100},
        {"g": "a", "n": 0},
        {"g": "a", "n": None},
    ]

    results = (
        group_by(items, "g")
        .count("cnt")
        .sum("n", "sum")
        .avg("n", "avg")
        .min("n", "min")
        .max("n", "max")
        .having("COUNT(*)", ">", 1)
        .having("sum", "=", 3)
        .execute()
    )

    assert len(results) == 1
    group = results[0]
    assert group.key == "a"
    assert group.count == 4
    assert group.aggregates["sum"].sum == 3.0
    assert group.aggregates["avg"].average == 1.0
    assert group.aggregates["min"].min == 1
    assert group.aggregates["max"].max == 2


def test_aggregate_helpers_on_objects() -> None:
    @dataclass
    class Rec:
        g: str
        n: int | None = None

    items = [Rec(g="a", n=1), Rec(g="a", n=2), Rec(g="a", n=0)]
    assert sum_field(items, "n") == 3.0
    assert min_field(items, "n") == 1


def test_min_max_errors_match_go_behavior() -> None:
    with pytest.raises(ValueError, match="no items found"):
        min_field([], "n")

    with pytest.raises(ValueError, match="no valid values found for field n"):
        max_field([{"n": 0}, {"n": 0}], "n")
