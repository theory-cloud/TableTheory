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
