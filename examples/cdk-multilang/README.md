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

The current pin is `aws-cdk-lib@2.278.0`, which publishes bundled `minimatch@11.0.9` — the fixed version for
GHSA-g3hv-75wg-m249 — and bundled `brace-expansion@5.0.13`, the fixed version for the three brace-expansion advisories
earlier `aws-cdk-lib` releases shipped. npm overrides cannot replace these copies because AWS publishes them inside
`aws-cdk-lib`, so the only fix was the `aws-cdk-lib` upgrade itself.

The two prior visible, expiring findings for this example (the bundled `minimatch` leaf and npm's derived `aws-cdk-lib`
parent entry) are retired. `gov-infra/planning/theorydb-visible-npm-audit-findings.json` declares no findings for
`examples/cdk-multilang`, and SEC-2 no longer needs `NPM_AUDIT_ACCEPT_VISIBLE_FINDINGS=true` to accept them.

Non-bundled minimatch copies are fixed by upgrade, never by exception: the `ts` eslint toolchain is pinned to minimatch
`11.0.1` through the `overrides` block in `ts/package.json`.

`scripts/test-npm-audit-policy.sh` is the guard that keeps this state honest. It asserts the graph directly and fails
closed in every direction:

- `aws-cdk-lib` must be `>= 2.278.0`, with a resolved tarball URL and a `sha512-` integrity hash.
- The bundled `node_modules/aws-cdk-lib/node_modules/minimatch` entry must exist, be marked `inBundle: true`, and resolve
  to `>= 11.0.1`; the bundled `brace-expansion` must exist, be `inBundle: true`, and resolve to `>= 5.0.12`.
- No `minimatch` below `11.0.1` may exist anywhere in the audited lockfiles (the CDK example, `ts`, and the
  contract-tests runners).
- The `ts` override and lockfile must pin minimatch exactly `11.0.1`.
- No residual visible policy entry may reference the retired GHSA-g3hv-75wg-m249 pair for `examples/cdk-multilang`.
- Malformed lockfiles, manifests, and policies all fail closed.

Synthetic negative controls (an old `10.2.6` bundled or root minimatch, a missing bundled entry, a missing CDK entry or
integrity, a bundle that is not `inBundle`, an old `ts` override or lockfile, a vulnerable contract-tests runner
lockfile, a residual visible-policy entry, and malformed inputs) all fail closed, so the guard is green only while the
reviewed fixed bundle and the empty policy agree. When AWS publishes a future bundle, take the version bump and update
the guard's floor — do not re-add a visible entry or an allowlist suppression to mask a non-bundled advisory.

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

The examples harness also drives the deployed Go, Node and Python handlers and
the archive handler locally, and runs
[`scripts/smoke-local.sh`](./scripts/smoke-local.sh) — the same journeys as
`scripts/smoke.sh` against the runtime interface:

```bash
make examples-local
```

`smoke-local.sh` needs `CDK_GO_URL`, `CDK_NODE_URL`, `CDK_PY_URL` and
`TABLE_NAME`, which the harness supplies. Its named gap is deployed-stack smoke:
Function URL IAM authentication, the deployed tables and the deployed stream
event source are not exercised without a real stack.
