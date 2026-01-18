from __future__ import annotations

import json
import os
import time
from collections.abc import Callable, Mapping, Sequence
from dataclasses import MISSING, fields, is_dataclass
from decimal import Decimal
from typing import TYPE_CHECKING, Any, Literal, cast, get_args, get_origin

import boto3
from boto3.dynamodb.types import TypeDeserializer, TypeSerializer
from botocore.exceptions import ClientError

from .aws_errors import map_client_error as _map_client_error
from .aws_errors import map_transaction_error as _map_transaction_error
from .errors import (
    BatchRetryExceededError,
    EncryptionNotConfiguredError,
    NotFoundError,
    ValidationError,
)
from .model import AttributeDefinition, ModelDefinition
from .query import (
    FilterCondition,
    FilterExpression,
    FilterGroup,
    Page,
    SortKeyCondition,
    decode_cursor,
    encode_cursor,
)
from .transaction import (
    TransactConditionCheck,
    TransactDelete,
    TransactPut,
    TransactUpdate,
    TransactWriteAction,
)

if TYPE_CHECKING:
    from .optimizer import QueryShape, ScanShape
    from .update_builder import UpdateBuilder


def _is_empty(value: Any) -> bool:
    if value is None:
        return True
    if value is False:
        return True
    if value == 0:
        return True
    if isinstance(value, (str, bytes, bytearray)) and len(value) == 0:
        return True
    if isinstance(value, (list, dict, set, tuple)) and len(value) == 0:
        return True
    return False


def _coerce_value(value: Any, annotation: Any) -> Any:
    if value is None:
        return None

    if annotation is int and isinstance(value, Decimal):
        return int(value)
    if annotation is float and isinstance(value, Decimal):
        return float(value)

    origin = get_origin(annotation)
    if origin is set and isinstance(value, set):
        (elem_type,) = get_args(annotation) or (Any,)
        return {_coerce_value(v, elem_type) for v in value}

    return value


def _backoff_seconds(attempt: int) -> float:
    seconds = 0.05 * (2.0 ** (attempt - 1))
    if seconds > 1.0:
        return 1.0
    return seconds


def _chunked[T](items: Sequence[T], size: int) -> Sequence[Sequence[T]]:
    if size <= 0:
        raise ValueError("size must be > 0")
    return [items[i : i + size] for i in range(0, len(items), size)]


class Table[T]:
    def __init__(
        self,
        model: ModelDefinition[T],
        *,
        client: Any | None = None,
        table_name: str | None = None,
        kms_key_arn: str | None = None,
        kms_client: Any | None = None,
        rand_bytes: Callable[[int], bytes] | None = None,
    ) -> None:
        if table_name is None:
            table_name = model.table_name
        if not table_name:
            raise ValueError("table_name is required (or set ModelDefinition.table_name)")

        self._model = model
        self._table_name = table_name
        self._client: Any = client or boto3.client("dynamodb")
        self._kms_key_arn = (kms_key_arn or "").strip() or None
        self._kms_client: Any | None = kms_client
        self._rand_bytes = rand_bytes or os.urandom
        self._serializer = TypeSerializer()
        self._deserializer = TypeDeserializer()

        if any(attr.encrypted for attr in self._model.attributes.values()):
            if not self._kms_key_arn:
                raise EncryptionNotConfiguredError(
                    "model has encrypted fields but kms_key_arn is not configured"
                )
            self._kms_client = self._kms_client or boto3.client("kms")

    def query(
        self,
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
    ) -> Page[T]:
        partition_attr, sort_attr, index_type = self._resolve_index(index_name)
        if index_type == "GSI" and consistent_read:
            raise ValidationError("consistent_read is not supported for GSIs")

        if partition is None:
            raise ValidationError("partition is required")

        if limit is not None and limit <= 0:
            raise ValidationError("limit must be > 0")

        names: dict[str, str] = {"#pk": partition_attr}
        values: dict[str, Any] = {":pk": self._serializer.serialize(partition)}

        key_expr = "#pk = :pk"
        if sort is not None:
            if sort_attr is None:
                raise ValidationError("model/index does not define a sort key")
            names["#sk"] = sort_attr
            key_expr = self._apply_sort_condition(key_expr, sort, values)

        req: dict[str, Any] = {
            "TableName": self._table_name,
            "KeyConditionExpression": key_expr,
            "ExpressionAttributeNames": names,
            "ExpressionAttributeValues": values,
            "ScanIndexForward": scan_forward,
            "ConsistentRead": consistent_read,
        }
        if index_name is not None:
            req["IndexName"] = index_name
        if limit is not None:
            req["Limit"] = limit
        if cursor is not None:
            try:
                decoded = decode_cursor(cursor)
            except Exception as err:
                raise ValidationError("invalid cursor") from err
            if decoded.index is not None and decoded.index != index_name:
                raise ValidationError("cursor index does not match query")
            expected_sort = "ASC" if scan_forward else "DESC"
            if decoded.sort is not None and decoded.sort != expected_sort:
                raise ValidationError("cursor sort does not match query")
            req["ExclusiveStartKey"] = decoded.last_key
        if projection is not None:
            req["ProjectionExpression"] = self._projection_expression(
                projection, req["ExpressionAttributeNames"]
            )
        if filter is not None:
            req["FilterExpression"] = self._filter_expression(filter, names, values)

        try:
            resp = self._client.query(**req)
        except ClientError as err:  # pragma: no cover
            raise _map_client_error(err) from err

        items = [self._from_item(item) for item in resp.get("Items", [])]
        last = resp.get("LastEvaluatedKey")
        return Page(
            items=items,
            next_cursor=(
                encode_cursor(last, index=index_name, sort="ASC" if scan_forward else "DESC")
                if last
                else None
            ),
        )

    def query_with_retry(
        self,
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
        max_retries: int = 5,
        initial_delay_seconds: float = 0.1,
        max_delay_seconds: float = 5.0,
        backoff_factor: float = 2.0,
        retry_on_empty: bool = True,
        retry_on_error: bool = True,
        verify: Callable[[Page[T]], bool] | None = None,
        sleep: Callable[[float], None] = time.sleep,
    ) -> Page[T]:
        if max_retries < 0:
            raise ValidationError("max_retries must be >= 0")

        delay = initial_delay_seconds
        last_page: Page[T] | None = None

        for attempt in range(max_retries + 1):
            try:
                page = self.query(
                    partition,
                    sort=sort,
                    index_name=index_name,
                    limit=limit,
                    cursor=cursor,
                    scan_forward=scan_forward,
                    consistent_read=consistent_read,
                    projection=projection,
                    filter=filter,
                )
                last_page = page

                if verify is not None:
                    if verify(page):
                        return page
                elif not retry_on_empty or page.items:
                    return page
            except Exception:
                if not retry_on_error or attempt == max_retries:
                    raise

            if attempt < max_retries:
                if delay > 0:
                    sleep(delay)
                delay = min(max_delay_seconds, delay * backoff_factor)

        if last_page is None:
            raise ValidationError("retry exhausted without results")
        return last_page

    def query_all(
        self,
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
    ) -> list[T]:
        out: list[T] = []
        next_cursor: str | None = cursor

        while True:
            page = self.query(
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

    def describe_query(
        self,
        partition: Any | None = None,
        *,
        sort: SortKeyCondition | None = None,
        index_name: str | None = None,
        scan_forward: bool = True,
        consistent_read: bool = False,
        projection: list[str] | None = None,
        filter: FilterExpression | None = None,
    ) -> QueryShape:
        from .optimizer import QueryShape

        _, sort_attr, index_type = self._resolve_index(index_name)
        return QueryShape(
            kind="query",
            model_name=self._model.model_type.__name__,
            table_name=self._table_name,
            index_name=index_name,
            index_type=index_type,
            has_partition_key=partition is not None,
            has_sort_key=sort_attr is not None,
            has_sort_condition=sort is not None,
            has_filter=filter is not None,
            projections=tuple(projection or ()),
            consistent_read=consistent_read,
            scan_forward=scan_forward,
        )

    def scan(
        self,
        *,
        index_name: str | None = None,
        limit: int | None = None,
        cursor: str | None = None,
        consistent_read: bool = False,
        projection: list[str] | None = None,
        filter: FilterExpression | None = None,
        segment: int | None = None,
        total_segments: int | None = None,
    ) -> Page[T]:
        _, _, index_type = self._resolve_index(index_name)
        if index_type == "GSI" and consistent_read:
            raise ValidationError("consistent_read is not supported for GSIs")

        if limit is not None and limit <= 0:
            raise ValidationError("limit must be > 0")

        req: dict[str, Any] = {"TableName": self._table_name, "ConsistentRead": consistent_read}
        names: dict[str, str] = {}
        values: dict[str, Any] = {}
        if index_name is not None:
            req["IndexName"] = index_name
        if limit is not None:
            req["Limit"] = limit
        if cursor is not None:
            try:
                decoded = decode_cursor(cursor)
            except Exception as err:
                raise ValidationError("invalid cursor") from err
            if decoded.index is not None and decoded.index != index_name:
                raise ValidationError("cursor index does not match scan")
            req["ExclusiveStartKey"] = decoded.last_key
        if projection is not None:
            req["ProjectionExpression"] = self._projection_expression(projection, names)
        if filter is not None:
            req["FilterExpression"] = self._filter_expression(filter, names, values)

        if (segment is None) != (total_segments is None):
            raise ValidationError("segment and total_segments must be provided together")
        if segment is not None and total_segments is not None:
            if segment < 0 or total_segments <= 0 or segment >= total_segments:
                raise ValidationError("invalid segment/total_segments")
            req["Segment"] = segment
            req["TotalSegments"] = total_segments

        if names:
            req["ExpressionAttributeNames"] = names
        if values:
            req["ExpressionAttributeValues"] = values

        try:
            resp = self._client.scan(**req)
        except ClientError as err:  # pragma: no cover
            raise _map_client_error(err) from err

        items = [self._from_item(item) for item in resp.get("Items", [])]
        last = resp.get("LastEvaluatedKey")
        return Page(items=items, next_cursor=encode_cursor(last, index=index_name) if last else None)

    def scan_all(
        self,
        *,
        index_name: str | None = None,
        limit: int | None = None,
        cursor: str | None = None,
        consistent_read: bool = False,
        projection: list[str] | None = None,
        filter: FilterExpression | None = None,
        segment: int | None = None,
        total_segments: int | None = None,
    ) -> list[T]:
        out: list[T] = []
        next_cursor: str | None = cursor

        while True:
            page = self.scan(
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

    def describe_scan(
        self,
        *,
        index_name: str | None = None,
        consistent_read: bool = False,
        projection: list[str] | None = None,
        filter: FilterExpression | None = None,
        segment: int | None = None,
        total_segments: int | None = None,
    ) -> ScanShape:
        from .optimizer import ScanShape

        _, _, index_type = self._resolve_index(index_name)
        parallel_scan_configured = (segment is not None) or (total_segments is not None)
        return ScanShape(
            kind="scan",
            model_name=self._model.model_type.__name__,
            table_name=self._table_name,
            index_name=index_name,
            index_type=index_type,
            has_filter=filter is not None,
            projections=tuple(projection or ()),
            consistent_read=consistent_read,
            parallel_scan_configured=parallel_scan_configured,
            total_segments=total_segments,
        )

    def scan_with_retry(
        self,
        *,
        index_name: str | None = None,
        limit: int | None = None,
        cursor: str | None = None,
        consistent_read: bool = False,
        projection: list[str] | None = None,
        filter: FilterExpression | None = None,
        max_retries: int = 5,
        initial_delay_seconds: float = 0.1,
        max_delay_seconds: float = 5.0,
        backoff_factor: float = 2.0,
        retry_on_empty: bool = True,
        retry_on_error: bool = True,
        verify: Callable[[Page[T]], bool] | None = None,
        sleep: Callable[[float], None] = time.sleep,
    ) -> Page[T]:
        if max_retries < 0:
            raise ValidationError("max_retries must be >= 0")

        delay = initial_delay_seconds
        last_page: Page[T] | None = None

        for attempt in range(max_retries + 1):
            try:
                page = self.scan(
                    index_name=index_name,
                    limit=limit,
                    cursor=cursor,
                    consistent_read=consistent_read,
                    projection=projection,
                    filter=filter,
                )
                last_page = page

                if verify is not None:
                    if verify(page):
                        return page
                elif not retry_on_empty or page.items:
                    return page
            except Exception:
                if not retry_on_error or attempt == max_retries:
                    raise

            if attempt < max_retries:
                if delay > 0:
                    sleep(delay)
                delay = min(max_delay_seconds, delay * backoff_factor)

        if last_page is None:
            raise ValidationError("retry exhausted without results")
        return last_page

    def scan_all_segments(
        self,
        *,
        total_segments: int,
        index_name: str | None = None,
        limit: int | None = None,
        consistent_read: bool = False,
        projection: list[str] | None = None,
        filter: FilterExpression | None = None,
        max_workers: int | None = None,
    ) -> list[T]:
        if total_segments <= 0:
            raise ValidationError("total_segments must be > 0")

        if max_workers is None:
            max_workers = total_segments
        if max_workers <= 0:
            raise ValidationError("max_workers must be > 0")

        from concurrent.futures import ThreadPoolExecutor

        def scan_segment(segment: int) -> list[T]:
            table: Table[T] = Table(
                self._model,
                client=self._client,
                table_name=self._table_name,
                kms_key_arn=self._kms_key_arn,
                kms_client=self._kms_client,
                rand_bytes=self._rand_bytes,
            )
            cursor: str | None = None
            out: list[T] = []
            while True:
                page = table.scan(
                    index_name=index_name,
                    limit=limit,
                    cursor=cursor,
                    consistent_read=consistent_read,
                    projection=projection,
                    filter=filter,
                    segment=segment,
                    total_segments=total_segments,
                )
                out.extend(page.items)
                cursor = page.next_cursor
                if cursor is None:
                    break
            return out

        results: list[list[T]] = [[] for _ in range(total_segments)]
        with ThreadPoolExecutor(max_workers=max_workers) as ex:
            futures = {ex.submit(scan_segment, seg): seg for seg in range(total_segments)}
            for fut, seg in futures.items():
                results[seg] = fut.result()

        out: list[T] = []
        for seg_items in results:
            out.extend(seg_items)
        return out

    def batch_get(
        self,
        keys: Sequence[Any],
        *,
        consistent_read: bool = False,
        projection: list[str] | None = None,
        max_retries: int = 5,
        sleep: Callable[[float], None] | None = time.sleep,
    ) -> list[T]:
        if max_retries < 0:
            raise ValidationError("max_retries must be >= 0")

        if not keys:
            return []

        normalized: list[tuple[Any, Any | None]] = []
        for key in keys:
            if self._model.sk is None:
                if isinstance(key, tuple):
                    if len(key) != 2:
                        raise ValidationError("expected key tuple (pk, None) for pk-only models")
                    pk, sk = key
                    if sk is not None:
                        raise ValidationError("sk must be None for pk-only models")
                    normalized.append((pk, None))
                else:
                    normalized.append((key, None))
                continue

            if not isinstance(key, tuple) or len(key) != 2:
                raise ValidationError("expected key tuple (pk, sk)")
            pk, sk = key
            normalized.append((pk, sk))

        out: list[T] = []
        base_req: dict[str, Any] = {"ConsistentRead": consistent_read}
        if projection is not None:
            names: dict[str, str] = {}
            base_req["ExpressionAttributeNames"] = names
            base_req["ProjectionExpression"] = self._projection_expression(projection, names)

        for chunk in _chunked(normalized, 100):
            pending_keys = [self._to_key(pk, sk) for pk, sk in chunk]
            attempts = 0

            while pending_keys:
                req = {self._table_name: dict(base_req, Keys=pending_keys)}
                try:
                    resp = self._client.batch_get_item(RequestItems=req)
                except ClientError as err:  # pragma: no cover
                    raise _map_client_error(err) from err

                for item in resp.get("Responses", {}).get(self._table_name, []):
                    out.append(self._from_item(item))

                pending_keys = resp.get("UnprocessedKeys", {}).get(self._table_name, {}).get("Keys") or []
                if pending_keys:
                    if attempts >= max_retries:
                        raise BatchRetryExceededError(
                            operation="batch_get", unprocessed_count=len(pending_keys)
                        )
                    attempts += 1
                    if sleep is not None:
                        sleep(_backoff_seconds(attempts))

        return out

    def batch_write(
        self,
        *,
        puts: Sequence[T] = (),
        deletes: Sequence[Any] = (),
        max_retries: int = 5,
        sleep: Callable[[float], None] | None = time.sleep,
    ) -> None:
        if max_retries < 0:
            raise ValidationError("max_retries must be >= 0")

        requests: list[dict[str, Any]] = []
        for item in puts:
            requests.append({"PutRequest": {"Item": self._to_item(item)}})

        for key in deletes:
            if self._model.sk is None:
                if isinstance(key, tuple):
                    if len(key) != 2:
                        raise ValidationError("expected key tuple (pk, None) for pk-only models")
                    pk, sk = key
                    if sk is not None:
                        raise ValidationError("sk must be None for pk-only models")
                    requests.append({"DeleteRequest": {"Key": self._to_key(pk, None)}})
                else:
                    requests.append({"DeleteRequest": {"Key": self._to_key(key, None)}})
                continue

            if not isinstance(key, tuple) or len(key) != 2:
                raise ValidationError("expected key tuple (pk, sk)")
            pk, sk = key
            requests.append({"DeleteRequest": {"Key": self._to_key(pk, sk)}})

        for chunk in _chunked(requests, 25):
            pending = list(chunk)
            attempts = 0

            while pending:
                try:
                    resp = self._client.batch_write_item(RequestItems={self._table_name: pending})
                except ClientError as err:  # pragma: no cover
                    raise _map_client_error(err) from err

                pending = resp.get("UnprocessedItems", {}).get(self._table_name, []) or []
                if pending:
                    if attempts >= max_retries:
                        raise BatchRetryExceededError(operation="batch_write", unprocessed_count=len(pending))
                    attempts += 1
                    if sleep is not None:
                        sleep(_backoff_seconds(attempts))

    def transact_write(self, actions: Sequence[TransactWriteAction[T]]) -> None:
        if not actions:
            raise ValidationError("actions is required")
        if len(actions) > 100:
            raise ValidationError("a transaction supports at most 100 actions")

        transact_items: list[dict[str, Any]] = []
        for action in actions:
            if isinstance(action, TransactPut):
                req: dict[str, Any] = {"TableName": self._table_name, "Item": self._to_item(action.item)}
                if action.condition_expression:
                    req["ConditionExpression"] = action.condition_expression
                if action.expression_attribute_names:
                    req["ExpressionAttributeNames"] = dict(action.expression_attribute_names)
                if action.expression_attribute_values:
                    req["ExpressionAttributeValues"] = self._serialize_values(
                        action.expression_attribute_values
                    )
                transact_items.append({"Put": req})
                continue

            if isinstance(action, TransactDelete):
                req = {"TableName": self._table_name, "Key": self._to_key(action.pk, action.sk)}
                if action.condition_expression:
                    req["ConditionExpression"] = action.condition_expression
                if action.expression_attribute_names:
                    req["ExpressionAttributeNames"] = dict(action.expression_attribute_names)
                if action.expression_attribute_values:
                    req["ExpressionAttributeValues"] = self._serialize_values(
                        action.expression_attribute_values
                    )
                transact_items.append({"Delete": req})
                continue

            if isinstance(action, TransactUpdate):
                transact_items.append(
                    {
                        "Update": self._build_update_request(
                            action.pk,
                            action.sk,
                            action.updates,
                            condition_expression=action.condition_expression,
                            expression_attribute_names=action.expression_attribute_names,
                            expression_attribute_values=action.expression_attribute_values,
                        )
                    }
                )
                continue

            if isinstance(action, TransactConditionCheck):
                req = {
                    "TableName": self._table_name,
                    "Key": self._to_key(action.pk, action.sk),
                    "ConditionExpression": action.condition_expression,
                }
                if action.expression_attribute_names:
                    req["ExpressionAttributeNames"] = dict(action.expression_attribute_names)
                if action.expression_attribute_values:
                    req["ExpressionAttributeValues"] = self._serialize_values(
                        action.expression_attribute_values
                    )
                transact_items.append({"ConditionCheck": req})
                continue

            raise ValidationError(f"unsupported transaction action: {type(action).__name__}")

        try:
            self._client.transact_write_items(TransactItems=transact_items)
        except ClientError as err:  # pragma: no cover
            raise _map_transaction_error(err) from err

    def put(
        self,
        item: T,
        *,
        condition_expression: str | None = None,
        expression_attribute_names: Mapping[str, str] | None = None,
        expression_attribute_values: Mapping[str, Any] | None = None,
    ) -> None:
        try:
            dynamodb_item = self._to_item(item)
            req: dict[str, Any] = {"TableName": self._table_name, "Item": dynamodb_item}
            if condition_expression:
                req["ConditionExpression"] = condition_expression
            if expression_attribute_names:
                req["ExpressionAttributeNames"] = dict(expression_attribute_names)
            if expression_attribute_values:
                req["ExpressionAttributeValues"] = self._serialize_values(expression_attribute_values)
            self._client.put_item(**req)
        except ClientError as err:  # pragma: no cover (depends on AWS error shapes)
            raise _map_client_error(err) from err

    def get(self, pk: Any, sk: Any | None = None, *, consistent_read: bool = False) -> T:
        key = self._to_key(pk, sk)
        try:
            resp = self._client.get_item(TableName=self._table_name, Key=key, ConsistentRead=consistent_read)
        except ClientError as err:  # pragma: no cover
            raise _map_client_error(err) from err

        item = resp.get("Item")
        if not item:
            raise NotFoundError("item not found")
        return self._from_item(item)

    def delete(
        self,
        pk: Any,
        sk: Any | None = None,
        *,
        condition_expression: str | None = None,
        expression_attribute_names: Mapping[str, str] | None = None,
        expression_attribute_values: Mapping[str, Any] | None = None,
    ) -> None:
        key = self._to_key(pk, sk)
        req: dict[str, Any] = {"TableName": self._table_name, "Key": key}
        if condition_expression:
            req["ConditionExpression"] = condition_expression
        if expression_attribute_names:
            req["ExpressionAttributeNames"] = dict(expression_attribute_names)
        if expression_attribute_values:
            req["ExpressionAttributeValues"] = self._serialize_values(expression_attribute_values)

        try:
            self._client.delete_item(**req)
        except ClientError as err:  # pragma: no cover
            raise _map_client_error(err) from err

    def update(
        self,
        pk: Any,
        sk: Any | None,
        updates: Mapping[str, Any],
        *,
        condition_expression: str | None = None,
        expression_attribute_names: Mapping[str, str] | None = None,
        expression_attribute_values: Mapping[str, Any] | None = None,
    ) -> T:
        req = self._build_update_request(
            pk,
            sk,
            updates,
            condition_expression=condition_expression,
            expression_attribute_names=expression_attribute_names,
            expression_attribute_values=expression_attribute_values,
            return_values="ALL_NEW",
        )

        try:
            resp = self._client.update_item(**req)
        except ClientError as err:  # pragma: no cover
            raise _map_client_error(err) from err

        attrs = resp.get("Attributes")
        if not attrs:
            raise ValidationError("update did not return Attributes")
        return self._from_item(attrs)

    def update_builder(self, pk: Any, sk: Any | None = None) -> UpdateBuilder[T]:
        from .update_builder import UpdateBuilder

        return UpdateBuilder(self, pk, sk)

    def _build_update_request(
        self,
        pk: Any,
        sk: Any | None,
        updates: Mapping[str, Any],
        *,
        condition_expression: str | None = None,
        expression_attribute_names: Mapping[str, str] | None = None,
        expression_attribute_values: Mapping[str, Any] | None = None,
        return_values: str | None = None,
    ) -> dict[str, Any]:
        key = self._to_key(pk, sk)

        update_names: dict[str, str] = {}
        update_values: dict[str, Any] = {}
        set_parts: list[str] = []
        remove_parts: list[str] = []

        for field_name, value in updates.items():
            if field_name not in self._model.attributes:
                raise ValidationError(f"unknown field: {field_name}")
            if field_name == self._model.pk.python_name or (
                self._model.sk is not None and field_name == self._model.sk.python_name
            ):
                raise ValidationError(f"cannot update key field: {field_name}")

            attr_def = self._model.attributes[field_name]
            name_ref = f"#d_{field_name}"
            update_names[name_ref] = attr_def.attribute_name

            if value is None:
                remove_parts.append(name_ref)
                continue

            value_ref = f":d_{field_name}"
            update_values[value_ref] = self._serialize_attr_value(attr_def, value)
            set_parts.append(f"{name_ref} = {value_ref}")

        expr_parts: list[str] = []
        if set_parts:
            expr_parts.append("SET " + ", ".join(set_parts))
        if remove_parts:
            expr_parts.append("REMOVE " + ", ".join(remove_parts))
        if not expr_parts:
            raise ValidationError("no updates provided")

        req: dict[str, Any] = {
            "TableName": self._table_name,
            "Key": key,
            "UpdateExpression": " ".join(expr_parts),
            "ExpressionAttributeNames": update_names,
        }
        if update_values:
            req["ExpressionAttributeValues"] = update_values
        if return_values is not None:
            req["ReturnValues"] = return_values
        if condition_expression:
            req["ConditionExpression"] = condition_expression

        if expression_attribute_names:
            for k, v in expression_attribute_names.items():
                if k in req["ExpressionAttributeNames"]:
                    raise ValidationError(f"expression attribute name collision: {k}")
                req["ExpressionAttributeNames"][k] = v

        if expression_attribute_values:
            req.setdefault("ExpressionAttributeValues", {})
            serialized = self._serialize_values(expression_attribute_values)
            for k, v in serialized.items():
                if k in req["ExpressionAttributeValues"]:
                    raise ValidationError(f"expression attribute value collision: {k}")
                req["ExpressionAttributeValues"][k] = v

        return req

    def _serialize_values(self, values: Mapping[str, Any]) -> dict[str, Any]:
        out: dict[str, Any] = {}
        for k, v in values.items():
            out[k] = self._serializer.serialize(v)
        return out

    def _serialize_attr_value(self, attr_def: AttributeDefinition, value: Any) -> Any:
        if attr_def.converter is not None and value is not None:
            value = attr_def.converter.to_dynamodb(value)

        if attr_def.set and isinstance(value, set) and len(value) == 0:
            return self._serializer.serialize(None)

        if attr_def.json and value is not None:
            value = json.dumps(value, separators=(",", ":"), sort_keys=True)

        av = self._serializer.serialize(value)

        if attr_def.encrypted:
            if not self._kms_key_arn or self._kms_client is None:
                raise EncryptionNotConfiguredError(
                    f"encrypted field requires kms_key_arn: {attr_def.python_name}"
                )
            from .encryption import encrypt_attribute_value

            envelope = encrypt_attribute_value(
                av,
                attr_name=attr_def.attribute_name,
                kms_key_arn=self._kms_key_arn,
                kms_client=self._kms_client,
                rand_bytes=self._rand_bytes,
            )
            return self._serializer.serialize(envelope)

        return av

    def _to_item(self, item: T) -> dict[str, Any]:
        if not is_dataclass(item):
            raise ValidationError("item must be a dataclass instance")

        out: dict[str, Any] = {}
        for field_name, attr_def in self._model.attributes.items():
            value = getattr(item, field_name)
            if attr_def.omitempty and _is_empty(value):
                continue
            out[attr_def.attribute_name] = self._serialize_attr_value(attr_def, value)

        if self._model.pk.attribute_name not in out:
            raise ValidationError("missing pk")
        if self._model.sk is not None and self._model.sk.attribute_name not in out:
            raise ValidationError("missing sk")

        return out

    def _to_key(self, pk: Any, sk: Any | None) -> dict[str, Any]:
        if pk is None:
            raise ValidationError("pk is required")
        if self._model.sk is None and sk is not None:
            raise ValidationError("model does not define sk")
        if self._model.sk is not None and sk is None:
            raise ValidationError("sk is required")

        key: dict[str, Any] = {self._model.pk.attribute_name: self._serialize_attr_value(self._model.pk, pk)}
        if self._model.sk is not None:
            key[self._model.sk.attribute_name] = self._serialize_attr_value(self._model.sk, sk)
        return key

    def _from_item(self, item: Mapping[str, Any]) -> T:
        model_cls = self._model.model_type
        model_annotations = getattr(model_cls, "__annotations__", {})

        kwargs: dict[str, Any] = {}
        for dc_field in fields(cast(Any, model_cls)):
            if dc_field.name not in self._model.attributes:
                continue

            attr_def = self._model.attributes[dc_field.name]
            if attr_def.attribute_name not in item:
                continue

            if attr_def.encrypted:
                if not self._kms_key_arn or self._kms_client is None:
                    raise EncryptionNotConfiguredError(
                        f"encrypted field requires kms_key_arn: {attr_def.python_name}"
                    )

                from .encryption import decrypt_attribute_value

                envelope = self._deserializer.deserialize(item[attr_def.attribute_name])
                if not isinstance(envelope, dict):
                    raise ValidationError(f"encrypted envelope must be a map: {attr_def.python_name}")

                decrypted_av = decrypt_attribute_value(
                    envelope,
                    attr_name=attr_def.attribute_name,
                    kms_key_arn=self._kms_key_arn,
                    kms_client=self._kms_client,
                )
                raw = self._deserializer.deserialize(cast(Any, decrypted_av))
            else:
                raw = self._deserializer.deserialize(item[attr_def.attribute_name])
            if attr_def.json and isinstance(raw, str):
                raw = json.loads(raw)
            if attr_def.converter is not None and raw is not None:
                raw = attr_def.converter.from_dynamodb(raw)

            kwargs[dc_field.name] = _coerce_value(raw, model_annotations.get(dc_field.name, Any))

        try:
            return model_cls(**kwargs)
        except TypeError as err:
            raise ValidationError(str(err)) from err

    def _resolve_index(
        self, index_name: str | None
    ) -> tuple[str, str | None, Literal["TABLE", "GSI", "LSI"]]:
        if index_name is None:
            return (
                self._model.pk.attribute_name,
                self._model.sk.attribute_name if self._model.sk else None,
                "TABLE",
            )

        for idx in self._model.indexes:
            if idx.name == index_name:
                return (
                    idx.partition,
                    idx.sort,
                    cast(Literal["GSI", "LSI"], idx.type),
                )

        raise ValidationError(f"unknown index: {index_name}")

    def _apply_sort_condition(self, prefix: str, cond: SortKeyCondition, values: dict[str, Any]) -> str:
        op = cond.op
        if op in {"=", "<", "<=", ">", ">="}:
            if len(cond.values) != 1:
                raise ValidationError("invalid sort key condition")
            values[":sk"] = self._serializer.serialize(cond.values[0])
            return f"{prefix} AND #sk {op} :sk"
        if op == "between":
            if len(cond.values) != 2:
                raise ValidationError("invalid sort key condition")
            values[":sk1"] = self._serializer.serialize(cond.values[0])
            values[":sk2"] = self._serializer.serialize(cond.values[1])
            return f"{prefix} AND #sk BETWEEN :sk1 AND :sk2"
        if op == "begins_with":
            if len(cond.values) != 1:
                raise ValidationError("invalid sort key condition")
            values[":sk"] = self._serializer.serialize(cond.values[0])
            return f"{prefix} AND begins_with(#sk, :sk)"
        raise ValidationError(f"unsupported sort key operator: {op}")

    def _filter_expression(
        self,
        expr: FilterExpression,
        names: dict[str, str],
        values: dict[str, Any],
    ) -> str:
        counter = 0

        def name_ref(field_name: str) -> tuple[str, AttributeDefinition]:
            if field_name not in self._model.attributes:
                raise ValidationError(f"unknown field: {field_name}")

            attr_def = self._model.attributes[field_name]
            if attr_def.encrypted:
                raise ValidationError(f"encrypted fields cannot be filtered: {field_name}")

            ref = f"#f_{field_name}"
            existing = names.get(ref)
            if existing is not None and existing != attr_def.attribute_name:
                raise ValidationError(f"expression attribute name collision: {ref}")
            names[ref] = attr_def.attribute_name
            return ref, attr_def

        def value_ref(attr_def: AttributeDefinition, value: Any) -> str:
            nonlocal counter
            counter += 1
            ref = f":f{counter}"
            if ref in values:
                raise ValidationError(f"expression attribute value collision: {ref}")
            values[ref] = self._serialize_attr_value(attr_def, value)
            return ref

        def build(node: FilterExpression) -> str:
            if isinstance(node, FilterGroup):
                parts = [build(f) for f in node.filters]
                parts = [p for p in parts if p]
                if not parts:
                    return ""
                return "(" + f" {node.op} ".join(parts) + ")"

            if not isinstance(node, FilterCondition):
                raise ValidationError("invalid filter expression")

            name, attr_def = name_ref(node.field)
            op = node.op.upper()
            vals = node.values

            if op in {"=", "EQ"}:
                if len(vals) != 1:
                    raise ValidationError(f"{node.op} requires one value")
                return f"{name} = {value_ref(attr_def, vals[0])}"

            if op in {"!=", "<>", "NE"}:
                if len(vals) != 1:
                    raise ValidationError(f"{node.op} requires one value")
                return f"{name} <> {value_ref(attr_def, vals[0])}"

            if op in {"<", "LT"}:
                if len(vals) != 1:
                    raise ValidationError(f"{node.op} requires one value")
                return f"{name} < {value_ref(attr_def, vals[0])}"

            if op in {"<=", "LE"}:
                if len(vals) != 1:
                    raise ValidationError(f"{node.op} requires one value")
                return f"{name} <= {value_ref(attr_def, vals[0])}"

            if op in {">", "GT"}:
                if len(vals) != 1:
                    raise ValidationError(f"{node.op} requires one value")
                return f"{name} > {value_ref(attr_def, vals[0])}"

            if op in {">=", "GE"}:
                if len(vals) != 1:
                    raise ValidationError(f"{node.op} requires one value")
                return f"{name} >= {value_ref(attr_def, vals[0])}"

            if op == "BETWEEN":
                if len(vals) != 2:
                    raise ValidationError("BETWEEN requires two values")
                left = value_ref(attr_def, vals[0])
                right = value_ref(attr_def, vals[1])
                return f"{name} BETWEEN {left} AND {right}"

            if op == "IN":
                if len(vals) != 1:
                    raise ValidationError("IN requires a single sequence")
                in_values = vals[0]
                if not isinstance(in_values, Sequence) or isinstance(
                    in_values, (str, bytes, bytearray, dict)
                ):
                    raise ValidationError("IN requires a sequence of values")
                if len(in_values) > 100:
                    raise ValidationError("IN supports maximum 100 values")
                refs = [value_ref(attr_def, v) for v in in_values]
                return f"{name} IN (" + ", ".join(refs) + ")"

            if op == "BEGINS_WITH":
                if len(vals) != 1:
                    raise ValidationError("BEGINS_WITH requires one value")
                return f"begins_with({name}, {value_ref(attr_def, vals[0])})"

            if op == "CONTAINS":
                if len(vals) != 1:
                    raise ValidationError("CONTAINS requires one value")
                return f"contains({name}, {value_ref(attr_def, vals[0])})"

            if op in {"EXISTS", "ATTRIBUTE_EXISTS"}:
                if vals:
                    raise ValidationError("EXISTS does not take a value")
                return f"attribute_exists({name})"

            if op in {"NOT_EXISTS", "ATTRIBUTE_NOT_EXISTS"}:
                if vals:
                    raise ValidationError("NOT_EXISTS does not take a value")
                return f"attribute_not_exists({name})"

            raise ValidationError(f"unsupported filter operator: {node.op}")

        return build(expr)

    def _projection_expression(self, projection: list[str], names: dict[str, str]) -> str:
        required = self._required_fields()
        missing = required.difference(projection)
        if missing:
            raise ValidationError(f"projection is missing required fields: {sorted(missing)}")

        refs: list[str] = []
        for field_name in projection:
            if field_name not in self._model.attributes:
                raise ValidationError(f"unknown field: {field_name}")
            ref = f"#p_{field_name}"
            names[ref] = self._model.attributes[field_name].attribute_name
            refs.append(ref)
        return ", ".join(refs)

    def _required_fields(self) -> set[str]:
        required: set[str] = {self._model.pk.python_name}
        if self._model.sk is not None:
            required.add(self._model.sk.python_name)
        for dc_field in fields(cast(Any, self._model.model_type)):
            if dc_field.name not in self._model.attributes:
                continue
            if dc_field.default is MISSING and dc_field.default_factory is MISSING:
                required.add(dc_field.name)
        return required
