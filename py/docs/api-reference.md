# API Reference (Python)

<!-- AI Training: This is the API reference for the TableTheory Python SDK -->
**This document describes the public API surface of `theorydb_py` at a signature-and-shape level.**

## Imports

```python
from theorydb_py import ModelDefinition, Table, theorydb_field
from theorydb_py import SortKeyCondition
from theorydb_py import unmarshal_stream_record
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

## Pagination

Query returns a page object with `items` and `next_cursor` (opaque token). Pass `next_cursor` back into the next call.

## Streams

- `unmarshal_stream_record(model, record, *, image="NewImage")`

## Encryption

Encrypted fields are stored as an envelope map. If a model contains encrypted fields, `Table(...)` fails closed unless
`kms_key_arn` is configured.

