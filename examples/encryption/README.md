# `theorydb:"encrypted"` Example

This example demonstrates a simple encrypt → store → load → decrypt round-trip using AWS KMS envelope encryption.

## Prerequisites

- AWS credentials with permission to use DynamoDB and KMS
- A KMS key ARN for `GenerateDataKey`/`Decrypt`

## Environment

- `AWS_REGION` (optional; default `us-east-1`)
- `KMS_KEY_ARN` (required)
- `DYNAMORM_ENCRYPTION_EXAMPLE_TABLE` (optional; default `theorydb-encryption-example`)
- `DYNAMORM_ENCRYPTION_CREATE_TABLE=1` (optional; creates the table if missing)

## Run

```bash
go run ./examples/encryption
```

