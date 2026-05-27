---
title: AppTheory
description: How AppTheory builds on TableTheory's persistence contract.
---

# AppTheory

[AppTheory](https://github.com/theory-cloud/apptheory) is the serverless application framework that sits directly on top of TableTheory in the Theory Cloud stack. Every AppTheory construct that needs durable state — event workloads, jobs ledger, idempotency, sanitization audit, logging profiles — persists through TableTheory.

```
            AppTheory (serverless runtime)
                      │
            ┌─────────┴──────────┐
            │                    │
   TableTheory (data layer)  KnowledgeTheory
```

## What AppTheory depends on

| AppTheory feature             | TableTheory primitive used                 |
|-------------------------------|--------------------------------------------|
| Event workloads               | Single-table state machines + version locking |
| Jobs ledger                   | Sort-key ordering + TTL                    |
| Idempotency state             | Conditional puts + TTL                     |
| Lambda configuration cache    | Lifecycle timestamps                       |
| Sanitization audit            | Append-only writes, time-ordered keys      |
| Logging profile bindings      | Per-tenant partition keys                  |

The AppTheory team owns the shape of these tables; TableTheory owns how the shape is expressed and persisted.

## Why this matters for TableTheory stewardship

A breaking change to TableTheory's marshaling, locking, or encryption semantics breaks AppTheory's idempotency guarantees. The blast radius is not "AppTheory needs a code update" — it's "every AppTheory workload silently misbehaves." That's why the [contract scenarios](../reference/contract-scenarios.md) gate every commit and why breaking changes are coordinated explicitly with the AppTheory steward.

## Stack diagram

```
            AppTheory  ──depends on──▶  TableTheory
                │                            │
                │                            ▼
                └──depends on──▶  DynamoDB
```

No reverse arrow. TableTheory does not consume AppTheory.

## Related

- [Architecture Patterns](../architecture-patterns.md) — the Theory Cloud stack and TableTheory's place in it
- [Features · Optimistic Locking](../features/optimistic-locking.md) — the primitive AppTheory's event workloads compose on
- [AppTheory repository](https://github.com/theory-cloud/apptheory) — AppTheory's own documentation site
