# `theorydb:"encrypted"` Example

This example demonstrates an encrypt → store → load → decrypt round-trip using
AWS KMS envelope encryption. It also reads the raw stored item and proves the
attribute on disk is a ciphertext envelope, not plaintext.

## Run locally (no AWS account)

`make examples-local` runs this example as `L2` against DynamoDB Local and the
local KMS emulator. The harness provisions the key, exports `KMS_KEY_ARN`, and
sets every endpoint override, so no AWS account or real credentials are used:

```bash
make examples-local
```

The row asserts:

- the decrypted field round-trips to the original plaintext;
- the stored attribute is a v2 envelope (`v`, `edk`, `nonce`, `ct`) whose
  ciphertext does not contain the plaintext.

Gap: the KMS is emulated (moto) — no HSM, key policies or grants. The
`GenerateDataKey`/`Decrypt` calls and the AES-GCM envelope are real.

## Run against AWS

### Prerequisites

- AWS credentials with permission to use DynamoDB and KMS
- A KMS key ARN for `GenerateDataKey`/`Decrypt`

### Environment

- `AWS_REGION` (optional; default `us-east-1`)
- `KMS_KEY_ARN` (required)
- `DYNAMORM_ENCRYPTION_EXAMPLE_TABLE` (optional; default `theorydb-encryption-example`)
- `DYNAMORM_ENCRYPTION_CREATE_TABLE=1` (optional; creates the table if missing)

### Run

```bash
go run ./examples/encryption
```
