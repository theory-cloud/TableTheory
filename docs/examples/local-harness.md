---
title: Local examples harness
---

# Local examples harness

`make examples-local` runs every classified TableTheory example against local
backends and asserts each one's real outcome. Nothing reaches AWS: the harness
sets its own endpoint overrides and static dummy credentials, and its preflight
refuses to start when a DynamoDB client would resolve anywhere else.

```bash
make examples-local        # run everything that is classified
make examples-local-down   # stop the profile container
```

## What the harness owns

| Piece | Where it lives |
| --- | --- |
| DynamoDB Local profile | `scripts/local/docker-compose.yml` — `amazon/dynamodb-local:3.1.0`, loopback port 8123 |
| Classification manifest | `scripts/local/examples-local.json` |
| Harness | `scripts/local/run_examples_local.py` |
| Endpoint-resolution preflight | `scripts/local/cmd/examples-local-preflight` |
| CI job | `.github/workflows/examples-local.yml` |

The harness sets this environment for every child process and unsets anything
that could redirect it: every inherited `AWS_*` variable (`AWS_PROFILE`,
`AWS_SESSION_TOKEN`, an inherited `AWS_ENDPOINT_URL`, and so on) and the proxy
variables `HTTP_PROXY`, `HTTPS_PROXY`, `ALL_PROXY` and `NO_PROXY` in both cases,
so no child request can be routed through an inherited proxy.

```bash
AWS_REGION=us-east-1
AWS_DEFAULT_REGION=us-east-1
AWS_ACCESS_KEY_ID=dummy
AWS_SECRET_ACCESS_KEY=dummy
AWS_EC2_METADATA_DISABLED=true
AWS_CONFIG_FILE=/dev/null
AWS_SHARED_CREDENTIALS_FILE=/dev/null
AWS_ENDPOINT_URL=http://127.0.0.1:8123
AWS_ENDPOINT_URL_DYNAMODB=http://127.0.0.1:8123
DYNAMODB_ENDPOINT=http://127.0.0.1:8123
```

`AWS_ENDPOINT_URL_DYNAMODB` is the AWS-standard override the Go SDK resolves;
`DYNAMODB_ENDPOINT` is the convention the examples already read; `AWS_ENDPOINT_URL`
keeps the multi-tenant entrypoint, which reads it, on the same profile.

The harness also strips inherited `USER`, `ORG_ID` and
`FACETHEORY_CACHE_TABLE_NAME`. Rows that need a deterministic identity set
their own runtime values (the notes transcript uses `USER=harness-user`, the
contacts transcript uses `ORG_ID=org-harness`, and the FaceTheory cache demo
uses its default table name).

## Preflight

The run stops before any example if:

- DynamoDB Local is not listening and cannot be started from the profile;
- the endpoint accepts HTTP but does not speak the DynamoDB protocol
  (`internal/dynamodbprobe` issues a real `ListTables` call);
- a DynamoDB client built from the harness environment resolves to an empty
  endpoint (unresolved) or to any endpoint other than the profile's;
- the resolved credentials are not the static dummy keys;
- the HTTP example port or the cross-language container port is already in use.

The profile's in-memory database is recreated when the harness owns the
container, so every run starts from an empty backend. Examples that are run by
hand against the shared `:8000` convention do not get this isolation.

Before the first row, the harness installs any missing JavaScript toolchain from
the checked-in lockfiles: `ts/` (for `tsx` and the TypeScript runtime) and
`examples/cdk-multilang/` (the Node reader resolves `@aws-sdk/client-dynamodb`
from that example's own dependency tree).

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
stays visible while `lib/`, `test/` and generated output stay out. Run them alone
with `make examples-local-selftest`.

## Classes

Every manifest entry declares exactly one of:

| Value | Meaning |
| --- | --- |
| `L1` | Fully local: real application and framework code on local backends, assertions pass, no cloud gap. |
| `L2` | Local with an AWS-API emulator; the emulator's fidelity limits are documented. |
| `L3` | Every locally executable part runs for real, plus a named cloud gap that is never faked. |
| `I` | Infrastructure-only: synthesis plus construct assertions. |
| `pending: T2/T3/T4a/T4b` | Not wired yet. Reported as `PENDING-MILESTONE`, never counted as pass. |

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
    "dynamodbLocal": {
      "image": "amazon/dynamodb-local:3.1.0",
      "port": 8123
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
      "class": "L1",
      "description": "Executed inside run-local.sh."
    },
    "examples/encryption/main.go": {
      "pending": "T3",
      "description": "KMS envelope round-trip."
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
- The manifest's `port` and `image` are checked against the compose file during
  validation, and port 8000 is rejected.

## Current coverage

The harness wires:

| Example | Class | How it is verified |
| --- | --- | --- |
| `examples/local-quickstart/main.go` | `L1` | `go run`; five print/CRUD outcome checks |
| `examples/cdk-multilang/local/run-local.sh` | `L1` | Real script; construct drift check plus Go → TypeScript → Python round-trip |
| `examples/cdk-multilang/local/go/writer.go` | `L1` | `coveredBy` the `run-local.sh` row: the Go writer half of the round-trip |
| `examples/cdk-multilang/local/read.mts` | `L1` | `coveredBy` the `run-local.sh` row: the Node reader validates the written shape |
| `examples/cdk-multilang/local/read.py` | `L1` | `coveredBy` the `run-local.sh` row: the Python reader validates the written shape |
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

`optimization` and `testing` are intentionally library demos: the query planner
takes metadata/executor interfaces rather than a database, and `testing`
demonstrates mocks for unit tests. They run locally without DynamoDB, as scoped
by the local-execution need (Q3).

Every other entrypoint in the tree is listed in the manifest as `pending` for
T3, T4a or T4b. That includes the `examples/cdk-multilang` runtime files that
local execution does not reach yet: `bin/app.ts`, `lambdas/node/handler.ts`,
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
requests to `staging` and on manual dispatch. The job provides only a DynamoDB
Local service container on port 8123; it sets no AWS secrets and no endpoint
variables, because the harness owns that environment.

## Known gaps

- `CreateTable` is not idempotent for a model with a TTL field: when the table
  already exists with TTL enabled, AWS returns `ValidationException:
  TimeToLive is already enabled` from the TTL sync inside `CreateTable`. The
  multi-tenant example therefore provisions tables with `EnsureTable` (create if
  missing) and treats `ResourceInUseException` as typed success. A library-level
  fix is tracked separately.
- DynamoDB Local has no Streams, so stream-delivery paths are `L3` with the
  named gap "managed stream delivery" from T4a onward.
