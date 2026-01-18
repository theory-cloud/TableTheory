# TableTheory Spec (DMS) v0.1 (Draft)

Status: **draft**  
Primary goal: make TableTheory’s behavior portable across languages (starting with TypeScript) without semantic drift.

This spec is intentionally **contract-first**: it defines what must be true at runtime, not what any single library API
looks like. Each language implementation is free to choose an ergonomic API as long as it passes the contract tests.

Related planning artifacts:
- `docs/development/planning/theorydb-multilang-roadmap.md`
- `docs/development/planning/theorydb-go-ts-parity-matrix.md`
- `docs/development/planning/theorydb-encryption-tag-roadmap.md`

## Goals (v0.1)

- Define a **language-agnostic** representation of TableTheory model schemas (keys, indexes, attribute names, lifecycle fields).
- Define a single **language-neutral schema source-of-truth** (DMS) that all implementations can load/validate against.
- Define the **runtime semantics** that prevent drift (encoding rules, cursor format, versioning behavior, encryption rules).
- Enable a shared **contract test suite** that can be run against Go, TypeScript, and Python implementations.

## Non-goals (v0.1)

- Infrastructure provisioning (CDK/Terraform/table creation/migrations) beyond describing required table/index shapes.
- Replacing DynamoDB’s own documented semantics (this spec only defines TableTheory-specific behavior).
- Defining the public API shape for each language (builders vs decorators vs codegen).

## Terminology

- **Attribute**: a DynamoDB attribute name stored in an item (e.g., `userId`, `PK`, `createdAt`).
- **Field**: a language-level property/struct field that maps to an attribute.
- **Model**: a named item schema + its table/index bindings.
- **Primary key**: DynamoDB partition key (PK) + optional sort key (SK).
- **Index**: a GSI or LSI definition.
- **Cursor**: an opaque pagination token that encodes DynamoDB `LastEvaluatedKey` + query context.
- **Encrypted envelope**: the stored representation of an encrypted attribute (map containing `v`, `edk`, `nonce`, `ct`).

## DMS file format (proposed)

DMS is a versioned, language-neutral document. v0.1 is intentionally minimal and focused on high-drift behavior.

### Canonical format (v0.1)

To make DMS predictable across languages and safe to parse:

- **Canonical authoring format:** **YAML 1.2** restricted to the **JSON-compatible subset** (no anchors/aliases, no merge
  keys, no custom tags, no timestamps/implicit type magic).
- **Equivalent interchange format:** **JSON** (same object model after parsing).
- **Rule:** a DMS file MUST parse to the same JSON object shape in Go/TypeScript/Python.

### Top-level shape

```yaml
dms_version: "0.1"
namespace: "acme.payments"   # optional; used for grouping + codegen
models:
  - name: "User"
    table:
      name: "users"
    naming:
      convention: "camelCase" # camelCase | snake_case
    keys:
      partition: { attribute: "PK", type: "S" }
      sort:      { attribute: "SK", type: "S" } # optional
    attributes:
      - attribute: "PK"
        type: "S"
        required: true
        roles: ["pk"]
      - attribute: "SK"
        type: "S"
        required: true
        roles: ["sk"]
      - attribute: "createdAt"
        type: "S"
        format: "rfc3339nano"
        roles: ["created_at"]
      - attribute: "updatedAt"
        type: "S"
        format: "rfc3339nano"
        roles: ["updated_at"]
      - attribute: "version"
        type: "N"
        format: "int"
        roles: ["version"]
      - attribute: "ttl"
        type: "N"
        format: "unix_seconds"
        roles: ["ttl"]
        optional: true
      - attribute: "tags"
        type: "SS"
        optional: true
        omit_empty: true
    indexes:
      - name: "gsi-email"
        type: "GSI"
        partition: { attribute: "emailHash", type: "S" }
        projection: { type: "ALL" }
```

### Attribute fields (v0.1)

Every `attributes[]` entry supports:

- `attribute` (string, required): canonical DynamoDB attribute name.
- `type` (string, required): DynamoDB type (`S`, `N`, `B`, `BOOL`, `M`, `L`, `SS`, `NS`, `BS`, `NULL`).
- `required` (bool, default false): attribute must be present (for keys and library-owned lifecycle fields).
- `optional` (bool, default false): attribute may be absent on reads/writes.
- `omit_empty` (bool, default false): omit the attribute on write when it is “empty” by the rules below.
- `roles` (string[], optional): semantic roles: `pk`, `sk`, `created_at`, `updated_at`, `version`, `ttl`, `index_pk:<name>`, `index_sk:<name>`.
- `format` (string, optional): value-level constraints for common patterns:
  - `rfc3339nano` (timestamps)
  - `unix_seconds` (TTL)
  - `int` (version)
- `json` (bool, default false): store a JSON-compatible value as a DynamoDB `S` JSON blob.
  - Requires `type: "S"`.
  - Serialization MUST be deterministic (no insignificant whitespace; object keys sorted recursively).
  - `null` values MUST be stored as DynamoDB `NULL` (not as the string `"null"`).
- `binary` (bool, default false): indicates the attribute is treated as a binary blob.
  - Requires `type: "B"`.
- `encryption` (object, optional): indicates the attribute is stored as an encrypted envelope (see below).
- `tags` (object, optional): extension metadata; must not change core semantics without a spec update.

### Index fields (v0.1)

Each `indexes[]` entry supports:

- `name` (string, required)
- `type` (`GSI` | `LSI`, required)
- `partition` (required): `{ attribute: <string>, type: <"S"|"N"|"B"> }`
- `sort` (optional): `{ attribute: <string>, type: <"S"|"N"|"B"> }`
- `projection` (optional): `{ type: "ALL"|"KEYS_ONLY"|"INCLUDE", fields?: string[] }`

## Runtime semantics (portable behavior)

### Attribute naming (drift prevention)

- Every implementation MUST resolve to the **same DynamoDB attribute names** for a given DMS.
- DMS SHOULD be treated as “fully explicit” (even if a language supports implicit naming conversions).
- If `naming.convention` is present, implementations MUST validate attribute names to that convention:
  - `camelCase`: `^[a-z][A-Za-z0-9]*$` with explicit allowance for `"PK"` / `"SK"` if used.
  - `snake_case`: `^[a-z][a-z0-9]*(_[a-z0-9]+)*$`.

### Timestamp encoding (`created_at`, `updated_at`)

- Stored as DynamoDB `S` strings in **RFC3339Nano** format.
- On write:
  - `created_at`: set to “now” on create/put.
  - `updated_at`: set to “now” on create/put and on update.
- Implementations MUST treat these as library-owned fields (no silent partial updates).

### TTL encoding (`ttl`)

- Stored as DynamoDB `N` containing Unix epoch seconds (integer).
- Implementations MAY accept convenience input types (Go `time.Time`, TS `Date`) but MUST serialize to epoch seconds.

### Optimistic locking (`version`)

- Stored as DynamoDB `N` containing an integer.
- On create/put:
  - if the model has a version field and it is “empty”, it MUST be written as `0`.
- On update:
  - implementations MUST add a condition expression requiring `version == currentVersion` (from the provided model state)
  - implementations MUST atomically increment version by `+1` as part of the update (DynamoDB `ADD`).
- On delete (optional but recommended):
  - if `currentVersion` is provided and non-empty, include a condition `version == currentVersion`.

### `omit_empty` emptiness rules (deterministic)

If `omit_empty: true`, an attribute MUST be omitted from the write when its value is empty by these rules:

- `null`/`undefined` (or language-equivalent) is empty.
- strings: `""` is empty.
- numbers: `0` is empty.
- booleans: `false` is empty.
- arrays/lists: length `0` is empty.
- maps/objects: size `0` is empty.
- structs/records: empty when **all** fields are empty.
- timestamps: empty when zero/invalid (Go `time.Time{}.IsZero()` / TS invalid `Date`).

Rationale: this mirrors the Go implementation’s `omitempty` notion (`internal/reflectutil.IsEmpty`) and avoids
cross-language “truthiness” divergence.

### Set encoding

- DMS set types are represented as DynamoDB sets (`SS`, `NS`, `BS`), not lists.
- DynamoDB does not allow empty sets. Therefore:
  - empty sets MUST be encoded as `NULL` and then omitted if `omit_empty` is true.

### Cursor encoding (pagination token)

Cursor MUST be stable across languages so pagination can be handed between services (Go → TS, TS → Go).

Canonical encoding (matches Go `pkg/query.EncodeCursor`):

- Cursor is `base64url(json(cursor))` using RFC4648 **URL encoding with padding** (`-`/`_` alphabet, `=` padding).
- JSON shape:
  - `lastKey`: map of attribute name → typed AttributeValue JSON (same single-key format as DynamoDB JSON)
  - `index`: optional index name
  - `sort`: optional sort direction
- JSON serialization MUST match Go’s output:
  - UTF-8 JSON with no insignificant whitespace (equivalent to Go `encoding/json.Marshal`)
  - top-level key order MUST be: `lastKey`, then `index` (if present), then `sort` (if present)
  - for all map/object values (including `lastKey` and maps nested under `M`), keys MUST be sorted lexicographically

Typed AttributeValue JSON (one key only):

- `{ "S": "..." }`, `{ "N": "..." }`, `{ "BOOL": true }`, `{ "NULL": true }`
- `{ "B": "<base64>" }` (`<base64>` is standard RFC4648 base64 with padding)
- `{ "SS": ["..."] }`, `{ "NS": ["..."] }`, `{ "BS": ["<base64>", ...] }` (`<base64>` is standard RFC4648 base64 with padding)
- `{ "L": [ <typed-av>, ... ] }`
- `{ "M": { "attr": <typed-av>, ... } }`

### Encrypted fields (`encryption` + envelope shape)

Encrypted fields are a cross-language “semantic drift risk” and must be specified explicitly.

Rules:
- Encrypted attributes MUST NOT be used as primary keys or index keys.
- Encrypted attributes MUST NOT be queryable/filterable (ciphertext is non-deterministic).
- If a model contains any encrypted attributes and encryption is not configured, implementations MUST **fail closed**
  before writing (no plaintext fallback).

Envelope shape (v0.1 aligns with `docs/development/planning/theorydb-encryption-tag-roadmap.md`):

- stored attribute value: DynamoDB `M` with keys:
  - `v`: number (format version; start at `1`)
  - `edk`: binary (encrypted data key)
  - `nonce`: binary (AES-GCM nonce)
  - `ct`: binary (ciphertext)

## Error taxonomy (portable)

Implementations MUST expose typed errors (or stable error codes) for these cases:

- `ErrInvalidModel` / “invalid schema/model”
- `ErrInvalidTag` / “invalid model annotation” (Go-only input; mapped to “invalid schema” in DMS)
- `ErrMissingPrimaryKey`
- `ErrItemNotFound`
- `ErrConditionFailed`
- `ErrInvalidOperator`
- `ErrEncryptedFieldNotQueryable`
- `ErrEncryptionNotConfigured`
- `ErrInvalidEncryptedEnvelope`

## Versioning & compatibility

- `dms_version` is semver-like (`0.x` can change quickly; `1.0+` requires migration rules).
- Every implementation MUST:
  - declare supported DMS versions
  - pin a DMS version in CI contract tests

## Open questions (to resolve before TS P0 ships)

- TTL field types: should convenience timestamp types be allowed everywhere (Go `time.Time`, TS `Date`) while always
  storing epoch seconds?
- Projection behavior: do we standardize projection expressions and “include fields” semantics in v0.2?
