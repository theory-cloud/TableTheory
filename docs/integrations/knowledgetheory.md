---
title: KnowledgeTheory
description: How KnowledgeTheory persists platform state and the knowledge graph through TableTheory.
---

# KnowledgeTheory

[KnowledgeTheory](https://github.com/theory-cloud/knowledgetheory) is the Theory Cloud knowledge graph and platform-state layer. Every platform record it manages — content tables, relations, telemetry, manifest cache, access policies — lives in TableTheory-modeled tables.

## What KnowledgeTheory persists through TableTheory

| KnowledgeTheory surface       | TableTheory primitive                       |
|-------------------------------|---------------------------------------------|
| Content records               | Single-table model + composite keys         |
| Relation edges                | Inverse-GSI access patterns                 |
| Telemetry events              | Append-only with TTL                        |
| Manifest cache                | Lifecycle timestamps + version locking      |
| Access policies               | Tenant-partitioned with conditional reads   |

## Single-table design at scale

KnowledgeTheory is the largest single-table consumer in the Theory Cloud stack. The access patterns are denormalized aggressively — joins are encoded into the key shape rather than computed at query time — which puts TableTheory's marshaling correctness directly on KnowledgeTheory's critical path.

A regression in how TableTheory marshals nested struct fields shows up first as inconsistent KnowledgeTheory query results, often before contract tests notice. This is one of the reasons KnowledgeTheory invests so heavily in its own integration coverage layered on top of the TableTheory contract.

## Why this matters for TableTheory stewardship

KnowledgeTheory writes everything platform-internal. A silent data-corruption regression in TableTheory shows up everywhere — schema evolution, access-policy enforcement, telemetry pipelines, manifest serving. The [contract scenarios](../../reference/contract-scenarios/) plus KnowledgeTheory's own integration suite are the line of defense.

## Related

- [Architecture Patterns](../../architecture-patterns/) — Theory Cloud stack and data-flow
- [Features · CRUD & Marshaling](../../features/crud/) — the marshaling contract KnowledgeTheory depends on
- [KnowledgeTheory repository](https://github.com/theory-cloud/knowledgetheory)
