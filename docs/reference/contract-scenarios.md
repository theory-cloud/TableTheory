---
title: Contract Scenarios
description: The P0 specification — the executable contract every runtime is verified against on every commit.
---

# Contract Scenarios

The contract scenarios are TableTheory's **executable specification**. Each scenario is authored language-neutrally and run against every runtime (Go, TypeScript, Python) using a shared DynamoDB Local instance. A scenario that passes in one runtime and fails in another is a parity regression — never a "runtime-specific quirk."

Scenarios live in [`contract-tests/scenarios/`](https://github.com/theory-cloud/tabletheory/tree/main/contract-tests/scenarios) with runners under [`contract-tests/runners/`](https://github.com/theory-cloud/tabletheory/tree/main/contract-tests/runners) and shared infrastructure (DynamoDB Local) in [`contract-tests/docker-compose.yml`](https://github.com/theory-cloud/tabletheory/blob/main/contract-tests/docker-compose.yml).

## The P0 scenarios

The current P0 set — five scenarios that pin the foundation of every Theory Cloud consumer's persistence layer:

### 1. CRUD

The bedrock. Create / read / update / delete against a single-table model with composite keys. Verifies that all three runtimes produce identical DynamoDB items for identical inputs and read them back identically.

See: [Features · CRUD & Marshaling](../features/crud.md)

### 2. Omit-empty

How zero values (`""`, `0`, `False`, empty collections) are handled when `theorydb:"omitempty"` is present vs absent. Every type variant in the matrix is exercised across all three runtimes.

### 3. Lifecycle timestamps

Automatic population of `created_at` and `updated_at` on write operations, with clock ordering semantics consistent across runtimes. Verifies the unit (epoch ms), the "set once on create / set every write on update" semantics, and the monotonicity guarantee.

See: [Features · Lifecycle Timestamps](../features/lifecycle-timestamps.md)

### 4. Optimistic locking

The `version` field populated on read, incremented on write, conditional-expression-guarded against concurrent writers, with consistent typed error types when a version mismatch occurs.

See: [Features · Optimistic Locking](../features/optimistic-locking.md)

### 5. TTL

The `ttl` tag producing a DynamoDB TimeToLive attribute, with consistent units (epoch seconds), consistent expiration behavior, and consistent post-expiration read semantics (expired item ⇒ typed "not found" error).

See: [Features · TTL](../features/ttl.md)

## How P0 grows

P0 scenarios grow only through deliberate `extend-contract-scenarios` work that runs all three runtimes against the new scenario *before* adding it to the P0 set. New shapes incubate in P1/P2 status — important but not yet frozen — until they pass cross-runtime in three consecutive releases.

## How a scenario is run

```bash
# Bring DynamoDB Local up
cd contract-tests
docker compose up -d

# Run scenarios across all three runtimes
make contract-tests          # or: bash contract-tests/run-all.sh
```

Locally and in CI, the runners share the same DynamoDB Local instance, ensuring that "the Go runtime wrote it" and "the Python runtime read it" are exercised against literally the same persisted items.

## Why this is the arbiter

When runtimes disagree, the contract scenario is the deciding evidence. There is no Go reference runtime; there is no TypeScript reference runtime. There is the scenario, and there are three peers that pass it (or one of them is broken).

A change that makes one runtime convenient at the cost of breaking a scenario in another runtime is a refusal by default — the scenarios are the load-bearing surface of the framework.

## Related

- [Reference · DMS Specification](dms-spec.md) — the shape spec the scenarios are layered on
- [Testing](../testing-guide.md) — how to write consumer-level tests on top of these guarantees
- [Architecture Patterns](../architecture-patterns.md) — cross-runtime contract architecture
