from __future__ import annotations

import math
from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from decimal import Decimal
from typing import Any, Literal


@dataclass(frozen=True)
class AggregateResult:
    min: Any | None = None
    max: Any | None = None
    count: int = 0
    sum: float = 0.0
    average: float = 0.0


@dataclass
class GroupedResult[T]:
    key: Any
    aggregates: dict[str, AggregateResult]
    items: list[T]
    count: int


type AggregateFunction = Literal["COUNT", "SUM", "AVG", "MIN", "MAX"]


@dataclass(frozen=True)
class _AggregateOp:
    function: AggregateFunction
    field: str
    alias: str


@dataclass(frozen=True)
class _HavingClause:
    aggregate: str
    operator: str
    value: Any


class GroupByQuery[T]:
    def __init__(self, items: Sequence[T], group_by_field: str) -> None:
        self._items = list(items)
        self._group_by = group_by_field
        self._aggregates: list[_AggregateOp] = []
        self._having: list[_HavingClause] = []

    def count(self, alias: str) -> GroupByQuery[T]:
        self._aggregates.append(_AggregateOp(function="COUNT", field="*", alias=alias))
        return self

    def sum(self, field: str, alias: str) -> GroupByQuery[T]:
        self._aggregates.append(_AggregateOp(function="SUM", field=field, alias=alias))
        return self

    def avg(self, field: str, alias: str) -> GroupByQuery[T]:
        self._aggregates.append(_AggregateOp(function="AVG", field=field, alias=alias))
        return self

    def min(self, field: str, alias: str) -> GroupByQuery[T]:
        self._aggregates.append(_AggregateOp(function="MIN", field=field, alias=alias))
        return self

    def max(self, field: str, alias: str) -> GroupByQuery[T]:
        self._aggregates.append(_AggregateOp(function="MAX", field=field, alias=alias))
        return self

    def having(self, aggregate: str, operator: str, value: Any) -> GroupByQuery[T]:
        self._having.append(_HavingClause(aggregate=aggregate, operator=operator, value=value))
        return self

    def execute(self) -> list[GroupedResult[T]]:
        groups: dict[str, GroupedResult[T]] = {}

        for item in self._items:
            key = _extract_field_value(item, self._group_by)
            if key is None:
                continue

            key_str = str(key)
            if key_str in groups:
                group = groups[key_str]
                group.items.append(item)
                group.count += 1
                continue

            groups[key_str] = GroupedResult(
                key=key,
                count=1,
                items=[item],
                aggregates={},
            )

        for group in groups.values():
            for op in self._aggregates:
                group.aggregates[op.alias] = _calculate_aggregate(group.items, op)

        out: list[GroupedResult[T]] = []
        for group in groups.values():
            if _evaluate_having(group, self._having):
                out.append(group)

        return out


def group_by[T](items: Sequence[T], field: str) -> GroupByQuery[T]:
    return GroupByQuery(items, field)


def sum_field[T](items: Sequence[T], field: str) -> float:
    total = 0.0
    for item in items:
        value = _extract_numeric_value(item, field)
        if value is None:
            continue
        total += value
    return total


def average_field[T](items: Sequence[T], field: str) -> float:
    if not items:
        return 0.0

    total = 0.0
    count = 0
    for item in items:
        value = _extract_numeric_value(item, field)
        if value is None:
            continue
        total += value
        count += 1

    if count == 0:
        return 0.0
    return total / count


def min_field[T](items: Sequence[T], field: str) -> Any:
    return _extreme_value(items, field, direction=-1)


def max_field[T](items: Sequence[T], field: str) -> Any:
    return _extreme_value(items, field, direction=1)


def aggregate_field[T](items: Sequence[T], field: str | None = None) -> AggregateResult:
    result = AggregateResult(count=len(items))
    if not field:
        return result

    total = 0.0
    numeric_count = 0
    min_value: Any | None = None
    max_value: Any | None = None

    for item in items:
        num = _extract_numeric_value(item, field)
        if num is not None:
            total += num
            numeric_count += 1

        value = _extract_field_value(item, field)
        if value is None:
            continue

        if min_value is None or _compare_values(value, min_value) < 0:
            min_value = value
        if max_value is None or _compare_values(value, max_value) > 0:
            max_value = value

    avg = total / numeric_count if numeric_count > 0 else 0.0
    return AggregateResult(
        count=len(items),
        sum=total,
        average=avg,
        min=min_value,
        max=max_value,
    )


def count_distinct[T](items: Sequence[T], field: str) -> int:
    unique: set[str] = set()
    for item in items:
        value = _extract_field_value(item, field)
        if value is None:
            continue
        unique.add(str(value))
    return len(unique)


def _calculate_aggregate[T](items: Sequence[T], op: _AggregateOp) -> AggregateResult:
    if op.function == "COUNT":
        return AggregateResult(count=len(items))
    if op.function == "SUM":
        return AggregateResult(sum=sum_field(items, op.field))
    if op.function == "AVG":
        return AggregateResult(average=average_field(items, op.field))
    if op.function == "MIN":
        return AggregateResult(min=_extreme_field_value(items, op.field, pick_max=False))
    if op.function == "MAX":
        return AggregateResult(max=_extreme_field_value(items, op.field, pick_max=True))
    raise ValueError(f"unsupported aggregate function: {op.function}")


def _extreme_value[T](items: Sequence[T], field: str, *, direction: Literal[-1, 1]) -> Any:
    if not items:
        raise ValueError("no items found")

    extreme: Any | None = None
    for item in items:
        value = _extract_field_value(item, field)
        if value is None:
            continue

        if extreme is None:
            extreme = value
            continue

        cmp = _compare_values(value, extreme)
        if (direction < 0 and cmp < 0) or (direction > 0 and cmp > 0):
            extreme = value

    if extreme is None:
        raise ValueError(f"no valid values found for field {field}")
    return extreme


def _extreme_field_value[T](items: Sequence[T], field: str, *, pick_max: bool) -> Any | None:
    selected: Any | None = None
    for item in items:
        value = _extract_field_value(item, field)
        if value is None:
            continue

        if selected is None:
            selected = value
            continue

        cmp = _compare_values(value, selected)
        if (pick_max and cmp > 0) or (not pick_max and cmp < 0):
            selected = value
    return selected


def _evaluate_having[T](group: GroupedResult[T], clauses: Sequence[_HavingClause]) -> bool:
    for clause in clauses:
        agg_value = _aggregate_value(group, clause.aggregate)
        if agg_value is None:
            return False

        compare_value = _to_float(clause.value)
        if compare_value is None:
            return False

        if not _compare_having(agg_value, clause.operator, compare_value):
            return False
    return True


def _aggregate_value[T](group: GroupedResult[T], aggregate: str) -> float | None:
    if aggregate == "COUNT(*)":
        return float(group.count)

    result = group.aggregates.get(aggregate)
    if result is None:
        return None

    value = _aggregate_result_value(result)
    if value is None:
        return None

    return value


def _aggregate_result_value(result: AggregateResult) -> float | None:
    if result.min is not None:
        return _to_float(result.min)
    if result.max is not None:
        return _to_float(result.max)
    if result.count != 0:
        return float(result.count)
    if result.sum != 0:
        return result.sum
    if result.average != 0:
        return result.average
    return 0.0


def _compare_having(agg_value: float, operator: str, compare_value: float) -> bool:
    match operator:
        case "=":
            return agg_value == compare_value
        case ">":
            return agg_value > compare_value
        case ">=":
            return agg_value >= compare_value
        case "<":
            return agg_value < compare_value
        case "<=":
            return agg_value <= compare_value
        case "!=":
            return agg_value != compare_value
        case _:
            raise ValueError(f"unsupported HAVING operator: {operator!r}")


def _extract_numeric_value(item: Any, field: str) -> float | None:
    value = _extract_raw_value(item, field)
    return _to_float(value)


def _extract_field_value(item: Any, field: str) -> Any | None:
    value = _extract_raw_value(item, field)
    if _is_zero_value(value):
        return None
    return value


def _extract_raw_value(item: Any, field: str) -> Any:
    if isinstance(item, Mapping):
        return item.get(field)
    return getattr(item, field, None)


def _compare_values(a: Any, b: Any) -> int:
    a_float = _to_float(a)
    b_float = _to_float(b)
    if a_float is not None and b_float is not None:
        if a_float < b_float:
            return -1
        if a_float > b_float:
            return 1
        return 0

    if isinstance(a, str) and isinstance(b, str):
        if a < b:
            return -1
        if a > b:
            return 1
        return 0

    a_str = str(a)
    b_str = str(b)
    if a_str < b_str:
        return -1
    if a_str > b_str:
        return 1
    return 0


def _to_float(value: Any) -> float | None:
    if isinstance(value, bool):
        return None
    if isinstance(value, (int, float, Decimal)):
        try:
            converted = float(value)
        except (OverflowError, TypeError, ValueError):
            return None
        if not math.isfinite(converted):
            return None
        return converted
    return None


def _is_zero_value(value: Any) -> bool:
    if value is None:
        return True
    if value is False:
        return True

    if isinstance(value, bool):
        return False

    if isinstance(value, (int, float, Decimal)) and value == 0:
        return True
    if isinstance(value, str) and value == "":
        return True
    if isinstance(value, (bytes, bytearray)) and len(value) == 0:
        return True
    if isinstance(value, (list, dict, set, tuple)) and len(value) == 0:
        return True

    return False
