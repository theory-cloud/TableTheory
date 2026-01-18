# Core Patterns (Python)

This document provides copy-pasteable patterns for the TableTheory Python SDK.

## Pattern: Define a model from a dataclass

```python
from dataclasses import dataclass

from theorydb_py import ModelDefinition, theorydb_field


@dataclass(frozen=True)
class Note:
    pk: str = theorydb_field(roles=["pk"])
    sk: str = theorydb_field(roles=["sk"])
    value: int = theorydb_field()


model = ModelDefinition.from_dataclass(Note, table_name="notes_contract")
```

## Pattern: CRUD with a Table

```python
from theorydb_py import Table

table = Table(model, client=client)
table.put(Note(pk="A", sk="1", value=123))
note = table.get("A", "1")
table.delete("A", "1")
```

## Pattern: Query + pagination

```python
from theorydb_py import SortKeyCondition

page1 = table.query("A", sort=SortKeyCondition.begins_with("1"), limit=25)
page2 = table.query("A", cursor=page1.next_cursor) if page1.next_cursor else None
```

## Pattern: Batch + transactions

```python
table.batch_write(puts=[Note(pk="A", sk="2", value=1)], deletes=[("A", "1")])
```

## Pattern: Streams unmarshalling

```python
from theorydb_py import unmarshal_stream_record

note = unmarshal_stream_record(model, record, image="NewImage")
```

## Pattern: Deterministic unit tests (strict fakes)

```python
from theorydb_py.mocks import ANY, FakeDynamoDBClient

fake = FakeDynamoDBClient()
fake.expect("put_item", {"TableName": "notes", "Item": {"PK": ANY, "SK": ANY}})
```

