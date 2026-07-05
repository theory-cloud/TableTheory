from __future__ import annotations

from typing import TYPE_CHECKING, Any

from .query import FilterExpression, SortKeyCondition

if TYPE_CHECKING:
    from .table import Table


def query_all(
    table: Table[Any],
    partition: Any,
    *,
    sort: SortKeyCondition | None = None,
    index_name: str | None = None,
    limit: int | None = None,
    cursor: str | None = None,
    scan_forward: bool = True,
    consistent_read: bool = False,
    projection: list[str] | None = None,
    filter: FilterExpression | None = None,
) -> list[Any]:
    out: list[Any] = []
    next_cursor: str | None = cursor
    while True:
        page = table.query(
            partition,
            sort=sort,
            index_name=index_name,
            limit=limit,
            cursor=next_cursor,
            scan_forward=scan_forward,
            consistent_read=consistent_read,
            projection=projection,
            filter=filter,
        )
        out.extend(page.items)
        if page.next_cursor is None:
            break
        next_cursor = page.next_cursor
    return out


def scan_all(
    table: Table[Any],
    *,
    index_name: str | None = None,
    limit: int | None = None,
    cursor: str | None = None,
    consistent_read: bool = False,
    projection: list[str] | None = None,
    filter: FilterExpression | None = None,
    segment: int | None = None,
    total_segments: int | None = None,
) -> list[Any]:
    out: list[Any] = []
    next_cursor: str | None = cursor
    while True:
        page = table.scan(
            index_name=index_name,
            limit=limit,
            cursor=next_cursor,
            consistent_read=consistent_read,
            projection=projection,
            filter=filter,
            segment=segment,
            total_segments=total_segments,
        )
        out.extend(page.items)
        if page.next_cursor is None:
            break
        next_cursor = page.next_cursor
    return out
