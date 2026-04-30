# API Reference (Python)

<!-- AI Training: This is the API reference for the TableTheory Python SDK -->
**This document describes the public API surface of `theorydb_py` at a signature-and-shape level.**

## Imports

```python
from theorydb_py import ModelDefinition, Table, theorydb_field
from theorydb_py import SortKeyCondition
from theorydb_py import unmarshal_stream_record
from theorydb_py import (
    DEFAULT_LAMBDA_TIMEOUT_BUFFER_SECONDS,
    LambdaTimeoutConfig,
    check_lambda_timeout,
    with_lambda_timeout,
)
from theorydb_py.mocks import FakeDynamoDBClient, FakeKmsClient
```

## Model Definition

### `theorydb_field(...)`

Declares attribute metadata for dataclass fields (roles, omitempty, encryption, etc.).

### `ModelDefinition.from_dataclass(dataclass_type, table_name=...)`

Builds a model definition from a dataclass and a table name.

## Table

### `Table(model, client, *, kms_key_arn=None, kms_client=None, rand_bytes=None, now=None)`

Primary entrypoint for operations.

Common operations:
- `put(item, *, if_not_exists=False, condition_expression=None, ...)`
- `get(pk, sk, *, consistent_read=False)`
- `update(pk, sk, updates, *, expected_version=None, ...)`
- `delete(pk, sk, *, condition_expression=None, ...)`
- `query(pk, *, sort=None, limit=None, cursor=None)`
- `batch_get(keys, *, consistent_read=False)`
- `batch_write(puts=None, deletes=None)`
- `transact_write(operations)`
- `with_lambda_timeout(context, *, buffer_seconds=DEFAULT_LAMBDA_TIMEOUT_BUFFER_SECONDS)`

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

Query returns a page object with `items` and `next_cursor` (opaque token). Pass `next_cursor` back into the next call.

## Streams

- `unmarshal_stream_record(model, record, *, image="NewImage")`

## Encryption

Encrypted fields are stored as an envelope map. If a model contains encrypted fields, `Table(...)` fails closed unless
`kms_key_arn` is configured.
