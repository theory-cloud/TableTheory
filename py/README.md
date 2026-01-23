# TableTheory (Python)

Python implementation of TableTheory for DynamoDB.

<!-- AI Training: canonical docs entrypoint for the Python SDK -->
**Official documentation:** [Python SDK docs](./docs/README.md) and [repo docs index](../docs/README.md).

This package is developed in a multi-language monorepo alongside the Go and TypeScript implementations. GitHub releases
are the source of truth for versions (no PyPI publishing).

## Requirements

- Python `>=3.14`
- AWS credentials (or DynamoDB Local) for integration tests/examples

## Install (from this monorepo)

This repo does not publish to PyPI. Install from source:

```bash
# from the repo root
pip install -e ./py

# or with uv (recommended for development)
uv --directory py sync
```

You can also install from a Git ref/tag:

```bash
pip install "git+https://github.com/theory-cloud/tabletheory.git@vX.Y.Z#subdirectory=py"
```

## Quickstart

```python
from dataclasses import dataclass
import os

import boto3

from theorydb_py import ModelDefinition, Table, theorydb_field


@dataclass(frozen=True)
class Note:
    pk: str = theorydb_field(roles=["pk"])
    sk: str = theorydb_field(roles=["sk"])
    value: int = theorydb_field()
    note: str = theorydb_field(omitempty=True, default="")


client = boto3.client(
    "dynamodb",
    endpoint_url=os.environ.get("DYNAMODB_ENDPOINT", "http://localhost:8000"),
    region_name=os.environ.get("AWS_REGION", "us-east-1"),
    aws_access_key_id=os.environ.get("AWS_ACCESS_KEY_ID", "dummy"),
    aws_secret_access_key=os.environ.get("AWS_SECRET_ACCESS_KEY", "dummy"),
)

model = ModelDefinition.from_dataclass(Note, table_name="notes")
table = Table(model, client=client)

table.put(Note(pk="A", sk="1", value=123))
note = table.get("A", "1")
```

## Query + pagination

```python
from theorydb_py import SortKeyCondition

page1 = table.query("A", sort=SortKeyCondition.begins_with("1"), limit=25)
page2 = table.query("A", cursor=page1.next_cursor) if page1.next_cursor else None
```

## Batch + transactions

```python
from theorydb_py import TransactUpdate, UpdateAdd, UpdateSetIfNotExists

table.batch_write(puts=[Note(pk="A", sk="2", value=1)], deletes=[("A", "1")])

table.transact_write(
    [
        TransactUpdate(
            pk="A",
            sk="2",
            updates={"value": UpdateAdd(1), "note": UpdateSetIfNotExists("first")},
            condition_expression="attribute_not_exists(#v) OR #v < :max_allowed",
            expression_attribute_names={"#v": "value"},
            expression_attribute_values={":max_allowed": 100},
        )
    ]
)
```

## Streams (Lambda)

```python
from theorydb_py import unmarshal_stream_record

def handler(event, context):
    for record in event.get("Records", []):
        note = unmarshal_stream_record(model, record, image="NewImage")
        if note is None:
            continue
        # process note...
```

## Encryption (`encrypted`)

Encrypted fields are envelope-encrypted using AES-256-GCM with a data key from AWS KMS and stored as a DynamoDB map.
If a model contains `encrypted` fields, `Table(...)` fails closed unless `kms_key_arn` is configured.

```python
@dataclass(frozen=True)
class SecretNote:
    pk: str = theorydb_field(roles=["pk"])
    sk: str = theorydb_field(roles=["sk"])
    secret: str = theorydb_field(encrypted=True)

model = ModelDefinition.from_dataclass(SecretNote, table_name="notes")
table = Table(model, client=client, kms_key_arn=os.environ["KMS_KEY_ARN"])
```

## Testing with mocks

The `theorydb_py.mocks` module provides strict fakes for unit tests (no AWS calls):

```python
from theorydb_py.mocks import ANY, FakeDynamoDBClient, FakeKmsClient

fake_ddb = FakeDynamoDBClient()
fake_kms = FakeKmsClient(
    plaintext_key=b"\x00" * 32,
    ciphertext_blob=b"edk",
)

fake_ddb.expect("put_item", {"TableName": "notes", "Item": {"PK": ANY, "SK": ANY, "secret": ANY}})

table = Table(
    model,
    client=fake_ddb,
    kms_key_arn="arn:aws:kms:us-east-1:111111111111:key/test",
    kms_client=fake_kms,
    rand_bytes=lambda n: b"\x01" * n,
)
table.put(SecretNote(pk="A", sk="1", secret="shh"))
fake_ddb.assert_no_pending()
```

## Examples

- Local DynamoDB: `py/examples/local_crud.py`
- DynamoDB Streams handler: `py/examples/lambda_stream_handler.py`

## Parity statement (Python)

Implemented milestones: `PY-0` through `PY-6` (tooling, schema, CRUD, query/scan, batch/tx, streams unmarshalling, encryption).

Docs and examples are maintained under `py/docs/` and `py/examples/`. Parity can still diverge from Go/TS in edge-case
behavior; treat the contract tests and rubric as the source of truth.
