---
title: MCP Memory
description: How theory-mcp-server uses TableTheory to persist agent memory entries.
surface: mcp
---

# MCP Memory

`theory-mcp-server` exposes per-agent append-only memory through MCP tools (`memory_recent`, `memory_append`, `memory_get`). The persistence layer behind those tools is a TableTheory-modeled `AgentMemoryEntry` table.

This page describes how the MCP server uses TableTheory — useful both for understanding the model shape and for the recursive observation that **the agent reading this page itself relies on this exact integration to recall context**.

## Shape

Each memory entry is a single TableTheory item. The model is intentionally lean — the MCP server adds richer query patterns on top via GSIs:

```go
type AgentMemoryEntry struct {
    _  struct{} `theorydb:"naming:snake_case"`

    PK string `theorydb:"pk" json:"pk"`  // agent#<agent_id>
    SK string `theorydb:"sk" json:"sk"`  // mem#<epoch_ms>#<entry_id>

    AgentID  string `json:"agent_id"`
    EntryID  string `json:"entry_id"`
    Kind     string `json:"kind"`        // observation | decision | correction | …
    Subject  string `json:"subject"`
    Body     string `json:"body"`
    Tags     []string `json:"tags"`

    CreatedAt int64 `theorydb:"created_at" json:"created_at"`
    UpdatedAt int64 `theorydb:"updated_at" json:"updated_at"`
}
```

The composite key — `agent#<id>` partition with a time-ordered sort — means `memory_recent` is a single backward DynamoDB Query, no scans.

## Why TableTheory and not a separate ledger?

Three reasons:

1. **One contract, many surfaces.** The MCP server already depends on TableTheory for tenant bindings, client namespaces, agent endpoints, and idempotency state. Adding a second persistence layer for memory would be a second contract surface to maintain.
2. **Cross-runtime read.** `theory-mcp-server` is written in Go, but agents written in any language can hit the same MCP endpoint. The persistence layer's contract is the same regardless of caller language.
3. **Encryption composes.** Memory entries that contain sensitive context use `theorydb:"encrypted"` — the same fail-closed primitive used by Autheory credentials.

## The recursive note

Because every TableTheory agent's stewardship session ledger is itself an `AgentMemoryEntry`, calls like `memory_recent` exercise the runtime they document. A failure to load `memory_recent` is either a platform problem **or** a TableTheory regression — and if it's the latter, every consumer downstream is already affected silently.

Treat persistent MCP memory failures as a signal to check the runtime's marshaling, query, and session behavior in parallel with the obvious "maybe the platform is down" hypothesis.

## Related

- [Features · Encryption](../../features/encryption/) — fail-closed model used for sensitive memory entries
- [Architecture Patterns](../../architecture-patterns/) — single-table design across the Theory Cloud stack
- [Reference · DMS Specification](../../reference/dms-spec/) — the underlying contract spec
