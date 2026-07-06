from __future__ import annotations

from collections.abc import Iterator
from typing import TYPE_CHECKING, Any

from .query import FilterExpression, Page, SortKeyCondition

if TYPE_CHECKING:
    from .table import Table


def query_iter(
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
) -> Iterator[Page[Any]]:
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
        yield page
        if page.next_cursor is None:
            break
        next_cursor = page.next_cursor


def scan_iter(
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
) -> Iterator[Page[Any]]:
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
        yield page
        if page.next_cursor is None:
            break
        next_cursor = page.next_cursor
