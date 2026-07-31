# Core Patterns (Python)

This document provides copy-pasteable patterns for the TableTheory Python SDK.

## Pattern: Define a model from a dataclass

```python
from dataclasses import dataclass

from tabletheory_py import ModelDefinition, theorydb_field


@dataclass(frozen=True)
class Note:
    pk: str = theorydb_field(roles=["pk"])
    sk: str = theorydb_field(roles=["sk"])
    value: int = theorydb_field()


model = ModelDefinition.from_dataclass(Note, table_name="notes_contract")
```

## Pattern: CRUD with a Table

```python
from tabletheory_py import Table

table = Table(model, client=client)
table.put(Note(pk="A", sk="1", value=123))
note = table.get("A", "1")
table.delete("A", "1")
```

## Pattern: Query + pagination

```python
from tabletheory_py import SortKeyCondition

page1 = table.query("A", sort=SortKeyCondition.begins_with("1"), limit=25)
page2 = table.query("A", cursor=page1.next_cursor) if page1.next_cursor else None
```

## Pattern: Batch + transactions

```python
table.batch_write(puts=[Note(pk="A", sk="2", value=1)], deletes=[("A", "1")])
```

## Pattern: explicitly clear an omit-empty field

DMS v0.2 treats keys in the update mapping as selected. Supplying an empty
value for a selected `omitempty=True` field removes the DynamoDB attribute:

```python
table.update(
    "USER#1",
    "PROFILE",
    {"nickname": ""},
    expected_version=current.version,
)
```

Fields absent from the mapping remain unchanged.

## Pattern: Lazy pagination for large reads

Use `query_iter` and `scan_iter` when a caller may stop before consuming every page. The generator fetches the next
DynamoDB page only when iteration advances:

```python
for page in table.query_iter("USER#1", limit=100):
    for item in page.items:
        if should_stop(item):
            break
    break  # no additional page is fetched
```

## Pattern: Aggregations are client-side

`sum_field`, `average_field`, `min_field`, `max_field`, `aggregate_field`, `count_distinct`, and `group_by(...).execute()`
operate on an already materialized Python sequence. If that sequence came from `query_all`, `scan_all`, or
`scan_all_segments`, every matching item is already in memory. Use these helpers only for bounded result sets. When the
planned native `count()` API lands, prefer it for count-only paths.

## Pattern: Streams unmarshalling

```python
from tabletheory_py import unmarshal_stream_record

note = unmarshal_stream_record(model, record, image="NewImage")
```

## Pattern: Deterministic unit tests (strict fakes)

```python
from tabletheory_py.mocks import ANY, FakeDynamoDBClient

fake = FakeDynamoDBClient()
fake.expect("put_item", {"TableName": "notes", "Item": {"PK": ANY, "SK": ANY}})
```

## Pattern: Key design and GSI queries

Keep key attributes explicit in the dataclass metadata. PK/SK and GSI key fields must be stable plaintext values;
encrypted fields are rejected for keys because DynamoDB must be able to route and compare key attributes.

```python
from dataclasses import dataclass

from tabletheory_py import IndexSpec, ModelDefinition, SortKeyCondition, Table, theorydb_field


@dataclass(frozen=True)
class User:
    pk: str = theorydb_field(name="PK", roles=["pk"])
    sk: str = theorydb_field(name="SK", roles=["sk"])
    gsi1pk: str = theorydb_field(name="GSI1PK", roles=["gsi1pk"])
    gsi1sk: str = theorydb_field(name="GSI1SK", roles=["gsi1sk"])
    email: str = theorydb_field()


model = ModelDefinition.from_dataclass(
    User,
    table_name="users_contract",
    indexes=[IndexSpec(name="gsi_email", type="GSI", partition="GSI1PK", sort="GSI1SK")],
)
table = Table(model, client=client)
page = table.query("EMAIL#ada@example.com", index_name="gsi_email", limit=25)
```

Do not request `consistent_read=True` on GSI queries; DynamoDB rejects strongly consistent GSI reads.

## Pattern: Fail-closed encryption

Encrypted fields require explicit KMS configuration. If a model has `encrypted=True` and `Table(...)` lacks a
`kms_key_arn`, construction fails closed instead of allowing plaintext fallback.

```python
from dataclasses import dataclass

from tabletheory_py import ModelDefinition, Table, theorydb_field


@dataclass(frozen=True)
class SecretNote:
    pk: str = theorydb_field(roles=["pk"])
    sk: str = theorydb_field(roles=["sk"])
    secret: str = theorydb_field(encrypted=True)


model = ModelDefinition.from_dataclass(SecretNote, table_name="secret_notes")
table = Table(
    model,
    client=client,
    kms_key_arn="arn:aws:kms:us-east-1:111111111111:key/example",
)
```

For unit tests, inject `FakeKmsClient` and deterministic random bytes; do not disable encryption in the model.

```python
from tabletheory_py.mocks import FakeDynamoDBClient, FakeKmsClient

fake_kms = FakeKmsClient(plaintext_key=b"\x01" * 32, ciphertext_blob=b"edk")
table = Table(
    model,
    client=FakeDynamoDBClient(),
    kms_key_arn="arn:aws:kms:us-east-1:111111111111:key/test",
    kms_client=fake_kms,
    rand_bytes=lambda n: b"\x02" * n,
)
```

## Pattern: Error handling by TableTheory type

Branch on exported exception types rather than message strings. Version conflicts remain condition failures for backward
compatibility while exposing the narrower conflict type.

```python
from tabletheory_py import ConditionFailedError, NotFoundError, VersionConflictError

try:
    table.update("USER#1", "PROFILE", {"nickname": "Ada"}, expected_version=3)
except VersionConflictError:
    # Reload, merge, and retry.
    raise
except ConditionFailedError:
    # Other conditional failure, such as create-if-absent collision.
    raise
except NotFoundError:
    # Missing item.
    raise
```
