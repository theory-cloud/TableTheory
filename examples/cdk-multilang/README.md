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

The current pin is `aws-cdk-lib@2.271.0`. `npm audit` reports three brace-expansion advisories (one medium, two high)
against the copy `aws-cdk-lib` publishes bundled, `node_modules/aws-cdk-lib/node_modules/brace-expansion@5.0.9`. Because
AWS publishes that dependency inside `aws-cdk-lib`, npm overrides cannot replace it and the finding is not fixable from
this repo.

SEC-2 therefore records all three advisories in `gov-infra/planning/theorydb-visible-npm-audit-findings.json` (reviewed
2026-10-03, self-expiring 2026-11-02), never in the supply-chain suppression allowlist. The findings stay explicitly
printed in SEC-2 evidence and are never described as allowlisted.

`scripts/test-npm-audit-policy.sh` is the guard that keeps that state honest, and it fails in both directions:

- While `aws-cdk-lib` bundles a vulnerable brace-expansion, the bundled version must still be exactly `5.0.9` and all
  three visible entries must still be present. Removing the entries early, or moving to a different bundled version,
  fails the guard and forces a re-review.
- Once `aws-cdk-lib` bundles a fixed brace-expansion (`>= 5.0.12`), the visible entries must be removed rather than left
  in place; keeping the retired exception fails the guard.

The verifier is therefore green only while the exception is still required, and stops being green once it goes stale.
When AWS publishes a fixed bundle, take the version bump and delete the visible policy entries — do not add an allowlist
suppression.

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
