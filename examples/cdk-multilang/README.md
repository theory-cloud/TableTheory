# TableTheory CDK Multi-language Demo

Deploys **one DynamoDB table** and **three Lambdas** (Go, Node.js 24, Python 3.14) that read/write the same item
shape. This is the deployable “proof” that the multi-language TableTheory stack can share a single table without drift.

This demo also exercises:
- **Encryption** (KMS envelope, cross-language decrypt)
- **Batching** (BatchWrite + BatchGet)
- **Transactions** (TransactWrite)

## Commands

From the repo root:

- Install deps: `npm --prefix examples/cdk-multilang ci`
- Synthesize: `npm --prefix examples/cdk-multilang run synth`
- Deploy (writes `cdk.outputs.json`): `AWS_PROFILE=... npm --prefix examples/cdk-multilang run deploy -- --profile $AWS_PROFILE --outputs-file cdk.outputs.json`

After deploy, the stack outputs three Function URLs. Use them to `GET`/`PUT` items:

- `GET ?pk=...&sk=...`
- `PUT` with JSON body: `{"pk":"...","sk":"...","value":"...","secret":"..."}`

Additional endpoints:
- `PUT /enc` (encryption demo; same payload as `PUT /`)
- `POST /batch` (batch write + batch get): `{"pk":"...","skPrefix":"...","count":3,"value":"...","secret":"..."}`
- `POST /tx` (transaction write): `{"pk":"...","skPrefix":"...","value":"...","secret":"..."}`

## Smoke test

Runs an end-to-end cross-language check (encryption + batch + tx) and verifies the encrypted attribute is stored as an
envelope in DynamoDB.

```bash
AWS_PROFILE=... bash examples/cdk-multilang/scripts/smoke.sh
```
