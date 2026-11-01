# Local example harness

The checked-in local profile and harness behind `make examples-local`.

- `docker-compose.yml` — the DynamoDB Local 3.1.0 profile on loopback port 8123.
- `examples-local.json` — the classification manifest. Every example entrypoint
  in the tree must appear here as `L1`/`L2`/`L3`/`I` or as `pending: <milestone>`,
  and every row must name a file that exists in the tree.
- `run_examples_local.py` — discovery, preflight, runners and the summary.
- `test_examples_local.py` — mutation self-tests proving discovery and manifest
  validation fail closed (`make examples-local-selftest`, run by
  `make examples-local` first).
- `cmd/examples-local-preflight` — proves the harness environment resolves
  DynamoDB to the profile and sees only the static dummy credentials.

Usage: `make examples-local`, or `make examples-local-down` to stop the profile
container.

The full description, class definitions and manifest rules live in
[`docs/examples/local-harness.md`](../../docs/examples/local-harness.md).
