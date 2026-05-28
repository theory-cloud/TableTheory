---
title: DMS Specification v0.1
description: The language-neutral Data Model Specification all three TableTheory runtimes validate against.
---

# DMS Specification v0.1

DMS — the **Data Model Specification** — is TableTheory's language-neutral source of truth. It is **not** generated from the Go runtime. It is authored independently and every runtime (Go, TypeScript, Python) is validated against it on every build.

When DMS evolves, every runtime evolves with it. The discipline for managing that synchronization is the [`evolve-dms`](https://github.com/theory-cloud/tabletheory/tree/main/docs/development/planning) skill.

The full canonical specification lives in the repository at [`docs/development/planning/theorydb-spec-dms-v0.1.md`](https://github.com/theory-cloud/tabletheory/blob/main/docs/development/planning/theorydb-spec-dms-v0.1.md). This page describes the public-facing shape; consult the canonical document for the precise semantics every runtime must implement.

## Shape

DMS is a YAML document describing one or more models:

```yaml
dms_version: "0.1"
models:
  - name: "Note"
    table:
      name: "notes_contract"
    keys:
      partition: { attribute: "PK", type: "S" }
      sort:      { attribute: "SK", type: "S" }
    attributes:
      - { attribute: "PK",      type: "S", required: true, roles: ["pk"] }
      - { attribute: "SK",      type: "S", required: true, roles: ["sk"] }
      - { attribute: "body",    type: "S", required: false }
      - { attribute: "version", type: "N", required: true, roles: ["version"] }
      - { attribute: "created_at", type: "N", required: true, roles: ["created_at"] }
      - { attribute: "updated_at", type: "N", required: true, roles: ["updated_at"] }
```

## What DMS specifies

- **Attribute types** — DynamoDB-native (`S`, `N`, `B`, `BOOL`, `L`, `M`, `SS`, `NS`, `BS`, `NULL`).
- **Key shapes** — partition / sort / GSI keys, including composite forms.
- **Tag roles** — every `theorydb:` tag has an equivalent DMS role.
- **Marshaling rules** — how each attribute type is serialized to DynamoDB AttributeValues.
- **Validation rules** — required attributes, omitempty semantics, naming strategies.
- **Portability boundaries** — what is shared across runtimes and what is runtime-specific (very little).

## What DMS does not specify

- **Access patterns.** DMS describes shape, not how consumers query. Access patterns are the consumer's design choice.
- **Indexing strategies beyond the keys.** DMS pins which GSIs exist; whether you use one for a given access pattern is up to you.
- **Application logic.** DMS is the persistence contract; application semantics live in your code.

## Why authored, not generated

DMS is authored separately from any runtime to enforce that runtimes implement *a specification* rather than mirror *each other*. When the Go runtime changes, that change is valid only if it still passes DMS validation — not because TypeScript also implements the same change.

This is the same discipline that keeps the [contract scenarios](contract-scenarios.md) the arbiter when runtimes disagree.

## Related

- [Contract Scenarios](contract-scenarios.md) — the executable spec layered on top of DMS
- [Struct Definition Guide](../struct-definition-guide.md) — how DMS tags become Go/TS/Python model declarations
- [Architecture Patterns](../architecture-patterns.md) — DMS's place in the cross-runtime contract
