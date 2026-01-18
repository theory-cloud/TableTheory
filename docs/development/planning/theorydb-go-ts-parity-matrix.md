# TableTheory Go↔TypeScript Parity Matrix (P0/P1/P2)

This document defines what “parity” means between the Go implementation (this repo) and the planned TypeScript
implementation (`tabletheory-ts`), using three tiers:

- **P0 — Core parity:** schema + encoding + CRUD + conditions (enough to ship production services).
- **P1 — Query parity:** query/scan + pagination + index selection (read-heavy production patterns).
- **P2 — Multi-item parity:** batch + transactions (high-throughput + atomic patterns).

Source-of-truth behavior definition:
- `docs/development/planning/theorydb-spec-dms-v0.1.md`

## Parity matrix

Legend:
- **Go**: current behavior in this repo
- **TS target**: tier where TS must match Go (and pass shared contract tests)

| Area | Feature | Go | TS target | Notes |
| --- | --- | --- | --- | --- |
| Schema | Explicit PK/SK roles | Yes | P0 | DMS keys + model validation |
| Schema | GSI/LSI definitions | Yes | P1 | Needed for query/index selection parity |
| Schema | Attribute naming conventions | Yes | P0 | Prefer explicit names in DMS; validate `camelCase`/`snake_case` |
| Schema | `attr:` explicit attribute names | Yes | P0 | TS should support explicit attribute names (no implicit drift) |
| Schema | `naming:*` convention selector | Yes | P0 | TS equivalent should be config-level, not per-field magic |
| Schema | Unknown key-value tags (extensions) | Yes | P0 | Allow metadata passthrough; must not affect semantics |
| Encoding | RFC3339Nano timestamps | Yes | P0 | Applies to `created_at` / `updated_at` |
| Encoding | Cursor format (base64url JSON) | Yes | P1 | Must be byte-for-byte compatible for cross-service pagination |
| Encoding | Sets (`SS`/`NS`/`BS`) | Partial | P0 | Go supports sets; empty-set handling must be standardized in tests |
| Encoding | `omitempty` emptiness rules | Yes | P0 | Mirror deterministic rules from DMS (not JS truthiness) |
| Lifecycle | `created_at` auto-populate | Yes | P0 | Library-owned timestamp |
| Lifecycle | `updated_at` auto-populate | Yes | P0 | Set on create + update |
| Lifecycle | `version` optimistic locking | Yes | P0 | Update adds condition + increments via `ADD` |
| Lifecycle | `ttl` (epoch seconds) | Yes | P0 | DMS says N unix seconds; TS should accept `Date` but store N |
| CRUD | Create/Put | Yes | P0 | Includes lifecycle behavior |
| CRUD | Get (First) | Yes | P0 | Typed error mapping for not-found vs other failures |
| CRUD | Update | Yes | P0 | Includes version increment/condition when model has version |
| CRUD | Delete | Yes | P0 | Version-guarded delete recommended when version present |
| Conditions | IfNotExists / IfExists | Yes | P0 | Canonical idempotency/guard patterns |
| Conditions | Additional write conditions | Yes | P0 | `WithCondition`/condition expressions |
| Query | Query (key conditions) | Yes | P1 | Operators + key-only vs filter semantics |
| Query | Scan | Yes | P1 | Include parallel scan later if needed |
| Query | Filter groups (AND/OR) | Yes | P1 | Expression builder parity required |
| Query | Projection / select | Yes | P1 | Align projection expression semantics |
| Query | Consistent read option | Yes | P1 | Read consistency toggle |
| Query | Pagination (`AllPaginated`) | Yes | P1 | Cursor compatibility is the critical part |
| Batch | BatchGet | Yes | P2 | Partial-failure handling must be explicit + tested |
| Batch | BatchWrite (create/delete) | Yes | P2 | Retry/unprocessed semantics must match |
| Tx | TransactWrite | Yes | P2 | Error taxonomy should expose per-op failures |
| Security | Encrypted fields (KMS envelope) | Yes | Post-P2 | TS should implement after P2 with same envelope + fail-closed rules |
| Streams | Stream image unmarshal helpers | Yes | Post-P2 | Useful, but not required for TS P0–P2 ship |
| Schema mgmt | Table create/migrate helpers | Yes | Post-P2 | Decide whether TS owns infra or stays runtime-only |

## Known parity hazards (track early)

- **TTL type mismatch in examples:** DMS standardizes storage as epoch seconds; ensure examples and TS implementation
  follow the same rule even if Go examples drift.
- **Empty sets:** DynamoDB forbids empty sets; the shared contract tests should pin the required encoding behavior.
- **Cursor byte compatibility:** base64 encoding variants and JSON key ordering can break cross-language cursors; TS must
  produce the same canonical encoding (including base64url alphabet and field names).

