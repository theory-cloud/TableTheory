# Testing Guide (Python)

This guide documents how to test Python services that use `theorydb_py`.

## Unit testing (recommended default)

Use strict fakes from `theorydb_py.mocks` to avoid real AWS calls and to keep tests deterministic.

```python
from theorydb_py.mocks import ANY, FakeDynamoDBClient, FakeKmsClient

fake_ddb = FakeDynamoDBClient()
fake_kms = FakeKmsClient(plaintext_key=b"\x00" * 32, ciphertext_blob=b"edk")

fake_ddb.expect("put_item", {"TableName": "notes", "Item": {"PK": ANY, "SK": ANY, "secret": ANY}})
```

### ✅ CORRECT: deterministic encryption inputs

For tests involving encryption, inject deterministic `rand_bytes`:

```python
table = Table(model, client=fake_ddb, kms_key_arn="arn:aws:kms:...", kms_client=fake_kms, rand_bytes=lambda n: b"\x01" * n)
```

## Integration testing (DynamoDB Local)

Use DynamoDB Local to validate real DynamoDB constraints (pagination, conditional writes, batch limits).

From repo root:

```bash
make docker-up
uv --directory py run pytest -q
```

Environment variables (typical for local):
- `DYNAMODB_ENDPOINT` (default `http://localhost:8000`)
- `AWS_REGION` (default `us-east-1`)
- `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` (use `dummy`)

