# TableTheory: Maintainability Roadmap (Rubric MAI)

Goal: reduce long-lived structural debt that makes a high-risk library hard to review, hard to change safely, and prone to “semantic drift” over time (especially under AI-assisted iteration).

This roadmap exists to make the maintainability gates in `docs/development/planning/theorydb-10of10-rubric.md` achievable and measurable.

## Baseline (start of MAI work)

Snapshot (2026-01-10):

- Largest production files (line count):
  - `theorydb.go`: **3726**
  - `pkg/query/query.go`: **1808**
  - `pkg/query/executor.go`: **1213**
  - `pkg/transaction/builder.go`: **1120**
- Query logic exists in more than one place (`theorydb.go` has a `query` implementation and `pkg/query` also implements a query builder/executor surface).

Snapshot (2026-01-11):

- Largest production files (line count):
  - `pkg/query/query.go`: **2409**
  - `pkg/query/executor.go`: **1262**
  - `query_executor.go`: **1186**
  - `pkg/transaction/builder.go`: **1153**
- Query logic is canonical in `pkg/query` (root `type query struct` removed; root package delegates via a thin executor layer).

Snapshot (2026-01-16):

- TypeScript implementation added (`ts/`).
- Largest production TypeScript files (line count):
  - `ts/src/client.ts`: **523**
  - `ts/src/query.ts`: **416**
  - `ts/src/model.ts`: **266**
  - `ts/src/encryption.ts`: **230**
- File-size budgets are enforced for both languages:
  - Go: `bash scripts/verify-go-file-size.sh` (max **2500**)
  - TypeScript: `bash scripts/verify-ts-file-size.sh` (max **1500**)

## Guardrails (keep refactors safe)

- Keep `make test-unit` and `make lint` green between milestones.
- Prefer small, mechanical moves (file/package splits) before behavior refactors.
- Preserve public APIs unless a change is explicitly planned and documented.

## Workstreams

### 1) File decomposition (remove “god files”)

Target: split large files into cohesive packages/files so review surface stays bounded and ownership is clearer.

Initial hotspot:
- `theorydb.go` (DB, query builder, metadata adapter, executors in one file)

### 2) Converge query semantics (one canonical path)

Target: choose one query implementation as canonical and make the other:
- a thin wrapper/delegator, or
- fully removed (with a deprecation window if public APIs require it).

### 3) Boundary hardening for future changes

Target: move “core” behavior behind stable interfaces and add focused tests around the boundaries that tend to drift:
- expression building
- marshaling/unmarshaling
- query compilation/execution

## Milestones (map to MAI rubric IDs)

### MAI-0 — Establish the roadmap (this document)

**Closes:** MAI-2 (once the verifier is wired)  
**Acceptance criteria**
- This document exists and includes: baseline, workstreams, and MAI milestones.

---

### MAI-1 — Enforce a file-size budget and shrink `theorydb.go`

**Closes:** MAI-1  
**Goal:** eliminate “god files” (starting with `theorydb.go`) so changes are reviewable.

**Acceptance criteria**
- `bash scripts/verify-file-size.sh` is green at the rubric budgets (Go + TypeScript).
- `theorydb.go` is split into cohesive files/packages (DB/session wiring vs query vs adapters vs executors).

---

### MAI-2 — Keep the maintainability plan current

**Closes:** MAI-2  
**Goal:** keep a current hotspot/convergence plan as the code evolves.

**Acceptance criteria**
- `bash scripts/verify-maintainability-roadmap.sh` is green.
- Baseline snapshot is updated when major refactors land (line counts + convergence status).

---

### MAI-3 — One canonical Query implementation

**Closes:** MAI-3  
**Goal:** avoid parallel semantics drift by having a single canonical query builder/executor surface.

**Acceptance criteria**
- `bash scripts/verify-query-singleton.sh` is green.
- The non-canonical query path is either removed or reduced to a documented delegator layer.
- Behavior parity is covered by unit tests around the public `core.Query` interface.

## Helpful commands

```bash
# Should be green after MAI-1/MAI-3
bash scripts/verify-file-size.sh
bash scripts/verify-query-singleton.sh

# Should become green early and stay green
bash scripts/verify-maintainability-roadmap.sh
```
