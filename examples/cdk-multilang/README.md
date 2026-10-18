# TableTheory CDK Multi-language Demo

Deploys **two DynamoDB tables** and **four Lambdas**:

- one shared application table exercised by Go, Node.js 24, and Python 3.14
- one TTL-driven evidence table with a DynamoDB Streams to S3 archival Lambda

This is the deployable “proof” that the multi-language TableTheory stack can share a single table without drift while
also supporting TTL-based retention pipelines.

> **No AWS account?** The [`local/`](./local/README.md) variant proves the same cross-language no-drift property against
> DynamoDB Local with `bash examples/cdk-multilang/local/run-local.sh` — no credentials or cloud mutation required.

This demo also exercises:

- **Encryption** (KMS envelope, cross-language decrypt)
- **Batching** (BatchWrite + BatchGet)
- **TTL + archival** (DynamoDB TTL, Streams, S3 lifecycle to Glacier)
- **Transactions** (TransactWrite)

## Commands

From the repo root:

- Install deps: `npm --prefix examples/cdk-multilang ci`
- Synthesize: `npm --prefix examples/cdk-multilang run synth`
- Test: `npm --prefix examples/cdk-multilang run test`
- Deploy (writes `cdk.outputs.json`): `AWS_PROFILE=... npm --prefix examples/cdk-multilang run deploy -- --profile $AWS_PROFILE --outputs-file cdk.outputs.json`

After deploy, the stack outputs three Function URLs. The URLs require IAM authentication (`AWS_IAM`), so every request must be SigV4-signed with credentials allowed to invoke the function URL:

```bash
curl --aws-sigv4 "aws:amz:us-east-1:lambda" \
  --user "$AWS_ACCESS_KEY_ID:$AWS_SECRET_ACCESS_KEY" \
  "https://<url-id>.lambda-url.us-east-1.on.aws/?pk=...&sk=..."
```

`scripts/smoke.sh` signs its requests with the exported `AWS_PROFILE` credentials. Endpoints:

- `GET ?pk=...&sk=...`
- `PUT` with JSON body: `{"pk":"...","sk":"...","value":"...","secret":"..."}`

Additional endpoints:

- `PUT /enc` (encryption demo; same payload as `PUT /`)
- `POST /batch` (batch write + batch get): `{"pk":"...","skPrefix":"...","count":3,"value":"...","secret":"..."}`
- `POST /tx` (transaction write): `{"pk":"...","skPrefix":"...","value":"...","secret":"..."}`

## Dependency audit note

The current pin is `aws-cdk-lib@2.273.0`, which publishes bundled `brace-expansion@5.0.12` — the fixed version for the
three brace-expansion advisories earlier `aws-cdk-lib` releases shipped. The retired copies were `5.0.9`, bundled under
`node_modules/aws-cdk-lib/node_modules/brace-expansion`, which npm overrides cannot replace because AWS publishes that
dependency inside `aws-cdk-lib`.

With the fixed bundle in place `npm audit` reports no findings for this example, so
`gov-infra/planning/theorydb-visible-npm-audit-findings.json` now carries an empty `findings` list. The three visible
entries recorded on 2026-10-03 were retired on 2026-10-11 under their documented `remove_when` condition (aws-cdk-lib
bundles brace-expansion `>= 5.0.12`). They were only ever recorded in that visible-findings file, never in the
supply-chain suppression allowlist, and no allowlist suppression was added in their place.

`scripts/test-npm-audit-policy.sh` is the guard that keeps the retired state honest. It asserts the fixed graph directly
and fails closed in every direction:

- `aws-cdk-lib` must be `>= 2.273.0`, with a resolved tarball URL and a `sha512-` integrity hash.
- The bundled `node_modules/aws-cdk-lib/node_modules/brace-expansion` entry must exist, be marked `inBundle: true`, and
  resolve to `>= 5.0.12`; the bundled `minimatch` must be `inBundle: true` and its declared brace-expansion range must be
  satisfied by the actual bundled version.
- No `brace-expansion` below `5.0.12` may exist anywhere in the lockfile, and no residual visible policy entry may
  reference the retired exception.

Synthetic negative controls (an old `5.0.9` bundle, a missing path/entry/integrity/resolved field, a new vulnerable root
entry, a mismatched minimatch range, a residual policy entry in the wrong advisory/package/node shape, a malformed
lockfile or policy, and a failed or empty audit transport) all fail closed, so the guard is green only while the fixed
bundle and the retired policy state agree. When AWS publishes a future bundle, take the version bump and record the new
bundle — do not re-add a visible entry or an allowlist suppression to mask a bundled advisory.

## Smoke test

Runs an end-to-end cross-language check (encryption + batch + tx) and verifies the encrypted attribute is stored as an
envelope in DynamoDB.

```bash
AWS_PROFILE=... bash examples/cdk-multilang/scripts/smoke.sh
```

For a no-AWS cross-language check, use the [`local/`](./local/README.md) variant instead:

```bash
bash examples/cdk-multilang/local/run-local.sh
```
