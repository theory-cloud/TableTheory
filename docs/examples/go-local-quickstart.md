---
title: Go Local Quickstart Example
---

# Go local quickstart example

This is the fastest way to prove the Go runtime can create, read, update, and delete an item without touching a cloud
account.

```bash
make docker-up
make example-local
```

Expected final line:

```text
OK: TableTheory Go quickstart CRUD against DynamoDB Local succeeded
```

The target runs the checked-in program at
[`examples/local-quickstart/main.go`](https://github.com/theory-cloud/TableTheory/blob/main/examples/local-quickstart/main.go).
It sets dummy local credentials, points the runtime at `DYNAMODB_ENDPOINT=http://localhost:8000`, creates the demo table
idempotently, writes a note, reads it back, updates it, deletes it, and fails the process if any step returns an error.

For the complete walkthrough and source listing, see [Getting Started](../getting-started.md#two-command-local-quickstart).
