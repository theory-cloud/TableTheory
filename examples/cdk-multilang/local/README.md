# Local variant (no AWS account)

The parent [cdk-multilang demo](../README.md) deploys real AWS resources to prove
that Go, Node.js, and Python share one DynamoDB table without drift. This local
variant proves the same **cross-language no-drift** property against a throwaway
[DynamoDB Local](https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/DynamoDBLocal.html)
container — **no AWS account, no credentials, no cloud mutation**.

## What it does

`run-local.sh`:

1. **Drift-checks the generated CDK construct.** It regenerates the table
   construct from `dms/demo.yml` with `tabletheory gen --cdk` and diffs it against
   the committed [`generated-table.ts`](./generated-table.ts). This is the same
   generator the parent stack's table shape aligns to.
2. **Starts a throwaway DynamoDB Local** on an isolated port (`8020` by default).
3. **Go writes** one shared `DemoItem` to the table.
4. **Node.js reads** it back with the in-repo TypeScript runtime and asserts the shape.
5. **Python reads** it back with the in-repo Python runtime and asserts the shape.

If any runtime marshaled the shared item differently, a read would fail — that is
the drift the framework exists to prevent.

## Run it

From the repo root:

```bash
bash examples/cdk-multilang/local/run-local.sh
```

Expected final line:

```
LOCAL CROSS-LANGUAGE NO-DRIFT: PASS
```

The script needs Docker (for DynamoDB Local), the Go toolchain, Node.js 24, and
`uv` for Python — the same tools the contract suite uses. It installs the
TypeScript runtime dev dependencies (`npm --prefix ts ci`) on first run if needed.

## What is intentionally not covered locally

**Encryption.** The parent demo encrypts the `secret` attribute through a KMS
envelope. Encrypted fields **fail closed** without KMS, which is not available in a
no-AWS run, so the local `DemoItem` omits `secret`. Exercise encryption end to end
with the deployed AWS variant ([`scripts/smoke.sh`](../scripts/smoke.sh)).
