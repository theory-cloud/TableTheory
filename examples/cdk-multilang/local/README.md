# Local variant (no AWS account)

The parent [cdk-multilang demo](../README.md) deploys real AWS resources to prove
that Go, Node.js, and Python share one DynamoDB table without drift. This local
variant proves the same **cross-language no-drift** property — including the
encrypted `secret` field — against a throwaway
[DynamoDB Local](https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/DynamoDBLocal.html)
container and a local KMS emulator: **no AWS account, no real credentials, no
cloud mutation**.

## What it does

`run-local.sh`:

1. **Drift-checks the generated CDK construct.** It regenerates the table
   construct from `dms/demo.yml` with `tabletheory gen --cdk` and diffs it against
   the committed [`generated-table.ts`](./generated-table.ts). This is the same
   generator the parent stack's table shape aligns to.
2. **Starts a throwaway DynamoDB Local** on an ephemeral loopback port (or
   `CDK_LOCAL_PORT` when set).
3. **Go writes** one shared `DemoItem` whose `secret` field is encrypted through
   the local KMS, and asserts the value decrypts on read.
4. **Node.js reads** it back with the in-repo TypeScript runtime, decrypts the
   secret through the local KMS, and asserts the shape.
5. **Python reads** it back with the in-repo Python runtime, decrypts the secret
   through the local KMS, and asserts the shape.

If any runtime marshaled the shared item or the envelope differently, a read or a
decrypt would fail — that is the drift the framework exists to prevent.

## Run it

The simple path is the examples harness, which provisions DynamoDB Local, the KMS
key and every endpoint for you:

```bash
make examples-local    # runs this row as L2
```

To run it by hand, point `KMS_KEY_ARN` at a key in a local KMS endpoint
(`AWS_ENDPOINT_URL_KMS`/`KMS_ENDPOINT`) and provide dummy credentials:

```bash
export AWS_ACCESS_KEY_ID=dummy AWS_SECRET_ACCESS_KEY=dummy
export AWS_REGION=us-east-1
export AWS_ENDPOINT_URL_KMS=http://127.0.0.1:5000
export KMS_KEY_ARN=<key-arn-in-the-local-kms>
bash examples/cdk-multilang/local/run-local.sh
```

Expected final line:

```
LOCAL CROSS-LANGUAGE NO-DRIFT: PASS
```

The script needs Docker (for DynamoDB Local), the Go toolchain, Node.js 24, and
`uv` for Python — the same tools the contract suite uses. It installs the
TypeScript runtime dev dependencies (`npm --prefix ts ci`) on first run if needed.

DynamoDB Local enforces no authentication, so `run-local.sh` publishes its port
on loopback only; without `CDK_LOCAL_PORT` it asks Docker for an ephemeral port
and discovers it, so two runs cannot collide.

## Fidelity limits

The KMS is an emulator (moto): there is no HSM, no key policy, IAM or grant
enforcement, and no cross-account semantics. The `GenerateDataKey`/`Decrypt`
calls, the envelope cryptography and the cross-runtime decrypt are real. See
[`docs/examples/local-harness.md`](../../../docs/examples/local-harness.md) for
the full emulator decision and limits.
