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

After deploy, the stack outputs three Function URLs. Use them to `GET`/`PUT` items:

- `GET ?pk=...&sk=...`
- `PUT` with JSON body: `{"pk":"...","sk":"...","value":"...","secret":"..."}`

Additional endpoints:

- `PUT /enc` (encryption demo; same payload as `PUT /`)
- `POST /batch` (batch write + batch get): `{"pk":"...","skPrefix":"...","count":3,"value":"...","secret":"..."}`
- `POST /tx` (transaction write): `{"pk":"...","skPrefix":"...","value":"...","secret":"..."}`

## Dependency audit note

As of the current dependency pin, `aws-cdk-lib@2.260.0` removes the previous bundled `fast-uri` high-severity audit finding. `npm audit` may still report a moderate `brace-expansion@5.0.5` finding bundled inside `aws-cdk-lib`; keep that visible rather than suppressing it, and refresh CDK again once AWS publishes a bundle with `brace-expansion >=5.0.6`.

SEC-2 records that advisory in `gov-infra/planning/theorydb-visible-npm-audit-findings.json`, not in the supply-chain
suppression allowlist. The verifier therefore stays green only while the finding remains explicitly printed in SEC-2
evidence as a visible upstream-bundled finding; if AWS CDK updates the bundled dependency, remove the visible policy entry
instead of adding an allowlist suppression.

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
