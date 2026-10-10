# Release-State Registry Example

This example shows the Go shape for a safety-critical release-state registry:

- a mutable actual-state row with protected registry pin fields;
- immutable write-once event-history rows;
- immutable write-once outbox rows for external side effects;
- deterministic provenance/confidence metadata;
- one DynamoDB transaction that updates the actual-state row, appends the event, and creates the conditional outbox row.

The example does not perform a Lambda alias or CodePipeline side effect. Those systems are outside DynamoDB's
transaction boundary and must be handled with explicit outbox, retry, and reconciliation behavior.

`make test` runs the example tests, which assert that the actual-state update, the event append, and the outbox create
are submitted as one transaction and that an injected transaction failure commits nothing.

See the shared [release-state safety patterns](../../docs/release-state-patterns.md) guide for the full rationale and
the TypeScript/Python companion examples.
