# Release-State Registry Example

This example shows the Go shape for a safety-critical release-state registry:

- a mutable actual-state row with protected registry pin fields;
- immutable write-once event-history rows;
- immutable write-once outbox rows for external side effects;
- deterministic provenance/confidence metadata;
- one DynamoDB transaction that updates the actual-state row, appends the event, and creates the conditional outbox row.

The example does not perform a Lambda alias or CodePipeline side effect. Those systems are outside DynamoDB's
transaction boundary and must be handled with explicit outbox, retry, and reconciliation behavior.

`make test` runs the example unit tests, which assert that the actual-state update, the event append, and the outbox create
are submitted as one transaction and that an injected transaction failure commits nothing. Those tests use an in-process
recording writer and remain unit coverage only.

For real local verification, `go run ./examples/release-state` runs the same transition against DynamoDB Local: it creates
the table, seeds an actual row at version 7, executes the transaction, asserts the stored actual/event/outbox rows and
version 8, and proves a stale expected version is refused. `make examples-local` runs that Go driver plus the matching
TypeScript (`ts/examples/release-state.ts`) and Python (`py/examples/release_state.py`) drivers; all three print the same
parity line.

See the shared [release-state safety patterns](../../docs/release-state-patterns.md) guide for the full rationale and
the TypeScript/Python companion examples.
