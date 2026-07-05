from __future__ import annotations

from ._compat import reexport as _reexport

__all__ = _reexport(
    "tabletheory_py.aggregates",
    globals(),
    (
        "AggregateFunction",
        "AggregateResult",
        "GroupByQuery",
        "GroupedResult",
        "aggregate_field",
        "average_field",
        "count_distinct",
        "group_by",
        "max_field",
        "min_field",
        "sum_field",
    ),
)
