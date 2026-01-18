from __future__ import annotations

from theorydb_py import SortKeyCondition


def test_sort_key_condition_constructors() -> None:
    assert SortKeyCondition.eq("a") == SortKeyCondition(op="=", values=("a",))
    assert SortKeyCondition.lt("a") == SortKeyCondition(op="<", values=("a",))
    assert SortKeyCondition.lte("a") == SortKeyCondition(op="<=", values=("a",))
    assert SortKeyCondition.gt("a") == SortKeyCondition(op=">", values=("a",))
    assert SortKeyCondition.gte("a") == SortKeyCondition(op=">=", values=("a",))
    assert SortKeyCondition.between("a", "b") == SortKeyCondition(op="between", values=("a", "b"))
    assert SortKeyCondition.begins_with("a") == SortKeyCondition(op="begins_with", values=("a",))
