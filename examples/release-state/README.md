# Release-State Registry Example

This example shows the Go shape for a safety-critical release-state registry:

- a mutable actual-state row with protected registry pin fields;
- immutable write-once event-history rows;
- immutable outbox rows for external side effects;
- deterministic provenance/confidence metadata;
- a transactional actual-state transition plus event append.

The example does not perform a Lambda alias or CodePipeline side effect. Those systems are outside DynamoDB's
transaction boundary and must be handled with explicit outbox, retry, and reconciliation behavior.

See the shared [release-state safety patterns](../../docs/release-state-patterns.md) guide for the full rationale and
the TypeScript/Python companion examples.
