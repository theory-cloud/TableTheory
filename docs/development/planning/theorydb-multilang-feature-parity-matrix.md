# TableTheory: Multi-language Feature Parity Matrix (Go / TypeScript / Python)

Goal: prevent “same name, different semantics” as we expand beyond Go. This document defines **what parity means** and
tracks **current parity status** across:

- Go (repo root) — current reference implementation
- TypeScript (`ts/`) — Node.js 24 / AWS SDK v3
- Python (`py/`) — Python 3.14 / boto3

Parity is not “APIs look similar”. Parity is **behavioral equivalence**, proven by shared fixtures + contract tests +
integration tests.

Related:

- Spec: `docs/development/planning/theorydb-spec-dms-v0.1.md`
- Roadmap: `docs/development/planning/theorydb-multilang-roadmap.md`
- Go↔TS matrix (historical): `docs/development/planning/theorydb-go-ts-parity-matrix.md`
- Contract tests outline: `docs/development/planning/theorydb-contract-tests-suite-outline.md`

## Parity tiers (behavioral)

- **P0 — Core parity:** schema + deterministic encoding + CRUD + conditional writes + typed errors (ship production services).
- **P1 — Query parity:** query/scan + index selection + pagination cursor rules (read-heavy patterns).
- **P2 — Multi-item parity:** batch get/write + transactions with explicit retry/partial failure semantics.
- **P3 — Streams parity:** stream image unmarshalling helpers (Lambda events).
- **P4 — Encryption parity:** `encrypted` semantics are real (KMS envelope, fail-closed, not queryable, not key/indexable).

## First-class scope (all languages)

The intention is that **every language is first-class**: not a minimal surface, not “best effort”.

Therefore parity scope includes (in addition to P0–P4):

- **Language-neutral schema source-of-truth:** a shared TableTheory Spec (DMS) document that all three implementations can
  load/validate against (no “same name, different meaning”).
- **Schema/table utilities:** dev/test table helpers (create/ensure/delete/describe), and optional local auto-migrate when
  appropriate (DynamoDB Local workflows should not be Go-only).
- **Full query/update surface:** filters (AND/OR groups), projections/select, pagination helpers, update-builder style ops,
  and other ergonomic helpers that exist in Go today.
- **Operational/runtime helpers:** Lambda-focused defaults and (where relevant) multi-account assume-role helpers.
- **Security/hardening modules:** validation and resource-protection helpers that are part of Go’s “secure by default”
  story (must be mapped into TS/Py with equivalent guarantees).
- **Extensibility + testability:** custom type conversion hooks and public mocks/testkit utilities so downstream services
  can test cheaply and consistently.

## Snapshot (current)

This snapshot is intentionally blunt. “Partial” means there is known drift risk, missing behavior, or missing contract
tests. P0–P4 is the **behavioral** view; “first-class scope” items are tracked in the feature matrix below.

| Language | P0 | P1 | P2 | P3 | P4 | Major known gaps |
| --- | --- | --- | --- | --- | --- | --- |
| Go | ✅ | ✅ | ✅ | ✅ | ✅ | (reference) |
| TypeScript (`ts/`) | ✅ | ✅ | ✅ | ✅ | ⚠️ | KMS provider exists; still needs stronger “encrypted attribute” end-to-end coverage in CI |
| Python (`py/`) | ⚠️ | ✅ | ✅ | ✅ | ✅ | Lifecycle automation missing (`created_at`/`updated_at`/`version`/`ttl`) |

## Parity matrix (features)

Legend:
- **Yes**: implemented with tests
- **Partial**: implemented but missing required behavior/tests for parity
- **No**: not implemented

| Area | Feature | Go | TypeScript | Python | Contract tests | Notes |
| --- | --- | --- | --- | --- | --- | --- |
| Spec | Language-neutral schema definition (DMS) used as the source-of-truth | **Partial** | **Partial** | **Partial** | N/A | DMS-first workflow exists; full “DMS is the only source-of-truth” enforcement is still in progress |
| Spec | DMS loader (YAML/JSON) → runtime model schema | Yes | Yes | Yes | N/A | Go: `pkg/dms`; TS: `ts/src/dms.ts`; Py: `py/src/theorydb_py/dms.py` |
| Schema | PK/SK roles | Yes | Yes | Yes | No | Py uses dataclass metadata; TS uses `defineModel` schema |
| Schema | GSI/LSI definitions | Yes | Yes | Yes | No | All languages can declare indexes |
| Schema | Attribute naming determinism | Yes | Yes | Yes | No | DMS should be explicit-name first; avoid implicit drift |
| Encoding | `omitempty` emptiness rules | Yes | Yes | Yes | No | Must be pinned by fixtures (falsey values, empty maps/lists) |
| Encoding | Empty sets encode as `NULL` (never empty `SS/NS/BS`) | Yes | Yes | Yes | Yes | Contract tests pin this behavior |
| Expressions | Reserved word escaping via `ExpressionAttributeNames` | Yes | Yes | Yes | Yes | Contract tests cover common reserved words (e.g., `size`) |
| Lifecycle | `created_at` / `updated_at` auto-populate | Yes | Yes | **No** | No | Py needs parity (or DMS must mark as “optional feature”) |
| Lifecycle | `version` optimistic locking | Yes | Yes | **No** | No | Py currently supports raw condition expressions but not automatic version semantics |
| Lifecycle | `ttl` epoch seconds | Yes | Yes | **No** | No | Py currently treats attributes as explicit; no TTL role semantics yet |
| CRUD | Put/Get/Update/Delete | Yes | Yes | Yes | No | All three work end-to-end against DynamoDB Local |
| Conditions | Conditional writes (if-not-exists / expressions) | Yes | Yes | Yes | No | TS has first-class flags; Py accepts raw expressions |
| Errors | Typed errors taxonomy | Yes | Yes | Yes | No | Needs parity mapping doc + contract tests for common AWS codes |
| Query | Query + key operators | Yes | Yes | Yes | No | Operators parity exists; must add cross-language fixtures |
| Query | Scan | Yes | Yes | Yes | No | Py/TS support basic scan + cursor |
| Pagination | Cursor encoding/decoding | Yes | Yes | Yes | Yes | Golden cursor fixture enforced in Go/TS/Py contract runners |
| Index | Index selection (table vs GSI/LSI) | Yes | Yes | Yes | No | Both TS and Py support index selection |
| Consistency | ConsistentRead rules | Yes | Yes | Yes | No | Must enforce “no consistent read on GSI” across languages |
| Batch | BatchGet + retry semantics | Yes | Yes | Yes | No | Needs explicit, shared “unprocessed” semantics fixtures |
| Batch | BatchWrite + retry semantics | Yes | Yes | Yes | No | Same as above |
| Tx | TransactWrite + error surfacing | Yes | Yes | Yes | No | TS/Py need parity on condition failures vs mixed cancellations |
| Streams | Unmarshal stream image | Yes | Yes | Yes | No | Py/TS implement Lambda stream helpers; add fixtures for map/list/binary |
| Encryption | Envelope format (`v`,`edk`,`nonce`,`ct`) | Yes | Yes | Yes | Yes | Contract tests pin envelope shape across languages |
| Encryption | Fail-closed when unconfigured | Yes | Yes | Yes | No | TS requires encryption provider; Py requires `kms_key_arn` |
| Encryption | AAD binding to attribute name | Yes | Yes | Yes | Yes | Contract tests pin AAD binding failure behavior (attribute swap must fail) |
| Schema mgmt | Create/Ensure/Delete/Describe table helpers | Yes | Yes | Yes | No | TS: `ts/src/schema.ts`; Py: `py/src/theorydb_py/schema.py` |
| Schema mgmt | AutoMigrate (dev/local) | Yes | No | No | No | Go provides; TS/Py should add or explicitly scope as not supported |
| Query DSL | Filter expressions + AND/OR filter groups | Yes | Yes | Yes | No | TS: `FilterExpressionBuilder`; Py: `FilterCondition`/`FilterGroup` + Table filter builder |
| Query DSL | OrderBy + Select/projection helpers | Yes | **Partial** | **Partial** | No | TS/Py support projections; Go has broader DSL; define standard behavior |
| Query DSL | Offset (skip N items) | Yes | No | No | No | Go supports; must define portable semantics (DynamoDB has no native offset) |
| Scan | ParallelScan / ScanAllSegments | Yes | Yes | Yes | No | TS: `ScanBuilder.scanAllSegments`; Py: `Table.scan_all_segments` |
| Update DSL | Fluent UpdateBuilder (SET/REMOVE/ADD/DELETE/list ops) | Yes | Yes | Yes | No | TS: `ts/src/update-builder.ts`; Py: `UpdateBuilder` in `py/src/theorydb_py/table.py` |
| Aggregates | Sum/Avg/Min/Max + GroupBy helpers | Yes | Yes | Yes | No | In-memory helpers; TS: `ts/src/aggregates.ts`; Py: `py/src/theorydb_py/aggregates.py` |
| Optimization | Query optimizer/plan cache | Yes | Yes | Yes | No | Advisory-only hints; TS: `ts/src/optimizer.ts`; Py: `py/src/theorydb_py/optimizer.py` |
| Runtime | Lambda-optimized DB wrapper + cold-start helpers | Yes | Yes | Yes | No | TS: `ts/src/lambda.ts`; Py: `py/src/theorydb_py/runtime.py` |
| Runtime | Multi-account assume-role wrapper | Yes | Yes | Yes | No | TS: `ts/src/multiaccount.ts`; Py: `py/src/theorydb_py/multiaccount.py` |
| Security | Field/operator/expression validation & injection hardening | Yes | Yes | Yes | No | TS: `ts/src/validation.ts`; Py: `py/src/theorydb_py/validation.py` |
| Security | Resource protection (rate limiting, concurrency, memory monitor) | Yes | Yes | Yes | No | TS: `ts/src/protection.ts`; Py: `py/src/theorydb_py/protection.py` |
| Extensibility | Custom type converters / pluggable marshaling | Yes | Yes | Yes | No | TS: `ValueConverter`; Py: `AttributeConverter` |
| Testing | Public mocks/testkit for DynamoDB + KMS | Yes | Yes | Yes | No | TS: `ts/src/testkit`; Py: `py/src/theorydb_py/testkit.py` |

## What “parity complete” means (acceptance criteria)

A feature is “at parity” only when:

1) It is defined in DMS (or explicitly marked as “not in DMS / intentionally language-specific”), and
2) It passes:
   - unit tests (pure logic), and
   - DynamoDB Local-backed integration tests (end-to-end), and
   - shared contract tests (golden fixtures where drift risk is high: cursor, encryption envelope, reserved words), and
3) It is wired into the rubric with “no green-by-exclusion” (pinned tooling, strict defaults).

## Highest-risk drift points (prioritize next)

- **DMS-first schema:** without a shared schema source-of-truth, semantic drift is always possible (even with good tests).
- **Lifecycle parity:** decide whether lifecycle roles are required in all languages; if yes, implement in Py with tests.
- **Full query/update DSL parity:** filters/groups and update-builder semantics are large drift surfaces without shared fixtures.
- **Schema mgmt parity:** dev/test workflows should not be Go-only (CDK isn’t always available in unit tests).
- **Mocks/testkit parity:** every language should ship a public, supported mocking surface for DynamoDB + KMS so downstream
  services can test cheaply and consistently.
