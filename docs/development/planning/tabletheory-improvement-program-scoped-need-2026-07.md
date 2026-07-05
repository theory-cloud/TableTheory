<!-- AI Training: Internal planning document — scoped need for the 2026-07 product strengthening program -->

# Scoped Need: TableTheory Product Strengthening Program (2026-07)

**Status:** scoped and confirmed — decisions ratified by the maintainer 2026-07-02; enumerated in `tabletheory-improvement-program-enumerated-changes-2026-07.md`
**Source:** `docs/development/planning/tabletheory-product-improvement-assessment-2026-07.md` (baseline `staging`@74a9eb7, v1.10.0 stable / v1.10.1-rc.1)
**Maintainer decisions (2026-07-02):** planned **v2.0.0 capstone included** in the program; **release-lane restructure enumerated in-program**; parity fills in scope: ergonomic fills, `TransactGetItems`, **and TS/Py schema-migration parity**; Python import rename via deprecation shim. PartiQL remains refused (single-path violation).

## Background

A full six-surface assessment of TableTheory (Go/TS/Py runtimes, contract+DMS layer, docs/onboarding, CI/release machinery) identified eight improvement themes. The maintainer directed a comprehensive scope covering **all** findings. This document scopes the assessment's findings into one program of eight workstreams, each with its own contract-impact, compatibility, and parity verdicts, so a single enumerate-changes pass can produce the flat change list.

## Driver

The sole human maintainer, acting on the steward's assessment. Underlying pressure sources: external open-source consumers (adoption blockers: platform floors, install friction, type-safety gap), Pay Theory production and all downstream Theory Cloud products (correctness traps in the foundation), and the maintainer/steward loop itself (~50% of commit throughput consumed by release machinery).

## Problem

Catalogued in full in the assessment. Compressed: (1) four correctness traps ship in the foundation today; (2) all three runtimes expose dynamically-typed surfaces in ecosystems that expect inference, and published docs already over-claim type safety; (3) the contract pins 9 fixtures while query/GSI/pagination/batch/transactions/encryption/naming/type-matrix behavior is unpinned and the triplicated harness already diverges internally; (4) the DMS generates nothing and is verified in one runtime only; (5) platform floors (Py ≥3.14, Node ≥24 ESM-only) and install ergonomics block adoption; (6) docs mislead (non-existent APIs documented, stale org references) and undersell (fragmented trees, invisible examples); (7) no runtime offers a behavioral consumer-testing seam; (8) CI runs each suite ~2× per staging PR and contributors cannot pass the rubric locally.

## Affected runtimes

All three (Go, TypeScript, Python), plus the contract harness, DMS tooling, docs site, examples, CI workflows, and repo scripts. Workstream B contains runtime-specific ergonomics (Go generics are Go-only by nature); everything contract-visible lands in all three runtimes per release discipline.

## DynamoDB capability

Mixed by workstream. New DynamoDB capability exposure: `TransactGetItems`, `Select=COUNT`. Everything else is either correction of existing exposure (transactions, number marshaling), pinning of existing behavior (contract scenarios), or pure runtime/tooling/process concern ("none — pure runtime concern").

## Workstreams and contract impact

| WS | Name | Contract impact | Discipline |
|---|---|---|---|
| A | Correctness repairs (Transaction semantics/doc, Lambda init poisoning + race, TS big-number precision, Py union/storage-type fallbacks, nil-factory, harness ConsistentRead) | **New/extended P0 scenarios** for transaction atomicity semantics and big-number round-trip; rest additive runtime fixes | extend-contract-scenarios for the two scenario items |
| B | Typed API layers (Go generic layer, TS `defineModel` inference, Py typed roles + typed client) | **Additive runtime extension, no scenario** — ergonomics only; error-code timing coordinated across runtimes | standard pipeline |
| C | Contract coverage expansion (query op in 3 harnesses, P1 query/GSI/projection/pagination scenarios, type-matrix scenarios, `item_equals`/`cursor_equals`, cursor golden corpus, encryption interop scenario + Go driver error mapping, KEY-M1 Python evaluator, batch/tx P2 scenarios, `pascalCase` drift resolution) | **New P0/P1 scenarios throughout**; `pascalCase` resolution is a **DMS touch** | extend-contract-scenarios; **evolve-dms** for the naming-enum drift and any DMS v0.2 items |
| D | DMS leverage + consumer CLI (`tabletheory init/validate/gen`, model codegen ×3 languages, CDK generation, `FromMetadata`/equivalence ports to TS/Py, contract models generated not hand-written) | **No contract change** to semantics — tooling over the existing spec; generated models must round-trip the existing contract | standard pipeline |
| E | Distribution & floors (Py ≥3.12, Node ≥20 + `require` condition, AWS SDK→peerDeps, Py import rename via shim, Renovate datasource snippet, pip find-links page, dependabot.yml, version-discovery in docs) | **No contract change** — packaging/runtime support only | standard pipeline |
| F | Docs & onboarding truth-up (API references corrected then generated, CONTRIBUTING/examples repair, doc-tree unification, Examples nav, Go quickstart command, llms.txt + machine-readable vocabulary + consumer rules template, KB over-claims corrected) | **No contract change** | standard pipeline |
| G | Consumer testing (Go `NewWithClient`, in-memory fake validated against P0 corpus, ports/adapters in TS/Py, moto guidance, mock panic→assertion fixes) | **Additive runtime extension**; the fake is *validated by* the contract corpus, not a contract change itself | standard pipeline |
| H | Process & repo health (CI de-duplication, `make rubric-fast`, verifier consolidation, DynamoDB Local as services container, rubric-report path fix, make clean, error-taxonomy enrichment coordinated with C) | **No contract change** except error-taxonomy additions, which ride WS-C scenarios | standard pipeline |
| I | Release-lane restructure (single release-please manifest, generated version files at release-build time, merge queue, retirement of superseded guards) — **maintainer opted in** | **No contract change** — process only; high operational risk, runs between release cycles with design-doc-first discipline | standard pipeline + release watchpoints |
| J | v2.0.0 capstone (complete all queued deprecations, migration guide, downstream coordination) — **maintainer opted in** | Breaking by design; every removal pre-deprecated in 1.x | major release cycle |

Parity fills decided in scope: native `count()` (Select=COUNT), `get_or_none`/optional get, lazy result iteration (Py iterators / TS async iterators), `TransactGetItems`, and **TS/Py schema-migration parity** (AutoMigrate + field-transform story ported from Go — maintainer opted in; largest fill, additive runtime, no P0 scenario since it concerns table shape not item semantics). **PartiQL is refused** (second way to express queries; single-path violation).

## Backward compatibility

**Additive-first through 1.x, with a planned v2.0.0 capstone that completes the deprecations.** All feature work ships additive with deprecation warnings during 1.x; the program's final phase is a coordinated v2.0.0 major with migration notes and downstream coordination (AppTheory, FaceTheory, KnowledgeTheory, Autheory, theory-mcp-server, Pay Theory via the maintainer). The deprecate-in-1.x → remove-at-2.0 pairs:

- `DB.Transaction`/`TransactionFunc`: doc correction + deprecation warning in 1.x; **removed at v2 in favor of the existing atomic `Transact()` surface** (steward decision: keeping two transaction surfaces violates single-path; overridable — see Open Questions).
- Python import: `tabletheory_py` canonical alias added in 1.x with `theorydb_py` deprecated; old name removed at v2.
- TS root re-exports of FaceTheory/release-state/lease: subpath exports in 1.x with root deprecation; removed at v2.
- TS big-number precision: opt-in marshaling flag in 1.x; becomes the default at v2 (pinned by scenario in both modes).
- Go `any`-typed option variants and the test-only `MainExecutor`: deprecated in 1.x, removed at v2.

## Parity check

- WS-A/C/G and the parity fills: implementable cleanly in all three runtimes; the in-memory fake is per-runtime idiomatic but behavior-pinned by the shared P0 corpus.
- WS-B is deliberately runtime-idiomatic (Go generics / TS conditional types / Py typing) — ergonomics are not contract surface; what must stay identical is observable error codes and timing, which enumeration must call out.
- WS-D codegen must emit models that pass `AssertModelsEquivalent` in Go **and** the equivalent new gates in TS/Py — the ports are part of the workstream precisely so parity is checkable.
- Known awkwardness: none identified that forces reshaping; Python async remains out of scope (below) because it *would* be shape-distorting to rush.

## Success criteria (observable)

1. All four correctness traps have failing-before/passing-after tests; `DB.Transaction` doc no longer claims atomicity; Lambda init returns the cached error on every post-failure invocation (unit-testable); `-race` clean on the Lambda paths.
2. A TS consumer gets compile errors for a typo'd field name in `filter()`/`.set()` against a `defineModel` schema; a Go consumer can use a future `ModelOf[User]` handle bound to `db` with no `any` and no reflection error path; mypy flags a typo'd Python role constant.
3. `contract-tests/scenarios/` contains query/GSI/projection/pagination scenarios executed by all three runners; a non-integer number and an `NS`/`BS`/`B`/`BOOL`/`NULL`/`L` round-trip are asserted cross-runtime; `item_equals` and `cursor_equals` exist and are used; the Python KEY-M1 evaluator passes the shared fixture file; an item encrypted by Go is decrypted by TS and Py in a scenario run.
4. `tabletheory init` produces a directory that reaches a successful CRUD write against DynamoDB Local in one documented command per language; `tabletheory gen` emits Go/TS/Py models from a DMS file that pass the equivalence gates; the hand-written contract models are deleted in favor of generated ones.
5. `pip install` works on Python 3.12; `require()` works on Node 20; `import tabletheory_py` works with a deprecation-free path and `import theorydb_py` warns.
6. All three API references contain no signatures that differ from source (spot-audit gate); `CONTRIBUTING.md` contains zero `theorydb`-org references and sets up all three runtimes; the docs site renders TS and Py runtime trees; `llms.txt` exists at the site root.
7. A consumer test can assert data behavior (write-then-query) against the in-memory fake in each runtime without Docker; the fake passes the P0 scenario corpus.
8. A staging PR runs each language suite exactly once; `make rubric-fast` completes without Docker and is documented; the tracked rubric report diffs only when checks change.

## Nearest existing surface

Per workstream: A — `Transact()`/`pkg/transaction` is the correct transaction surface that already exists; B — `Table[T]`/`Page[T]` generics in Python and the TS testkit's typed command maps prove the patterns in-repo; C — the 9 P0 fixtures, runner capability gating, and the KEY-M1 Go/TS evaluators; D — `pkg/dms` validation + `FromMetadata`, `cmd/tabletheory-contract generate-ts`, and the runners' DMS→CreateTable translation (twice-implemented); E — the release pipeline already builds per-language assets; F — the Jekyll site, nav.yml, and the accurate `AGENTS.md`; G — `pkg/query.DynamoDBAPI` seam and the TS testkit; H — `SKIP_INTEGRATION` already honored by the verify scripts.

## Out of scope

- **Registry publication** (npm/PyPI) — refused by default per distribution policy; within-policy mitigations are in WS-E. Revisit only as an explicit maintainer policy decision.
- **PartiQL** — refused (single-path violation).
- **A second database backend, SQL idioms, softened fail-closed encryption, hidden DynamoDB limits** — identity guardrails, permanently out.
- **Python `AsyncTable`** — deferred; WS-F documents the executor pattern for async stacks instead. Rushing an async surface would distort the API shape.
- **Retiring dead Go internals** (`MainExecutor`, converter unification, TS builder de-dup) is IN scope but sequenced after WS-B to avoid double churn.

(Previously out of scope, now IN by maintainer decision 2026-07-02: the v2.0.0 capstone → WS-J; the release-lane restructure → WS-I; TS/Py schema-migration parity → parity fills.)

## Open questions

1. **Semver scope** — RESOLVED by maintainer: planned v2.0.0 capstone in-program (WS-J).
2. **Release-lane restructure** — RESOLVED by maintainer: enumerated in-program (WS-I), design-doc-first, executed between release cycles.
3. **TS/Py schema-migration parity** — RESOLVED by maintainer: in scope.
4. **Python naming** — RESOLVED by maintainer: rename via deprecation shim.
5. **DB.Transaction final disposition at v2** — steward decision taken: **remove** in favor of the existing atomic `Transact()` surface (single-path: two transaction surfaces is the exact two-ways smell the framework refuses). Overridable before WS-J executes; the alternative (make it atomic) keeps a duplicate surface.
6. **`pascalCase` naming drift** — still open at commit time: steward leans code-removal (emit only spec'd conventions) unless a consumer depends on it. The enumerated item carries a consumer check as its first acceptance condition.
