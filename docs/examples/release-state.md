---
title: Release-state Example
---

# Release-state example

The release-state example shows how TableTheory models protected release records, write policies, and transactional state
transitions.

- Source: [`examples/release-state`](https://github.com/theory-cloud/TableTheory/tree/main/examples/release-state)
- Concept guide: [Release State Patterns](../release-state-patterns.md)
- Runtime contract: [Contract Scenarios](../reference/contract-scenarios.md)

Use this example when a consumer needs append-only or protected state transitions on top of DynamoDB transactions. The
pattern is additive and does not weaken optimistic locking or fail-closed encryption semantics.

## Local verification

`make examples-local` runs the release-state transition for real on DynamoDB Local in all three runtimes:
`examples/release-state` (Go), `ts/examples/release-state.ts` and `py/examples/release_state.py`. Each driver creates
its own table, seeds an actual row at version 7, executes the actual-state update, event append and conditional outbox
create in one `TransactWriteItems` call, asserts the stored rows and version 8, and proves a stale expected version is
rejected. All three print the same parity line.

The Go unit tests in `examples/release-state/main_test.go` use an in-process recording transaction writer. They remain
unit tests only; they are not the example's verification.
