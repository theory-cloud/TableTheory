---
title: Local examples harness
---

# Local examples harness

`make examples-local` runs every classified TableTheory example against local
backends and asserts each one's real outcome. Nothing reaches AWS: the harness
sets its own endpoint overrides and static dummy credentials, and its preflight
refuses to start unless DynamoDB, KMS, STS, S3 and SQS all resolve to loopback
endpoints it owns.

```bash
make examples-local        # run everything that is classified
make examples-local-down   # remove leftover containers/networks from any run
```

## What the harness owns

| Piece | Where it lives |
| --- | --- |
| Local profile | `scripts/local/docker-compose.yml` — digest-pinned DynamoDB Local, moto (KMS + STS), s3proxy and ElasticMQ |
| Classification manifest | `scripts/local/examples-local.json` |
| Harness | `scripts/local/run_examples_local.py` |
| Endpoint-resolution preflight | `scripts/local/cmd/examples-local-preflight` |
| Lambda runtime driver | `scripts/local/lambda_runtime.py` |
| In-container Lambda preflight | `scripts/local/cmd/examples-local-lambda-preflight` |
| Payment and Lambda fixtures | `scripts/local/cmd/examples-local-payment-fixtures` |
| Generated stream record | `scripts/local/cmd/examples-local-stream-event` |
| KMS key provisioning and round-trip | `scripts/local/cmd/examples-local-kms-key` |
| CI job | `.github/workflows/examples-local.yml` |

The harness sets this environment for every child process and unsets anything
that could redirect it: every inherited `AWS_*` variable (`AWS_PROFILE`,
`AWS_SESSION_TOKEN`, an inherited `AWS_ENDPOINT_URL_KMS`, and so on) and the
proxy variables `HTTP_PROXY`, `HTTPS_PROXY`, `ALL_PROXY` and `NO_PROXY` in both
cases, so no child request can be routed through an inherited proxy.

```bash
AWS_REGION=us-east-1
AWS_DEFAULT_REGION=us-east-1
AWS_ACCESS_KEY_ID=dummy
AWS_SECRET_ACCESS_KEY=dummy
AWS_EC2_METADATA_DISABLED=true
AWS_CONFIG_FILE=/dev/null
AWS_SHARED_CREDENTIALS_FILE=/dev/null
AWS_ENDPOINT_URL=http://127.0.0.1:<dynamodb-port>
AWS_ENDPOINT_URL_DYNAMODB=http://127.0.0.1:<dynamodb-port>
DYNAMODB_ENDPOINT=http://127.0.0.1:<dynamodb-port>
AWS_ENDPOINT_URL_KMS=http://127.0.0.1:<moto-port>
KMS_ENDPOINT=http://127.0.0.1:<moto-port>
AWS_ENDPOINT_URL_STS=http://127.0.0.1:<moto-port>
STS_ENDPOINT=http://127.0.0.1:<moto-port>
AWS_ENDPOINT_URL_S3=http://127.0.0.1:<s3proxy-port>
S3_ENDPOINT=http://127.0.0.1:<s3proxy-port>
AWS_ENDPOINT_URL_SQS=http://127.0.0.1:<elasticmq-port>
SQS_ENDPOINT=http://127.0.0.1:<elasticmq-port>
```

`AWS_ENDPOINT_URL_<SERVICE>` is the AWS-standard override the SDKs resolve;
`DYNAMODB_ENDPOINT`, `KMS_ENDPOINT`, `S3_ENDPOINT` and the other aliases are the
conventions the examples already read. `AWS_ENDPOINT_URL` keeps the multi-tenant
entrypoint, which reads it, on the DynamoDB profile. The ports are not fixed:
the harness reads them back from Docker after the stack starts (see
"Isolation" below).

The harness also strips inherited `USER`, `ORG_ID`, `FACETHEORY_CACHE_TABLE_NAME`
and `KMS_KEY_ARN`. Rows that need a deterministic identity set their own runtime
values (the notes transcript uses `USER=harness-user`, the contacts transcript
uses `ORG_ID=org-harness`, and the FaceTheory cache demo uses its default table
name). `KMS_KEY_ARN` is provisioned fresh per run.

## Preflight

The run stops before any example if:

- Docker is unavailable (the profile is a real compose stack, not an optional
  convenience);
- DynamoDB Local, moto, s3proxy or ElasticMQ does not answer;
- any of DynamoDB, KMS, STS, S3 or SQS resolves to an empty endpoint, to a
  non-loopback host, or to anything other than the profile endpoint the harness
  started;
- the resolved credentials are not the static dummy keys;
- the HTTP example port or the cross-language container port is already in use;
- a service does not speak its AWS protocol: the preflight issues a real
  `ListTables` (DynamoDB), `ListKeys` (KMS), `GetCallerIdentity` (STS),
  `ListBuckets` (S3, path-style) and `ListQueues` (SQS);
- the Lambda function container resolves an SDK endpoint anywhere but the
  profile, sees credentials other than the dummy static keys, fails a backend
  protocol probe, or resolves a real AWS hostname;
- the local KMS cannot wrap and unwrap a data key with the framework's
  `EncryptionContext`.

Before the first row, the harness installs any missing JavaScript toolchain from
the checked-in lockfiles: `ts/` (for `tsx` and the TypeScript runtime) and
`examples/cdk-multilang/` (the Node reader resolves `@aws-sdk/client-dynamodb`
from that example's own dependency tree). It then creates one KMS key with
`examples-local-kms-key`, which also performs a real `GenerateDataKey`/`Decrypt`
round-trip, and exports the key ARN to the rows that need it.

## Isolation

Every run gets a private Docker identity:

- the compose project is `tabletheory-examples-local-<10 hex>`, computed from
  the resolved checkout path and a run id (random per run; `--run-id` makes it
  reproducible for debugging);
- every service publishes an ephemeral loopback port (`127.0.0.1::<container
  port>`); the harness reads the assigned host port back with
  `docker compose port` and exports it to the examples;
- the harness removes its own containers and network in a `finally` block, and
  `make examples-local-down` (`--cleanup`) removes leftovers from any checkout.

Two harness instances on one host therefore never share a container, a network
or a port. That matters in this workspace: an implementer and a reviewer run
the harness from different checkouts at the same time. The regression test
`ConcurrencyIsolationTests` starts two real profiles concurrently and asserts
that the second one neither recreates nor removes the first one's container,
that the endpoints differ, and that the first database still answers after the
second stack is torn down.

`examples/cdk-multilang/local/run-local.sh` also starts its own throwaway
DynamoDB Local. Without `CDK_LOCAL_PORT` it now publishes an ephemeral port and
discovers it, so a hand run cannot collide either.


## Emulator decision (T3)

The profile uses one permissively licensed, maintained emulator per service.
Each candidate below was trialed with the Go SDK v2 at the versions this module
pins, against a real container, before it was chosen.

| Service | Choice | Licence | Image pin | What the trial proved | Fidelity limits | Class |
| --- | --- | --- | --- | --- | --- | --- |
| KMS | moto 5.2.5 | Apache-2.0 | `motoserver/moto@sha256:59a1965d845249b04def37a0e2565f9cc6c44543cb89b68e90f28f344b3b0d1a` | `CreateKey`, `GenerateDataKey` (AES-256, `EncryptionContext`) and `Decrypt` round-trip; a mismatched context is refused; `examples/encryption` ran end to end with real envelope crypto | No HSM, no key-policy/IAM/grant enforcement, no cross-account semantics; key material is in process memory. `GenerateDataKey`/`Decrypt`/`CreateKey` and context binding are real | `L2` |
| STS | moto 5.2.5 | Apache-2.0 | same image as KMS | `AssumeRole` returned credentials with an expiration | Synthetic credentials; no trust-policy or IAM evaluation, any role ARN is accepted, session policies/tags are not enforced | `L2` |
| S3 | s3proxy (`gaul/s3proxy`, commit `c7d5140`) | Apache-2.0 | `andrewgaul/s3proxy@sha256:1f8915c2b4a8af22bf1fc191500f242974a458c457391b5788f9107544273dc3` | SigV4 with the harness's dummy identity; `CreateBucket`, `PutObject`, `GetObject`, `ListObjectsV2` | No IAM/bucket policies, no event notifications, no Object Lock; versioning, multipart and checksum enforcement are partial. S3 → EventBridge delivery is absent, so S3 event paths remain `L3` "managed S3 event delivery" | `L2` |
| SQS | ElasticMQ 1.7.1 | Apache-2.0 | `softwaremill/elasticmq-native@sha256:e4580ab9ad1bd5cd37b4ba04911bc5ccc8cd2d9ab4de56ece65acee71c24e05c` | `CreateQueue`, `SendMessage`, `ReceiveMessage`, `DeleteMessage` | In-memory queues; no IAM policies, cross-account access or SNS/EventBridge subscriptions. Polling delivery is real, so a later SQS polling example can be `L2`; managed event-source-mapping delivery remains `L3` | `L2` |

Evaluated and rejected:

- **LocalStack** — excluded. The `localstack/localstack` repository is archived
  (2026-03-23), and the current documentation makes a `LOCALSTACK_AUTH_TOKEN`
  mandatory to start the container: the token is a per-user or workspace
  credential from the LocalStack web app, and startup fails without it. That is
  an operator account credential and a network activation dependency, which the
  no-real-AWS/no-operator-configuration rules forbid. It also cannot start
  unauthenticated. Source: <https://docs.localstack.cloud/getting-started/auth-token/>.
- **local-kms** 3.11.7 (MIT) — passed the same `CreateKey`/`GenerateDataKey`/
  `Decrypt` trial and the mismatched-context refusal, but its stable image is
  from 2023-10 while the current `latest` tag is `4.0.0-alpha.1` (2026-02). A
  maintained KMS in moto was preferred over an alpha line.
- **moto S3** — passed the same put/get/list trial, but s3proxy was chosen for a
  dedicated, SigV4-enforcing S3 service with a filesystem backend.

DynamoDB Local predates this decision (T1) and is now digest-pinned
(`amazon/dynamodb-local@sha256:7ef4a2c45b58c2901e70a4f28e0953a422c2c631baaaf5e2c15e0805740c7752`,
tag `3.1.0`).

Every image in the profile is pinned by digest, and the manifest validation
fails if a tag-only image, a fixed host port, a fixed container name or a fixed
compose project name is introduced. The tags that produced the current digests
are `amazon/dynamodb-local:3.1.0`, `motoserver/moto:5.2.5`,
`andrewgaul/s3proxy:master` and `softwaremill/elasticmq-native:1.7.1`.

## Lambda handlers (T4a)

Every handler is built with its real toolchain and started in the official AWS
Lambda runtime image, which embeds the Runtime Interface Emulator (RIE). The
harness invokes `POST /2015-03-31/functions/function/invocations` with a real
event payload; no handler is called in process.

| Runtime | Image | Handler build |
| --- | --- | --- |
| Go | `public.ecr.aws/lambda/provided:al2023` | `GOOS=linux GOARCH=<docker server arch> CGO_ENABLED=0 go build`, mounted as `/var/task/bootstrap` |
| Node | `public.ecr.aws/lambda/nodejs:24` | esbuild bundle of the `.ts` entrypoint, mounted as `/var/task/handler.js` |
| Python | `public.ecr.aws/lambda/python:3.14` | the checked-in handler plus a verbatim copy of `py/src/tabletheory_py`, mounted at `/var/task` |

The function container joins the run's private compose network, gets a hosts
entry for every profile service, and is started with a closed DNS server
(`127.0.0.2`). Its environment carries the static dummy credentials and the
profile's endpoint overrides. Before any example runs, the harness invokes
`examples-local-lambda-preflight` in the same runtime image; it asserts that
DynamoDB, KMS, STS, S3 and SQS all resolve to the profile, that each backend
answers its real protocol, that only the dummy credentials resolve, and that
`dynamodb.us-east-1.amazonaws.com` does not resolve. A failure stops the run.

Fixture tooling (`scripts/local/cmd/examples-local-payment-fixtures`) provisions
the tables the handlers use and seeds the merchant and user rows through the real
models. The stream event tool performs one real write and reads the stored item
back; the generated record is then fed to the Python handler.

| Row | Class | How it is verified | Named gap |
| --- | --- | --- | --- |
| `examples/lambda/main.go` | `L2` | base-account create/get plus partner `AssumeRole` create/get through RIE; unknown-action and unknown-partner refusals | moto STS has no authorization fidelity: no trust policy, external-ID or IAM enforcement |
| `examples/payment/lambda/process` | `L2` | real API Gateway proxy event; merchant policy, idempotency and the atomic payment/transaction/counter write; `403`/`401` refusals; the webhook HTTPS/public-IP rule refuses `https://127.0.0.1` and stores no webhook record | KMS is emulated (no HSM, key policies or grants); the envelope decrypt is real |
| `examples/payment/lambda/query` | `L1` | real API Gateway proxy events for list, detail, summary and export, plus an unauthenticated refusal | none |
| `examples/payment/lambda/reconcile` | `L3` | generated S3 event plus a real `GetObject` on s3proxy; two rows reconcile; settlement and audit rows are stored | managed S3 event delivery: s3proxy has no bucket notifications |
| `ts/examples/lambda/handler.ts` | `L1` | Node 24 handler reads a seeded item from DynamoDB Local | none |
| `py/examples/lambda_stream_handler.py` | `L3` | generated DynamoDB stream record from a real local write and read-back; Python 3.14 handler logs the decoded note | managed stream delivery: DynamoDB Local has no Streams |

The reconcile handler uses path-style S3 addressing when `AWS_ENDPOINT_URL_S3`
or `S3_ENDPOINT` is set; production AWS keeps virtual-host addressing. The
payment process handler reads `KMS_KEY_ARN` so it can decrypt the merchant's
encrypted fields, which the harness provisions in the local KMS.

## Discovery

Entrypoints are discovered from the tree, not from a hand-maintained list:

- `examples/**/*.go` that declares `func main` is a Go entrypoint (`func main`
  is the marker; path conventions do not apply to Go);
- a `.sh` file under `examples/`, `ts/examples/` or `py/examples/` is a shell
  runner;
- files under `examples/`, `ts/examples/` and `py/examples/` with a runtime
  suffix (`.ts`, `.mts`, `.cts`, `.js`, `.mjs`, `.cjs`, `.py`) are runtime
  entrypoints, so a `.ts` or `.py` file under `examples/` cannot hide from
  classification;
- any other executable file under those trees is a runner.

These are not entrypoints, and discovery skips them by documented convention:

| Rule | Covers |
| --- | --- |
| `node_modules`, `cdk.out`, `dist`, `vendor`, `__pycache__`, `.venv` | dependency trees and build output |
| `lib/` | support modules inside an example, such as the CDK constructs in `examples/cdk-multilang/lib` |
| `test/`, `tests/`, `*.test.*`, `*.spec.*`, `*_test.*` | tests and test fixtures |
| first lines contain `Code generated ... DO NOT EDIT` | generated output such as `examples/cdk-multilang/local/generated-table.ts` |

`bin/` is deliberately not excluded: `examples/cdk-multilang/bin/app.ts` is the
CDK app entrypoint and is classified like any other runtime file.

The run fails when the tree contains an entrypoint that the manifest does not
classify. It also fails when a `pending` or classified row names a file that does
not exist in the tree or that discovery does not treat as an entrypoint; only a
`coveredBy` row may point at a non-entrypoint file, because it documents a helper
executed inside another row. Unknown or reused runner ids fail as well.

## Self-tests

`make examples-local` runs `make examples-local-selftest` first. The self-tests
build throwaway repository trees and assert that validation fails when:

- a rogue `.ts` or `.py` file under `examples/` is unclassified, or a rogue
  `.sh` runner under `ts/examples/` is unclassified;
- a `pending` row names a file that does not exist, or one discovery does not
  treat as an entrypoint.

They also pin the documented exclusions against the real tree, so `bin/app.ts`
stays visible while `lib/`, `test/` and generated output stay out.

The profile itself is tested the same way: a tag-only image, a fixed host port,
a fixed `container_name`, a fixed compose project name, a missing logical
endpoint, or an endpoint that omits its `AWS_ENDPOINT_URL_<SERVICE>` override
all fail validation. `ConcurrencyIsolationTests` starts two real profiles at
once and asserts that the second run neither recreates nor removes the first
run's container, that the endpoints differ, and that the first database still
answers after the second stack is torn down. That test needs Docker and fails
with a clear message when Docker is unavailable, because a skipped concurrency
proof would not prove isolation. Run the suite alone with
`make examples-local-selftest`.

## Classes

Every manifest entry declares exactly one of:

| Value | Meaning |
| --- | --- |
| `L1` | Fully local: real application and framework code on local backends, assertions pass, no cloud gap. |
| `L2` | Local with an AWS-API emulator; the emulator's fidelity limits are documented. |
| `L3` | Every locally executable part runs for real, plus a named cloud gap that is never faked. |
| `I` | Infrastructure-only: synthesis plus construct assertions. |
| `pending: T4b` | Not wired yet. Reported as `PENDING-MILESTONE`, never counted as pass. |

A skip is a failure. Go tests are run through `go test -json`, and any `skip`
event fails the row; example runners treat a non-zero exit, a timeout, or a
missing expected output as a failure.

## Manifest format

```json
{
  "version": 1,
  "profile": {
    "name": "tabletheory-examples-local",
    "composeFile": "scripts/local/docker-compose.yml",
    "containers": {
      "dynamodb-local": {
        "image": "amazon/dynamodb-local@sha256:...",
        "containerPort": 8000
      },
      "moto": {
        "image": "motoserver/moto@sha256:...",
        "containerPort": 5000
      }
    },
    "endpoints": {
      "dynamodb": {
        "container": "dynamodb-local",
        "env": ["AWS_ENDPOINT_URL", "AWS_ENDPOINT_URL_DYNAMODB", "DYNAMODB_ENDPOINT"]
      },
      "kms": {
        "container": "moto",
        "env": ["AWS_ENDPOINT_URL_KMS", "KMS_ENDPOINT"]
      }
    }
  },
  "entrypoints": {
    "examples/local-quickstart/main.go": {
      "run": "go-local-quickstart",
      "class": "L1",
      "description": "Go CRUD quickstart on DynamoDB Local."
    },
    "examples/cdk-multilang/local/go/writer.go": {
      "coveredBy": "examples/cdk-multilang/local/run-local.sh",
      "class": "L2",
      "description": "Executed inside run-local.sh."
    },
    "examples/encryption/main.go": {
      "run": "go-encryption",
      "class": "L2",
      "description": "KMS envelope round-trip; gap: the KMS is emulated."
    }
  },
  "suites": {
    "examples/multi-tenant": {
      "run": "multi-tenant-module-tests",
      "class": "L1",
      "description": "The example's separate Go module."
    }
  }
}
```

- `containers` declares every compose service with its digest-pinned image and
  container port; `endpoints` maps each logical service (`dynamodb`, `kms`,
  `sts`, `s3`, `sqs`; the example above abbreviates the last three) to a
  container and the environment variables that must point at it.
- Validation checks that every container image is digest-pinned, that the
  compose file declares the same image and publishes an ephemeral loopback port
  (`127.0.0.1::<containerPort>`), that no `container_name` or top-level compose
  `name` fixes an identity, that all five logical services exist, and that each
  one exposes its `AWS_ENDPOINT_URL_<SERVICE>` override.
- `run` names a runner implemented in `scripts/local/run_examples_local.py`.
  Each runner is used by exactly one entry.
- `coveredBy` points at another executed entry; the covered row inherits its
  result and is not run a second time. It is how a helper entrypoint
  (`local/go/writer.go`, `local/read.mts`, `local/read.py` — the halves of the
  cross-language check) stays visible without duplicating work.
- `pending` names a milestone, not a runner. A pending row still has to name a
  file that exists in the tree and that discovery treats as an entrypoint; it is
  printed on every run and never counted as pass.
- `suites` cover separate Go modules. A root `go test ./...` never reaches
  `examples/multi-tenant`, so its module test suite is a first-class row.

## Current coverage

The harness wires:

| Example | Class | How it is verified |
| --- | --- | --- |
| `examples/local-quickstart/main.go` | `L1` | `go run`; five print/CRUD outcome checks |
| `examples/cdk-multilang/local/run-local.sh` | `L2` | Real script; construct drift check plus a Go → TypeScript → Python round-trip whose `secret` field is encrypted on write and decrypted through the local KMS by all three runtimes. Gap: emulated KMS (no HSM, key policies or grants) |
| `examples/cdk-multilang/local/go/writer.go` | `L2` | `coveredBy` the `run-local.sh` row: writes the encrypted item and asserts it decrypts on read |
| `examples/cdk-multilang/local/read.mts` | `L2` | `coveredBy` the `run-local.sh` row: the Node reader validates the shape and decrypts the secret |
| `examples/cdk-multilang/local/read.py` | `L2` | `coveredBy` the `run-local.sh` row: the Python reader validates the shape and decrypts the secret |
| `examples/encryption/main.go` | `L2` | `go run` against DynamoDB Local and moto KMS: the plaintext field round-trips on read, and a raw `GetItem` proves the stored attribute is a v2 envelope (`edk`/`nonce`/`ct`) whose ciphertext does not contain the plaintext. Gap: emulated KMS (no HSM, key policies or grants) |
| `examples/multi-tenant/cmd/local/main.go` | `L1` | Real HTTP server; health, 401 refusals, signed-JWT signup, authenticated reads, cross-tenant 404 |
| `examples/multi-tenant/cmd/lambda/main.go` | `L1` | Route-matrix tests through API Gateway events and a loopback HTTP server |
| `examples/multi-tenant/cmd/token/main.go` | `L1` | Minted JWT claims plus refusal of incomplete arguments |
| `examples/multi-tenant` (module) | `L1` | `go test -json ./...` with skip detection; handler unit tests use the in-process fake, while the route-matrix and HTTP rows above cover the real-DynamoDB paths |
| `ts/examples/local.ts` | `L1` | `tsx` run; create/query/update/delete output checks |
| `py/examples/local_crud.py` | `L1` | `uv run`; put/get/`begins_with` output checks |
| `examples/basic/todo/main.go` | `L1` | The real CLI driven through `transcript.txt` on stdin; create/list/complete/delete output checks and a post-delete assertion |
| `examples/basic/notes/main.go` | `L1` | The real CLI driven through its transcript; CRUD plus the `gsi-user` and `gsi-category` reads, scoped so the personal note never appears in the work listing |
| `examples/basic/contacts/main.go` | `L1` | The real CLI driven through its transcript; CRUD plus `gsi-email`, `gsi-phone` and `gsi-name`, with a post-delete assertion |
| `examples/initialization/main.go` | `L3` | `-mode local` creates the table and performs a real write/read. The `aws`, `profile`, `minimal` and `debug` modes resolve real AWS defaults; they are the named cloud gap and never run here |
| `examples/typed-go/main.go` | `L1` | Creates the table, runs `typedFlow`, then reads back the updated name and asserts the `begins_with` result |
| `examples/drivers/main.go` | `L1` | Drives the loose `examples/*.go` snippets with seeded rows; asserts conditional-helper, transaction-builder, batch-get, list-operation, embedded-struct and FaceTheory TTL outcomes |
| `examples/feature_spotlight.go` | `L1` | `coveredBy` the driver row: insert-only create, optimistic update and guarded delete |
| `examples/list_operations_example.go` | `L1` | `coveredBy` the driver row: list append/prepend/element operations and a set DELETE |
| `examples/embedded_structs_example.go` | `L1` | `coveredBy` the driver row: single-table writes, `gsi1`/`gsi2` reads and the email/GSI2 update |
| `examples/facetheory_ttl_cache_example.go` | `L1` | `coveredBy` the driver row: real metadata write plus the freshness and TTL boundary helpers |
| `examples/release-state/main.go` | `L1` | Real `TransactWrite`: actual-state update, event append and conditional outbox create; read-back assertions plus refusal of a stale version |
| `ts/examples/release-state.ts` | `L1` | The same transition, read-backs and stale-version refusal through the TypeScript runtime; identical parity line |
| `py/examples/release_state.py` | `L1` | The same transition, read-backs and stale-version refusal through the Python runtime; identical parity line |
| `py/examples/gsi_query.py` | `L1` | Creates the table and index, writes two users and asserts the real GSI query returns only the matching email |
| `examples/optimization/main.go` | `L1` | Library demo: the query planner runs locally without DynamoDB; its real plan output is asserted |
| `examples/testing/user_service_test.go` | `L1` | `coveredBy` the optimization row: the mock-based unit-test suite runs with skip detection. These fakes are test doubles, never example verification |

The T2 rows above are all `L1` except `examples/initialization`, which is `L3`
because its AWS/profile/minimal/debug modes need a real AWS account and
credentials. The harness runs only the local mode, which performs a real
DynamoDB write and read.

The T3 rows (`examples/encryption` and the cdk-multilang local variant) are
`L2`: the envelope cryptography, KMS calls and cross-language decryption are
real, and the KMS emulator's limits are the named gap. The S3 and SQS emulators
are part of the profile and preflighted. The T4a payment reconcile row consumes
S3; AppTheory will consume SQS with polling delivery.

`optimization` and `testing` are intentionally library demos: the query planner
takes metadata/executor interfaces rather than a database, and `testing`
demonstrates mocks for unit tests. They run locally without DynamoDB, as scoped
by the local-execution need (Q3).

Every other entrypoint in the tree is listed in the manifest as `pending` for
T4b. That includes the `examples/cdk-multilang` runtime files that local
execution does not reach yet: `bin/app.ts`, `lambdas/node/handler.ts`,
`lambdas/archive/handler.ts` and `lambdas/python/handler.py` are T4b rows,
alongside the deployed `lambdas/go` handler and `scripts/smoke.sh`. Pending rows
are printed on every run and are never counted as pass.

## Adding an example

1. Add a runner function to `scripts/local/run_examples_local.py` that executes
   the real example and asserts its real outcome. Do not assert on mocks.
2. Register the runner in `RUNNERS`.
3. Add the entrypoint to `scripts/local/examples-local.json` with `class` and
   `run`. If the example belongs to a later milestone, use `pending` instead.
4. Run `make examples-local` twice in a row. The second run must pass against a
   fresh database without manual cleanup.

## CI

`.github/workflows/examples-local.yml` runs `make examples-local` on pull
requests to `staging` and on manual dispatch. The job has no service containers
and no fixed ports: the harness starts the whole profile itself with a project
name and ephemeral ports unique to the run, and removes it afterwards. It sets
no AWS secrets and no endpoint variables, because the harness owns that
environment.

## Known gaps

- `CreateTable` is not idempotent for a model with a TTL field: when the table
  already exists with TTL enabled, AWS returns `ValidationException:
  TimeToLive is already enabled` from the TTL sync inside `CreateTable`. The
  multi-tenant example therefore provisions tables with `EnsureTable` (create if
  missing) and treats `ResourceInUseException` as typed success. A library-level
  fix is tracked separately.
- DynamoDB Local has no Streams, so stream-delivery paths are `L3` with the
  named gap "managed stream delivery" in the T4a Python row and later rows.
- The S3, SQS and STS emulators do not deliver managed events: S3 event
  notifications, SQS event-source mappings and EventBridge delivery remain `L3`
  with the named "managed <service> delivery" gap. Polling an ElasticMQ queue
  is real delivery and can be `L2`.
- moto's STS accepts any role ARN and does not evaluate trust or IAM policies;
  the T4a partner-AssumeRole row treats the authorization decision as a named
  gap even though the `AssumeRole` call itself is real.
- The emulator images are digest-pinned but the s3proxy image publishes no
  semantic tag (the current pin came from `master`/commit `c7d5140`); refreshing
  it means re-running the S3 trial and updating both the compose file and this
  document.
