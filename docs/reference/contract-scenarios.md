---
title: Contract Scenarios
description: The P0 specification — executable scenarios for supported runtime capabilities.
---

# Contract Scenarios

The contract scenarios are TableTheory's **executable specification**. Each
scenario is authored language-neutrally and run against the runtimes that declare
support for that capability using shared DynamoDB Local infrastructure. A
scenario that passes in one supported runtime and fails in another is a parity
regression — never a "runtime-specific quirk."

Scenarios live in [`contract-tests/scenarios/p0/`](https://github.com/theory-cloud/tabletheory/tree/main/contract-tests/scenarios/p0)
with runners under [`contract-tests/runners/`](https://github.com/theory-cloud/tabletheory/tree/main/contract-tests/runners)
and shared infrastructure in [`contract-tests/docker-compose.yml`](https://github.com/theory-cloud/tabletheory/blob/main/contract-tests/docker-compose.yml).

## The P0 scenarios

The current P0 directory contains these scenarios:

| File | Behavior pinned |
|------|-----------------|
| [`01-crud-basic.yml`](https://github.com/theory-cloud/tabletheory/blob/main/contract-tests/scenarios/p0/01-crud-basic.yml) | Create / read / update / delete against a single-table model with composite keys. |
| [`02-omitempty.yml`](https://github.com/theory-cloud/tabletheory/blob/main/contract-tests/scenarios/p0/02-omitempty.yml) | How zero values are handled when `omitempty` is present vs absent. |
| [`03-lifecycle-created-updated.yml`](https://github.com/theory-cloud/tabletheory/blob/main/contract-tests/scenarios/p0/03-lifecycle-created-updated.yml) | Persisted `createdAt` / `updatedAt` shape and update ordering in the contract fixture. |
| [`04-version-optimistic-lock.yml`](https://github.com/theory-cloud/tabletheory/blob/main/contract-tests/scenarios/p0/04-version-optimistic-lock.yml) | Versioned update succeeds once, then stale-version update fails with a typed condition error. |
| [`05-ttl-epoch-seconds.yml`](https://github.com/theory-cloud/tabletheory/blob/main/contract-tests/scenarios/p0/05-ttl-epoch-seconds.yml) | TTL role persists a numeric epoch-seconds attribute and reads it back unchanged. |
| [`06-release-state-write-policy.yml`](https://github.com/theory-cloud/tabletheory/blob/main/contract-tests/scenarios/p0/06-release-state-write-policy.yml) | Release-state write-policy enforcement. |
| [`07-release-state-protected-fields.yml`](https://github.com/theory-cloud/tabletheory/blob/main/contract-tests/scenarios/p0/07-release-state-protected-fields.yml) | Release-state protected-field enforcement. |
| [`08-release-state-transactional-transition.yml`](https://github.com/theory-cloud/tabletheory/blob/main/contract-tests/scenarios/p0/08-release-state-transactional-transition.yml) | Release-state transition append performed transactionally. |
| [`09-release-state-provenance-confidence.yml`](https://github.com/theory-cloud/tabletheory/blob/main/contract-tests/scenarios/p0/09-release-state-provenance-confidence.yml) | Release-state deploy-authority provenance and confidence validation. |

### CRUD

The bedrock. Create / read / update / delete against a single-table model with
composite keys. Verifies that supported runtimes produce compatible DynamoDB
items for identical inputs and read them back identically.

See: [Features · CRUD & Marshaling](../features/crud.md)

### Omit-empty

How zero values (`""`, `0`, `False`, empty collections) are handled when
`theorydb:"omitempty"` is present vs absent. Every type variant in the matrix is
exercised across supported runtimes.

### Lifecycle timestamps

The lifecycle fixture pins the persisted `createdAt` / `updatedAt` fields and
the ordering expectation after an update. Go, TypeScript, and Python all execute
this shared P0 fixture and generate lifecycle values on the high-level write
paths used by the scenario.

See: [Features · Lifecycle Timestamps](../features/lifecycle-timestamps.md)

### Optimistic locking

The `version` field is populated on read, incremented on versioned write, and
conditional-expression-guarded against concurrent writers, with consistent typed
error behavior when a version mismatch occurs.

See: [Features · Optimistic Locking](../features/optimistic-locking.md)

### TTL

The TTL fixture pins the `ttl` role as a numeric epoch-seconds attribute. It does
not assert read-time masking of still-present expired items. DynamoDB TTL deletion
is eventual, and current read paths return any item DynamoDB physically returns.

See: [Features · TTL](../features/ttl.md)

## How P0 grows

P0 scenarios grow only through deliberate `extend-contract-scenarios` work that
runs the supported runtimes against the new scenario *before* adding it to the P0
set. New shapes incubate in P1/P2 status — important but not yet frozen — until
they pass cross-runtime in three consecutive releases.

## How a scenario is run

```bash
# Bring DynamoDB Local up
cd contract-tests
docker compose up -d

# Run scenarios across all supported runners
make contract-tests          # or: bash contract-tests/run-all.sh
```

Locally and in CI, the runners share the same DynamoDB Local instance, ensuring
that "the Go runtime wrote it" and "another runtime read it" are exercised
against literally the same persisted items when the scenario requires that shape.

## Why this is the arbiter

When runtimes disagree for a supported scenario, the contract scenario is the
deciding evidence. There is no Go reference runtime; there is no TypeScript
reference runtime. There is the scenario, and there are peers that pass it (or
one of them is broken).

A change that makes one runtime convenient at the cost of breaking a scenario in
another runtime is a refusal by default — the scenarios are the load-bearing
surface of the framework.

## Related

- [Reference · DMS Specification](dms-spec.md) — the shape spec the scenarios are layered on
- [Testing](../testing-guide.md) — how to write consumer-level tests on top of these guarantees
- [Architecture Patterns](../architecture-patterns.md) — cross-runtime contract architecture
