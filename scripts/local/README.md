# Local example harness

The checked-in local profile and harness behind `make examples-local`.

- `docker-compose.yml` — the local profile: DynamoDB Local plus digest-pinned
  moto (KMS + STS), s3proxy (S3) and ElasticMQ (SQS) services. Every service
  publishes an ephemeral loopback port; the harness reads the assigned port back
  and gives each run a unique compose project name.
- `examples-local.json` — the classification manifest. Every example entrypoint
  in the tree must appear here as `L1`/`L2`/`L3`/`I` or as `pending: <milestone>`,
  and every row must name a file that exists in the tree.
- `run_examples_local.py` — discovery, profile startup, preflight, runners, the
  summary and `--cleanup`.
- `test_examples_local.py` — mutation self-tests proving discovery, manifest
  validation and profile isolation fail closed, including a real two-instance
  concurrency test (`make examples-local-selftest`, run by
  `make examples-local` first).
- `cmd/examples-local-preflight` — proves the harness environment resolves
  DynamoDB, KMS, STS, S3 and SQS to the profile, sees only the static dummy
  credentials, and that each backend speaks its AWS protocol.
- `cmd/examples-local-kms-key` — creates the per-run envelope key and proves the
  local KMS wraps and unwraps a data key with the framework's context.

Usage: `make examples-local`, or `make examples-local-down` to remove leftover
profile containers and networks from any run.

The full description, the emulator decision table, class definitions and
manifest rules live in
[`docs/examples/local-harness.md`](../../docs/examples/local-harness.md).
