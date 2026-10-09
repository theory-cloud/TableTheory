from __future__ import annotations

from typing import Any

from botocore.exceptions import ClientError

from .aws_errors import map_client_error as _map_client_error
from .errors import ValidationError
from .query import FilterExpression, SortKeyCondition, decode_cursor


def query_count(
    table: Any,
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
) -> int:
    """Count matching items with an optional evaluated-item budget.

    ``limit`` bounds the total number of items evaluated across every page, not
    the number of matched items. A ``limit`` therefore never triggers a full
    remaining-partition scan; the returned count covers only the evaluated
    budget. Pass ``limit=None`` to page to exhaustion.
    """
    del projection
    if limit is not None and limit <= 0:
        raise ValidationError("limit must be > 0")
    partition_attr, sort_attr, index_type = table._resolve_index(index_name)
    if index_type == "GSI" and consistent_read:
        raise ValidationError("consistent_read is not supported for GSIs")
    if partition is None:
        raise ValidationError("partition is required")

    names: dict[str, str] = {"#pk": partition_attr}
    values: dict[str, Any] = {":pk": table._serializer.serialize(partition)}
    key_expr = "#pk = :pk"
    if sort is not None:
        if sort_attr is None:
            raise ValidationError("model/index does not define a sort key")
        names["#sk"] = sort_attr
        key_expr = table._apply_sort_condition(key_expr, sort, values)

    req: dict[str, Any] = {
        "TableName": table._table_name,
        "KeyConditionExpression": key_expr,
        "ExpressionAttributeNames": names,
        "ExpressionAttributeValues": values,
        "ScanIndexForward": scan_forward,
        "ConsistentRead": consistent_read,
        "Select": "COUNT",
    }
    if index_name is not None:
        req["IndexName"] = index_name
    if filter is not None:
        req["FilterExpression"] = table._filter_expression(filter, names, values)

    start_key = _query_count_start_key(cursor, index_name, scan_forward)
    return _count_pages(table._client.query, req, start_key, limit)


def scan_count(
    table: Any,
    *,
    index_name: str | None = None,
    limit: int | None = None,
    cursor: str | None = None,
    consistent_read: bool = False,
    projection: list[str] | None = None,
    filter: FilterExpression | None = None,
    segment: int | None = None,
    total_segments: int | None = None,
) -> int:
    """Count scan matches with an optional evaluated-item budget.

    ``limit`` bounds the total number of items evaluated across every page. A
    ``limit`` therefore never triggers a full remaining-table scan; the returned
    count covers only the evaluated budget. Pass ``limit=None`` to page to
    exhaustion.
    """
    del projection
    if limit is not None and limit <= 0:
        raise ValidationError("limit must be > 0")
    _, _, index_type = table._resolve_index(index_name)
    if index_type == "GSI" and consistent_read:
        raise ValidationError("consistent_read is not supported for GSIs")

    req: dict[str, Any] = {
        "TableName": table._table_name,
        "ConsistentRead": consistent_read,
        "Select": "COUNT",
    }
    names: dict[str, str] = {}
    values: dict[str, Any] = {}
    if index_name is not None:
        req["IndexName"] = index_name
    if filter is not None:
        req["FilterExpression"] = table._filter_expression(filter, names, values)
    _apply_scan_segments(req, segment, total_segments)
    if names:
        req["ExpressionAttributeNames"] = names
    if values:
        req["ExpressionAttributeValues"] = values

    start_key = _scan_count_start_key(cursor, index_name)
    return _count_pages(table._client.scan, req, start_key, limit)


def _query_count_start_key(cursor: str | None, index_name: str | None, scan_forward: bool) -> Any | None:
    if cursor is None:
        return None
    try:
        decoded = decode_cursor(cursor)
    except Exception as err:
        raise ValidationError("invalid cursor") from err
    if decoded.index is not None and decoded.index != index_name:
        raise ValidationError("cursor index does not match query")
    expected_sort = "ASC" if scan_forward else "DESC"
    if decoded.sort is not None and decoded.sort != expected_sort:
        raise ValidationError("cursor sort does not match query")
    return decoded.last_key


def _scan_count_start_key(cursor: str | None, index_name: str | None) -> Any | None:
    if cursor is None:
        return None
    try:
        decoded = decode_cursor(cursor)
    except Exception as err:
        raise ValidationError("invalid cursor") from err
    if decoded.index is not None and decoded.index != index_name:
        raise ValidationError("cursor index does not match scan")
    return decoded.last_key


def _apply_scan_segments(req: dict[str, Any], segment: int | None, total_segments: int | None) -> None:
    if (segment is None) != (total_segments is None):
        raise ValidationError("segment and total_segments must be provided together")
    if segment is None or total_segments is None:
        return
    if segment < 0 or total_segments <= 0 or segment >= total_segments:
        raise ValidationError("invalid segment/total_segments")
    req["Segment"] = segment
    req["TotalSegments"] = total_segments


def _count_pages(call: Any, req: dict[str, Any], start_key: Any | None, limit: int | None = None) -> int:
    """Sum matched counts while bounding the total number of evaluated items.

    ``limit`` is a budget over evaluated items (DynamoDB ``ScannedCount``), not
    matched items: each page requests at most the remaining budget as its own
    ``Limit`` and the loop stops once the budget is spent, so a small ``limit``
    can never page the whole remaining partition or table.
    """
    total = 0
    evaluated = 0
    while True:
        if start_key is not None:
            req["ExclusiveStartKey"] = start_key
        else:
            req.pop("ExclusiveStartKey", None)
        if limit is not None:
            remaining = limit - evaluated
            if remaining <= 0:
                break
            req["Limit"] = remaining
        try:
            resp = call(**req)
        except ClientError as err:  # pragma: no cover
            raise _map_client_error(err) from err
        page_count = int(resp.get("Count", 0))
        total += page_count
        evaluated += int(resp.get("ScannedCount", page_count))
        start_key = resp.get("LastEvaluatedKey")
        if not start_key:
            return total
        if limit is not None and evaluated >= limit:
            return total
