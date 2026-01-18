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

**Solution:** use the wheel filename from the GitHub Release asset list (e.g., `theorydb_py-1.2.1rc1-...whl`).

