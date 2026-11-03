# Local example harness

The checked-in local profile and harness behind `make examples-local`.

- `docker-compose.yml` — the local profile: DynamoDB Local plus digest-pinned
  moto (KMS + STS, and the reconcile row's S3 notifications + SQS), s3proxy
  (the default S3) and ElasticMQ (SQS). Every service publishes an ephemeral
  loopback port; the harness reads the assigned port back and gives each run a
  unique compose project name.
- `examples-local.json` — the classification manifest. Every example entrypoint
  in the tree must appear here as `L1`/`L2`/`L3`/`I` or as `pending: <milestone>`,
  every row must name a file that exists in the tree, and every `L3`/`I` row
  carries the named `gap` the summary prints.
- `run_examples_local.py` — discovery, profile startup, preflight, runners, the
  summary and `--cleanup`.
- `lambda_runtime.py` — builds each handler with its real toolchain, starts it
  in the official Lambda runtime image (RIE), joins it to the run's network with
  a closed DNS server, and invokes it over the RIE HTTP API. No handler is
  called in process.
- `test_examples_local.py` — mutation self-tests proving discovery, manifest
  validation and profile isolation fail closed, including a real two-instance
  concurrency test (`make examples-local-selftest`, run by
  `make examples-local` first).
- `cmd/examples-local-preflight` — proves the harness environment resolves
  DynamoDB, KMS, STS, S3 and SQS to the profile, sees only the static dummy
  credentials, and that each backend speaks its AWS protocol.
- `cmd/examples-local-lambda-preflight` — runs inside the Lambda runtime image
  and proves the same endpoints, credentials and backend protocols from the
  function container, and that a real AWS hostname does not resolve.
- `cmd/examples-local-payment-fixtures` — provisions the payment and Lambda
  tables, seeds the merchant and user rows, configures a real moto S3 bucket
  notification and puts the reconciliation CSV, and reads stored state back for
  assertions.
- `cmd/examples-local-stream-event` — performs a real local write (and, for the
  archive row, a real delete) and reads the resulting record from DynamoDB Local
  Streams through `DescribeStream`, `GetShardIterator` and `GetRecords`. It
  never derives a record from the stored item.
- `internal/streams` — the shared stream reader, Lambda event-shape conversion
  and the documented local deviations (`eventSourceARN` is filled from the real
  stream ARN, and the TTL service identity is added to a harness delete because
  DynamoDB Local does not run the TTL sweeper).
- `cmd/examples-local-kms-key` — creates the per-run envelope key and proves the
  local KMS wraps and unwraps a data key with the framework's context.

Usage: `make examples-local`, or `make examples-local-down` to remove leftover
profile containers and networks from any run.

The full description, the emulator decision table, class definitions and
manifest rules live in
[`docs/examples/local-harness.md`](../../docs/examples/local-harness.md).
