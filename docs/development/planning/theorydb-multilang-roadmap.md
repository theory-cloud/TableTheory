# TableTheory: Multi-language Roadmap (TypeScript First)

Goal: expand TableTheory’s “one way to define and access data” pattern beyond Go (starting with **TypeScript**, then Python)
while preventing **semantic drift** and **inconsistency** across services written in different languages.

This is a roadmap, not an API promise. The key constraint is that multi-language TableTheory only works if we treat behavior as
a **versioned contract** and verify it continuously.

## Monorepo direction (decision)

The goal is a **multi-language monorepo** so the spec, fixtures, and implementations evolve together and drift is caught early.

Initial layout (no Go move yet):

- `/` — Go implementation (this repo today)
- `contract-tests/` — shared DMS fixtures + cross-language runners
- `ts/` — TypeScript implementation (Phase 1 focus)
- `py/` — Python implementation (Phase 2)
- `docs/` — spec + planning + developer docs

Versioning in a monorepo:

- **Single shared version:** Go, TypeScript, and Python move together under the same GitHub tag/release (`vX.Y.Z` and `vX.Y.Z-rc.N`).
- **No registry publishing (for now):** TypeScript is not published to npm and Python is not published to PyPI; GitHub releases are the source of truth.
- **DMS is separately versioned:** implementations pin a DMS version (it may or may not match the repo version).

## Principles (non-negotiable)

- **Single source of truth:** a model’s keys/indexes/attribute names should be defined once, not re-invented per service.
- **First-class languages:** Go/TypeScript/Python are equal implementations; Go-only features must be explicitly scoped and tracked.
- **Contract over convenience:** behavior is specified and tested; implementations must match the contract.
- **Safe-by-default:** no “looks secure” metadata-only features; tags like `encrypted` must have enforced semantics.
- **Serverless-first:** optimize for AWS Lambda cold start, bundle size, and minimal runtime overhead.
- **Typed surface:** ergonomic, strongly typed APIs in each language (TS generics; Python typing/pydantic/dataclasses).

## Strategy: Spec + contract tests (drift prevention)

### 1) Create a language-agnostic “TableTheory Spec” (DMS)

Treat this Go repo’s behavior as the starting reference, then move the *definition* of behavior into a versioned spec.

The spec should cover two things:

1) **Model schema contract**
   - PK/SK definition
   - attribute naming (defaults + explicit override)
   - GSI/LSI definitions
   - lifecycle fields (`created_at`, `updated_at`, `version`, `ttl`)
   - modifiers (`omitempty`, `set`, `json`, `binary`, `encrypted`, `-`)
2) **Operation semantics**
   - CRUD (including conditional writes)
   - query/scan semantics (operators, index selection, pagination)
   - batch + transactions
   - streams unmarshalling
   - error taxonomy (typed errors, “fail closed” rules)

Additionally, DMS must be a **language-neutral schema source-of-truth**:

- Canonical authoring format: **YAML 1.2** restricted to the **JSON-compatible subset**
- Equivalent interchange format: **JSON**
- Rule: a DMS file must parse to the **same JSON object shape** in Go/TypeScript/Python

Recommended: publish DMS as its own repo (or a dedicated folder) with:
- a versioned schema (JSON Schema/YAML + examples)
- a feature/compatibility matrix
- a changelog and “breaking change” rules

Drafts in this repo (starting point):
- `docs/development/planning/theorydb-spec-dms-v0.1.md`
- `docs/development/planning/theorydb-go-ts-parity-matrix.md`

### 2) Build a shared contract test suite

The primary drift-prevention mechanism is **one test suite** that all language implementations must pass:

- DynamoDB Local-backed integration tests for end-to-end semantics
- deterministic unit tests for pure components (expression building, marshaling)
- “golden” fixtures for edge cases (reserved words, nested docs, sets, null-ish values)

This should be runnable in CI for each language implementation with a pinned DynamoDB Local version (same philosophy as this
repo’s `docs/development/planning/*` gates: pinned tools, no “green by exclusion”).

Runnable outline (starting point):
- `docs/development/planning/theorydb-contract-tests-suite-outline.md`

## Roadmap

### Phase 0 — Alignment (before writing TypeScript code)

#### ML-0 — Decide repo layout + ownership

**Goal:** avoid fragmentation and unclear canonical behavior.

**Decisions to make (pick and document)**
- Repo strategy:
  - **Option B (chosen):** multi-language monorepo (Go at repo root, plus `ts/` and `py/`)
  - Option A (rejected for now): separate repos per language + separate spec repo
- “Reference implementation” policy:
  - start with Go as reference, then move “truth” to DMS
- Release + versioning policy:
  - single shared repo version across languages (Go + TS + Python)
  - no registry publishing (GitHub releases only)
  - DMS semver’ed and pinned by implementations

**Acceptance criteria**
- A short decision doc exists (can live alongside DMS) covering repo strategy, reference policy, and versioning.
- Monorepo layout is documented (at least `contract-tests/`, `ts/`, `py/` conventions).

---

#### ML-1 — Draft DMS v0.1 + feature matrix

**Goal:** make “one way to define/access data” portable and explicit.

**Acceptance criteria**
- DMS v0.1 can express:
  - PK/SK + attribute naming
  - GSI/LSI
  - lifecycle fields (`created_at`, `updated_at`, `version`, `ttl`)
  - `encrypted` constraints (not keys; not queryable; fail-closed when unconfigured)
- DMS v0.1 specifies a **canonical language-neutral format** (YAML 1.2 JSON-subset, plus equivalent JSON).
- A DMS schema validator exists (JSON Schema recommended) and is used in CI to validate DMS fixtures/examples.
- A feature matrix exists: rows are features, columns are `go/ts/py`, with parity tiers (P0/P1/P2…)

---

#### ML-2 — DMS loaders + validation (all languages)

**Goal:** remove schema duplication as a drift vector by making DMS loadable everywhere.

**Acceptance criteria**
- Go/TS/Py each have a DMS loader that can read YAML/JSON and validate:
  - required fields/types
  - invariant rules (no encrypted keys/indexes, role uniqueness, etc)
  - naming convention constraints
- A repo-local command exists to validate all DMS fixtures (example: `bash scripts/verify-dms.sh`).
- Contract tests and examples can consume the same DMS model definitions without hand-translating them per language.

---

#### ML-3 — Shared contract tests (minimal)

**Goal:** prevent “same name, different semantics” across languages.

**Acceptance criteria**
- A minimal cross-language test plan exists (even as a doc first), covering:
  - CRUD + conditional writes
  - query operators + pagination/cursors
  - index selection and GSI/LSI behavior
  - transactions + batch behavior (including partial failure semantics)
  - `encrypted` semantics (fail-closed + round-trip)
- DynamoDB Local version is pinned for these tests.
- Contract runners for Go/TS/Py load the same DMS + scenarios (no per-language re-definition of model shape).

### Phase 1 — TypeScript (`tabletheory-ts`) MVP

#### TS-0 — Tooling + package skeleton

**Goal:** a stable foundation that can ship and be maintained.

**Recommended defaults**
- Node 24 (Lambda runtime), TypeScript 5+
- AWS SDK v3
- strict lint + formatting + typecheck in CI
- integration tests run against DynamoDB Local (endpoint via `DYNAMODB_ENDPOINT`)

**Acceptance criteria**
- Package builds cleanly (ESM-first), passes `tsc`, lint, and tests.
- CI runs unit + integration tests against pinned DynamoDB Local.

---

#### TS-1 — Model definition API (one way to define data)

**Goal:** match Go’s “struct tags define schema” with an ergonomic, typed TS equivalent.

**Decisions to make**
- Model definition approach:
  - schema builder (`defineModel({ pk, sk, indexes, attributes… })`)
  - decorators/reflect metadata (heavier; more magic)
  - codegen from DMS (ideal for “single source of truth”)

**Acceptance criteria**
- TS can define: PK/SK, attribute names, GSI/LSI, lifecycle fields, modifiers (`omitempty/set/json/binary/encrypted/-`).
- Model definitions are losslessly representable as DMS v0.1 (YAML/JSON) and can be loaded from DMS for contract tests and examples.
- The library can validate model definitions early (fail fast on invalid combinations).

---

#### TS-2 — Core operations parity (P0)

**Goal:** shipping-grade CRUD + conditions with typed results.

**Acceptance criteria**
- Create/Put, Get, Update, Delete work with:
  - conditional expressions (idempotency / optimistic concurrency use-cases)
  - typed errors for common DynamoDB failure modes (condition failed, validation, not found)
- Marshaling/unmarshaling is deterministic and round-trips correctly for supported types.

---

#### TS-3 — Query builder parity (P1)

**Goal:** match the core query ergonomics without leaking raw expression strings.

**Acceptance criteria**
- Query + Scan support:
  - index selection (table vs GSI/LSI)
  - common operators (`=`, `<`, `<=`, `>`, `>=`, `between`, `begins_with`)
  - pagination (cursor in/out; deterministic ordering rules)
  - `limit`, projection/selection, optional consistent reads

---

#### TS-4 — Batch + transactions (P2)

**Goal:** unlock production patterns (bulk writes; atomic multi-item updates).

**Acceptance criteria**
- Batch get/write with partial-failure handling and retry semantics that are explicit and test-covered.
- Transaction write support with condition checks and clear error reporting.

---

#### TS-5 — Streams + events (P3)

**Goal:** parity with Go’s stream parsing ergonomics.

**Acceptance criteria**
- Streams image unmarshalling (New/Old image) into typed models.
- Clear handling of missing/optional attributes and type mismatches.

---

#### TS-6 — `encrypted` semantics (P4)

**Goal:** enforce real encryption semantics, not metadata.

**Acceptance criteria**
- Envelope encryption via AWS KMS (mirrors Go semantics from `docs/development/planning/theorydb-encryption-tag-roadmap.md`):
  - fail closed when encrypted fields exist but KMS config is missing
  - encrypted fields rejected for PK/SK and indexes
  - encrypted fields not queryable/filterable
  - round-trip tests (write → read → decrypt)

---

#### TS-7 — Documentation + examples + first stable release (P0–P2)

**Goal:** make adoption copy/pasteable and reduce “AI-generated misuse”.

**Acceptance criteria**
- README + “Getting started” + core patterns equivalent to Go docs (Lambda init, pagination, optimistic locking, batch, tx).
- Examples include local DynamoDB and Lambda usage.
- A parity statement exists: which tiers/features match Go and which are intentionally missing.

### Phase 2 — Python (`tabletheory-py`)

Build `tabletheory-py` with the same contract-driven posture as Go and TypeScript.

#### PY-0 — Tooling + package skeleton

**Goal:** a stable foundation that can ship and be maintained.

**Recommended defaults**
- Python 3.14 (pinned), AWS Lambda target runtime (or container runtime if 3.14 is not yet available)
- AWS SDK: `boto3` (sync) + `botocore` exceptions mapping
- `ruff` (format + lint), `pyright` or `mypy` (typecheck), `pytest` (tests)
- integration tests run against DynamoDB Local (endpoint via `DYNAMODB_ENDPOINT`)

**Acceptance criteria**
- `py/` package builds cleanly (wheel/sdist) and is importable.
- CI runs format check, lint, typecheck, unit tests, and DynamoDB Local-backed integration tests.
- Integration tests are strict-by-default (only skipped when `SKIP_INTEGRATION=1|true`).
- Rubric is extended to enforce the Python surface (no weaker than Go/TS).
- Release automation updates Python package version files to match the repo version (and a verifier enforces alignment).

---

#### PY-1 — Model definition API (one way to define data)

**Goal:** match Go’s struct-tag schema and TS schema builder with a typed Python equivalent.

**Decisions to make**
- Model definition approach:
  - dataclasses + field metadata (lightweight)
  - pydantic models (heavier; validation-first)
  - codegen from DMS (ideal for “single source of truth”)

**Acceptance criteria**
- Python can define: PK/SK, attribute names, GSI/LSI, lifecycle fields, modifiers (`omitempty/set/json/binary/encrypted/-`).
- Model definitions are losslessly representable as DMS v0.1 (YAML/JSON) and can be loaded from DMS for contract tests and examples.
- The library validates model definitions early (fail fast on invalid combinations).

---

#### PY-2 — Core operations parity (P0)

**Goal:** shipping-grade CRUD + conditions with typed results.

**Acceptance criteria**
- Put/Get/Update/Delete support conditional expressions (idempotency / optimistic concurrency use-cases).
- Errors map to typed exceptions with a consistent taxonomy (condition failed, validation, not found).
- Marshaling/unmarshaling is deterministic and round-trips correctly for supported types.

---

#### PY-3 — Query builder parity (P1)

**Goal:** match core query ergonomics without leaking raw expression strings.

**Acceptance criteria**
- Query + Scan support:
  - index selection (table vs GSI/LSI)
  - common operators (`=`, `<`, `<=`, `>`, `>=`, `between`, `begins_with`)
  - pagination (cursor in/out; deterministic ordering rules)
  - `limit`, projection/selection, optional consistent reads

---

#### PY-4 — Batch + transactions (P2)

**Goal:** unlock production patterns (bulk writes; atomic multi-item updates).

**Acceptance criteria**
- Batch get/write with partial-failure handling and retry semantics that are explicit and test-covered.
- Transaction write support with condition checks and clear error reporting.

---

#### PY-5 — Streams + events (P3)

**Goal:** parity with Go/TS stream parsing ergonomics.

**Acceptance criteria**
- Streams image unmarshalling (New/Old image) into typed models.
- Clear handling of missing/optional attributes and type mismatches.

---

#### PY-6 — `encrypted` semantics (P4)

**Goal:** enforce real encryption semantics, not metadata.

**Acceptance criteria**
- Envelope encryption via AWS KMS (mirrors `docs/development/planning/theorydb-encryption-tag-roadmap.md`):
  - fail closed when encrypted fields exist but KMS config is missing
  - encrypted fields rejected for PK/SK and indexes
  - encrypted fields not queryable/filterable
  - round-trip tests (write → read → decrypt)

---

#### PY-7 — Documentation + examples + first stable release (P0–P2)

**Goal:** make adoption copy/pasteable and reduce “AI-generated misuse”.

**Acceptance criteria**
- README + “Getting started” + core patterns equivalent to Go/TS docs (Lambda init, pagination, optimistic locking, batch, tx).
- Examples include local DynamoDB and AWS Lambda usage.
- A parity statement exists: which tiers/features match Go/TS and which are intentionally missing.

### Phase 3 — First-class parity (beyond P0–P4)

Phase 1/2 target P0–P4 behavioral parity. Phase 3 expands the scope to cover the broader Go surface area that makes
TableTheory “foundational” in real services: schema mgmt, full query/update DSL, runtime/ops helpers, security/hardening,
extensibility, and public testkits.

This phase is tracked by `docs/development/planning/theorydb-multilang-feature-parity-matrix.md`.

#### FC-0 — DMS-first schema workflow (author once)

**Goal:** models are defined once in DMS and consumed across all runtimes.

**Acceptance criteria**
- Services can choose a DMS-first workflow:
  - author schema in DMS (YAML/JSON)
  - load it at runtime and/or generate language-specific artifacts (codegen)
- A verifier ensures DMS fixtures are valid and used by:
  - contract-tests scenarios
  - at least one “real” deployable example (CDK stack)
- “Code-first” definitions (Go tags / TS builder / Py dataclasses) may still exist, but MUST be provably equivalent to DMS
  (either generated from DMS, or validated against DMS in CI).

---

#### FC-1 — Schema management parity (table helpers)

**Goal:** schema/table utilities are not Go-only.

**Acceptance criteria**
- TypeScript and Python provide first-class equivalents of:
  - create/ensure/delete/describe table
  - optional dev-only auto-migrate semantics (or a clearly documented non-support decision + alternative)
- Behaves consistently across DynamoDB Local and real AWS (idempotent and safe-by-default).

---

#### FC-2 — Query DSL parity (filters, groups, ordering, retries, parallel scan)

**Goal:** match Go’s query expressiveness without raw-string footguns.

**Acceptance criteria**
- TypeScript and Python support:
  - filter expressions and AND/OR filter groups
  - ordering/projection helpers and deterministic pagination behavior
  - eventual-consistency helpers (retry/backoff patterns) where relevant
  - parallel scan semantics for large-table workflows
- Contract + integration tests exist for drift-prone behaviors (filters/groups, projection escaping, cursor handoff).

---

#### FC-3 — Update DSL parity (UpdateBuilder)

**Goal:** expose Go’s update-builder semantics in TS/Py with the same safety rules.

**Acceptance criteria**
- TypeScript and Python support the UpdateBuilder feature class:
  - SET/REMOVE/ADD/DELETE, list ops (append/prepend/remove index/set element)
  - conditional updates, version helpers, return values
- Encrypted fields remain non-queryable and are rejected in update conditions consistently across languages.
- Contract tests pin update-expression construction and reserved word escaping.

---

#### FC-4 — Runtime/ops parity (Lambda + multi-account)

**Goal:** “serverless-first” ergonomics exist in all languages.

**Acceptance criteria**
- TypeScript and Python ship Lambda-focused defaults/helpers (connection reuse, timeouts, optional metrics).
- Multi-account assume-role patterns are supported (wrapper or documented recipe) without ad-hoc per-service code.

---

#### FC-5 — Security/hardening parity (validation + protection)

**Goal:** Go’s secure-by-default utilities are available across runtimes.

**Acceptance criteria**
- TypeScript and Python ship equivalents of Go’s:
  - naming/attribute validation and injection hardening
  - expression safety constraints (length/depth/operator allowlists)
  - resource protection utilities (rate limiting, concurrency limits) where relevant to each runtime
- Tests prove “fail closed” behavior for unsafe inputs and no sensitive data leakage in error messages.

---

#### FC-6 — Extensibility parity (custom converters + json/binary)

**Goal:** downstream services can represent domain-specific types consistently.

**Acceptance criteria**
- Each language exposes a stable custom-conversion hook (marshal/unmarshal) for non-primitive types.
- `json`/`binary` attribute modifiers are defined in DMS and implemented consistently across languages.

---

#### FC-7 — Public mocks/testkit parity (cheap testing for consumers)

**Goal:** testing is easy and consistent across runtimes.

**Acceptance criteria**
- Each language exposes a public, supported testkit for mocking:
  - DynamoDB calls used by TableTheory
  - KMS (or deterministic encryption provider)
  - time/clock and randomness sources (where applicable)
- Coverage for these modules meets the same repo-wide >=90% gate and is rubric-enforced.

---

#### FC-8 — Advanced helpers parity (aggregates + optimization)

**Goal:** keep “non-core but real-world” helpers from becoming Go-only drift points.

**Acceptance criteria**
- Aggregate helpers (sum/avg/min/max, group-by style helpers) exist in TS/Py with the same behavior class as Go.
  - If a helper is intentionally not supported in a language, that decision is explicitly documented and tracked in the
    feature parity matrix.
- Query optimization/planning helpers (if present) must be:
  - deterministic for a given query shape
  - advisory-only (must not change query semantics/results)
  - test-covered (unit + contract/integration where applicable)

## Key risks (and mitigations)

- **Drift across implementations:** mitigated by DMS + shared contract tests + pinned infra/tooling.
- **Type-system mismatch:** mitigate by defining what is runtime-validated vs compile-time-only per language.
- **Lambda performance regressions (TS):** mitigate with bundle-size budgets and cold-start benchmarks in CI.
- **Over-scoping parity:** mitigate with explicit parity tiers (ship P0/P1 first; expand only with tests).
- **Go-only creep:** mitigate by tracking “first-class scope” explicitly and requiring an explicit decision for any Go-only feature.
