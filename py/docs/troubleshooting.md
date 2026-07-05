# Troubleshooting (Python)

This guide maps common problems to verified fixes for the Python SDK.

## Error: `NoCredentialsError` when using DynamoDB Local

**Cause:** boto3 still requires credentials even for DynamoDB Local.

**Solution:** Provide dummy credentials:

```python
client = boto3.client(
    "dynamodb",
    endpoint_url="http://localhost:8000",
    region_name="us-east-1",
    aws_access_key_id="dummy",
    aws_secret_access_key="dummy",
)
```

## Error: encrypted fields without `kms_key_arn`

**Cause:** models with encrypted fields fail closed unless `kms_key_arn` is configured.

**Solution:** pass `kms_key_arn` and (in tests) inject a fake KMS client.

## Confusion: Git tag `vX.Y.Z-rc.N` vs Python version `X.Y.ZrcN`

**Cause:** Python follows PEP 440 prerelease normalization.

**Solution:** use the wheel filename from the GitHub Release asset list (e.g., `tabletheory_py-1.2.1rc1-...whl`).


## Error: `ResourceNotFoundException` or `TableNotFoundError`

**Cause:** the `ModelDefinition` table name does not exist at the configured boto3 endpoint/region, or the client points
at AWS when you expected DynamoDB Local.

**Fix:** verify `table_name`, `endpoint_url`, and `region_name`, then create the table through infrastructure or your
integration-test setup.

```python
client = boto3.client(
    "dynamodb",
    endpoint_url="http://localhost:8000",
    region_name="us-east-1",
    aws_access_key_id="dummy",
    aws_secret_access_key="dummy",
)
```

## Error: missing or empty key fields

**Cause:** `put`, `get`, `update`, or `delete` received an item/key without the dataclass fields marked `roles=["pk"]`
and `roles=["sk"]`. DynamoDB key attributes cannot be empty strings.

**Fix:** use a small key helper and keep field names aligned with `theorydb_field(name=...)`.

```python
def profile_key(user_id: str) -> tuple[str, str]:
    return (f"USER#{user_id}", "PROFILE")

pk, sk = profile_key("42")
item = table.get(pk, sk)
```

## Error: GSI query returns no items or rejects consistent reads

**Cause:** GSI reads require `index_name=...`, and DynamoDB rejects `consistent_read=True` for GSIs.

**Fix:** pass the index name and leave `consistent_read` at its default `False`.

```python
page = table.query("EMAIL#a@example.com", index_name="gsi_email", limit=25)
```

## Error: encrypted fields without `kms_key_arn`

**Cause:** models with encrypted fields fail closed unless `kms_key_arn` is configured.

**Fix:** pass `kms_key_arn` in production and inject `FakeKmsClient` in tests. Do not remove `encrypted=True` to make a
local test pass.

## Error: encrypted field used in a filter or condition

**Cause:** DynamoDB sees encrypted envelopes, not plaintext, so encrypted attributes cannot be queried or compared.

**Fix:** query by plaintext PK/SK/GSI attributes and keep secrets in non-key encrypted fields.

## Pagination surprise: `query_all` or `scan_all` loads too much data

**Cause:** `query_all`, `scan_all`, and `scan_all_segments` follow every cursor and materialize the full result set.

**Fix:** use `query`, `query_iter`, `scan`, or `scan_iter` for large reads.

```python
for page in table.query_iter("TENANT#1", limit=100):
    for item in page.items:
        if done(item):
            break
    break
```

## Testing surprise: timestamp assertions are flaky

**Cause:** lifecycle fields are populated from the runtime clock.

**Fix:** inject `now` and strict fakes in unit tests.

```python
from datetime import datetime, timezone

from tabletheory_py.mocks import FakeDynamoDBClient

table = Table(
    model,
    client=FakeDynamoDBClient(),
    now=lambda: datetime(2026, 1, 16, tzinfo=timezone.utc),
)
```
