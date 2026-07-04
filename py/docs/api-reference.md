# API Reference (Python)

<!-- AI Training: This is the API reference for the TableTheory Python SDK -->
**This document describes the public API surface of `tabletheory_py` at a signature-and-shape level.**

## Imports

```python
from tabletheory_py import ModelDefinition, Table, theorydb_field
from tabletheory_py import SortKeyCondition
from tabletheory_py import unmarshal_stream_record
from tabletheory_py import (
    DEFAULT_LAMBDA_TIMEOUT_BUFFER_SECONDS,
    LambdaTimeoutConfig,
    check_lambda_timeout,
    with_lambda_timeout,
)
from tabletheory_py import TransactConditionCheck, TransactDelete, TransactPut, TransactUpdate
from tabletheory_py import UpdateAdd, UpdateSetIfNotExists
from tabletheory_py import aggregate_field, average_field, count_distinct, group_by, max_field, min_field, sum_field
from tabletheory_py.mocks import FakeDynamoDBClient, FakeKmsClient
```

## Model Definition

### `theorydb_field(...)`

Declares attribute metadata for dataclass fields (roles, omitempty, encryption, defaults, converters, etc.).

### `ModelDefinition.from_dataclass(dataclass_type, table_name=...)`

Builds a model definition from a dataclass and a table name.

## Table

### `Table(model, *, client=None, table_name=None, kms_key_arn=None, kms_client=None, rand_bytes=None, now=None)`

Primary entrypoint for operations. `client` is a boto3-compatible DynamoDB client; when omitted, `Table` constructs one
with `boto3.client("dynamodb")`. If any model attribute is encrypted, construction fails closed unless `kms_key_arn` is
provided.

### CRUD

Actual signatures:

- `put(item, *, condition_expression=None, expression_attribute_names=None, expression_attribute_values=None) -> None`
- `save(item, *, condition_expression=None, expression_attribute_names=None, expression_attribute_values=None) -> None`
- `get(pk, sk=None, *, consistent_read=False) -> T`
- `update(pk, sk, updates, *, condition_expression=None, expression_attribute_names=None, expression_attribute_values=None, protected_attributes=(), expected_version=None) -> T`
- `delete(pk, sk=None, *, condition_expression=None, expression_attribute_names=None, expression_attribute_values=None) -> None`
- `update_builder(pk, sk=None) -> UpdateBuilder[T]`

Notes:

- `put` does **not** accept an `if_not_exists` parameter. Use a `condition_expression` when a one-off condition is needed;
  write-once/protected models add their create-if-absent condition automatically.
- `save` is a mutable-model convenience wrapper around `put` and rejects write-once/protected overwrite paths.
- `update` accepts a mapping of field names to values. Use `UpdateAdd(...)` or `UpdateSetIfNotExists(...)` inside the
  mapping for DynamoDB `ADD` and `if_not_exists` update expressions.

## Query and scan

### `query(partition, *, sort=None, index_name=None, limit=None, cursor=None, scan_forward=True, consistent_read=False, projection=None, filter=None) -> Page[T]`

All query parameters are listed above:

- `partition`: required partition-key value
- `sort`: optional `SortKeyCondition`
- `index_name`: optional GSI/LSI name
- `limit`: DynamoDB page limit; must be greater than zero when provided
- `cursor`: opaque cursor returned as `Page.next_cursor`
- `scan_forward`: `True` for ascending sort-key order, `False` for descending
- `consistent_read`: defaults to `False`; rejected for GSIs
- `projection`: optional list of projected attribute names
- `filter`: optional `FilterExpression`

### `query_all(...) -> list[T]`

Accepts the same query parameters and follows cursors until exhaustion. It materializes every matching item into a Python
list; prefer `query_iter(...)` or page-by-page `query` calls for large partitions.

### `query_iter(...) -> Iterator[Page[T]]`

Accepts the same query parameters and yields one `Page[T]` at a time. This is the preferred large-result path because the
next DynamoDB page is fetched only when the caller advances the generator; breaking out of the loop stops pagination.

### `query_with_retry(..., max_retries=5, initial_delay_seconds=0.1, max_delay_seconds=5.0, backoff_factor=2.0, retry_on_empty=True, retry_on_error=True, verify=None, sleep=time.sleep) -> Page[T]`

Retries query pages for eventually consistent reads or custom `verify` checks. Lambda timeout errors are not swallowed.

### `scan(*, index_name=None, limit=None, cursor=None, consistent_read=False, projection=None, filter=None, segment=None, total_segments=None) -> Page[T]`

Runs a DynamoDB Scan and returns a page with `items` and `next_cursor`.

### `scan_all(...) -> list[T]`

Accepts the same scan parameters and follows cursors until exhaustion. It materializes every scanned item into a Python
list; prefer `scan_iter(...)` or page-by-page `scan` calls for large tables.

### `scan_iter(...) -> Iterator[Page[T]]`

Accepts the same scan parameters and yields one `Page[T]` at a time. Use it for large scans so consumers can stop early
without fetching the remaining pages.

### `scan_all_segments(*, total_segments, index_name=None, limit=None, consistent_read=False, projection=None, filter=None, max_workers=None) -> list[T]`

Runs a parallel scan across `total_segments` and materializes the combined result list.

## Client-side aggregation helpers

The Python package exports client-side helpers:

- `sum_field(items, field)`
- `average_field(items, field)`
- `min_field(items, field)`
- `max_field(items, field)`
- `aggregate_field(items, field=None)`
- `count_distinct(items, field)`
- `group_by(items, field).count(alias).sum(field, alias).avg(field, alias).min(field, alias).max(field, alias).execute()`

These helpers operate on an already materialized Python sequence. If the input came from `query_all`, `scan_all`, or
`scan_all_segments`, every matching item is already resident in memory. Use only for bounded result sets. There is no
native server-side `count()` in this release; when the planned native `count()` API lands, prefer it for count-only paths.

## Batch + transactions

Actual signatures and defaults:

- `batch_get(keys, *, consistent_read=False, projection=None, max_retries=5, sleep=time.sleep) -> list[T]`
- `batch_write(*, puts=(), deletes=(), max_retries=5, sleep=time.sleep) -> None`
- `transact_get(keys, *, projection=None) -> list[T | None]`
- `transact_write(actions) -> None`

`transact_get` accepts 1-100 keys and returns one result slot per requested key; missing items are `None`.

`transact_write(actions)` accepts a sequence of dataclass actions:

- `TransactPut(item, condition_expression=None, expression_attribute_names=None, expression_attribute_values=None)`
- `TransactUpdate(pk, sk, updates, condition_expression=None, expression_attribute_names=None, expression_attribute_values=None, protected_attributes=())`
- `TransactDelete(pk, sk=None, condition_expression=None, expression_attribute_names=None, expression_attribute_values=None)`
- `TransactConditionCheck(pk, sk, condition_expression, expression_attribute_names=None, expression_attribute_values=None)`

The Python runtime rejects an empty action list and more than 100 actions before submitting to DynamoDB.

## Lambda runtime helpers

### `DEFAULT_LAMBDA_TIMEOUT_BUFFER_SECONDS`

The default buffer, `1.0`, left before the Lambda hard timeout.

### `LambdaTimeoutConfig`

Frozen dataclass shape used by the runtime wrapper:

```python
LambdaTimeoutConfig(buffer_seconds=1.0)
```

### `check_lambda_timeout(context, *, buffer_seconds=1.0)`

Checks the AWS Lambda context before work starts. If the remaining invocation time is less than or equal to the buffer,
the helper raises the built-in `TimeoutError`.

### `with_lambda_timeout(client, context, *, buffer_seconds=1.0)`

Returns a derived client wrapper that checks `check_lambda_timeout` before each non-private callable client method. If no
Lambda deadline is available, the original client is returned unchanged.

Python cannot cancel an in-flight boto3 call once it has started; this helper is a pre-call guard that prevents starting
new DynamoDB work when the invocation is already inside the cleanup buffer.

### `Table.with_lambda_timeout(context, *, buffer_seconds=1.0)`

Returns an invocation-scoped `Table` that preserves the model, table name, encryption configuration, KMS client, and
random-byte source while wrapping the DynamoDB client with the Lambda timeout guard.

```python
base_table = Table(model, client=client)


def handler(event: dict, context: object) -> dict:
    table = base_table.with_lambda_timeout(context, buffer_seconds=0.5)
    item = table.get("USER#1", "PROFILE")
    return {"item": item}
```

## Pagination

Query and scan methods return a page object with `items` and `next_cursor` (opaque token). Pass `next_cursor` back into
the next call.

## Streams

- `unmarshal_stream_record(model, record, *, image="NewImage")`

## Encryption

Encrypted fields are stored as an envelope map. If a model contains encrypted fields, `Table(...)` fails closed unless
`kms_key_arn` is configured.
