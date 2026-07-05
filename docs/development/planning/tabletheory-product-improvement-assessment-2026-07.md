<!-- AI Training: Internal planning document — product improvement assessment, July 2026 -->

# TableTheory Product Improvement Assessment — July 2026

**Status:** proposal / assessment (no code changes)
**Baseline:** stable `v1.10.0`, prerelease `v1.10.1-rc.1`, branch `staging`
**Scope:** all three runtimes (Go, TypeScript, Python), contract-tests + DMS, docs/examples/onboarding, CI/release machinery
**Method:** six parallel deep-read investigations of the repository (one per surface), synthesized and filtered through TableTheory's identity constraints (DynamoDB-first, three peer runtimes, contract-first parity, fail-closed encryption, immutable GitHub Releases). File:line references are to the `staging` working tree at the baseline above.

---

## Executive summary

TableTheory's core is genuinely strong: the feature surface is mature in all three runtimes (CRUD, query/scan, batch with unprocessed-item retry, transactions, GSI/LSI, cursor pagination, fail-closed KMS encryption, TTL, optimistic locking, leases, write policies), code hygiene is high (no TODOs/stubs in shipped paths, strict lint/typecheck gates, ~90% coverage in Python, zero `any` in TS internals), and the contract-test + DMS architecture is a real differentiator no competitor has.

The product's weaknesses cluster into eight themes, in descending order of consequence:

1. **A small number of real correctness traps** — most seriously, Go's `DB.Transaction` is documented as a transaction but performs independent non-atomic writes, and a failed Lambda cold-start init can poison later invocations with a `(nil, nil)` return.
2. **A type-safety gap in every runtime.** Go exposes an `any`-typed, stringly-operated API with zero generics; TS's `defineModel` produces no type inference (everything is `Record<string, unknown>`); Python's role declarations are runtime-only strings. Competing libraries (ElectroDB, dynamodb-toolbox, PynamoDB) win precisely here.
3. **The contract pins far less than the product promises.** The P0 set covers 9 fixtures; query semantics, GSIs, pagination, batches, generic transactions, encryption interop, naming strategies, and most of the type matrix are unpinned. The harness itself is triplicated and already diverges internally.
4. **The DMS is under-leveraged.** It is consumed at runtime only by TS, reverse-verified only in Go, and generates nothing. Forward codegen (models in three languages, CDK constructs) would convert the spec from a verification artifact into the product's engine.
5. **Distribution and platform floors block adoption.** Python requires `>=3.14` when the code needs only 3.12; TS is ESM-only on Node ≥24; the tarball/wheel install path defeats semver ranges and update bots; Python's install name and import name differ.
6. **Docs undersell and mislead simultaneously.** Three fragmented doc trees (only the Go-centric one published), API references that document non-existent parameters, stale `CONTRIBUTING.md` pointing at the retired `theorydb` org, examples that are invisible from the docs site and whose quick-start commands fail.
7. **Consumer testing is the weakest usability area in all three runtimes.** No injectable client seam in Go, no stateful in-memory fake anywhere, brittle ordered-expectation mocks in Python.
8. **Process weight is consuming the project.** ~50% of all commits touch release/hygiene machinery; ~11,600 LOC of governance/CI/release tooling (vs ~59,800 LOC of product source); the 1.10.1 patch took four RCs and ~8 PRs; every staging PR runs each language suite roughly twice.

The prioritized roadmap is at the end. The single highest-leverage sequence: **fix the correctness traps (Part I) → build the typed API layers additively (Part II) → extend the contract to query/GSI + type matrix (Part III) → ship DMS codegen + a consumer CLI (Part IV) → lower the platform floors and truth-up the docs (Parts V–VI).**

---

## Part I — Correctness and safety issues to fix first

These are ranked above every feature because TableTheory is the floor of the stack: a correctness trap here is inherited by every downstream consumer.

### I.1 `DB.Transaction` is not a transaction (Go) — severity: critical

The `core.DB` interface documents `Transaction(fn func(tx any) error)` as "executes a function within a database transaction" (`pkg/core/interfaces.go:21`), but the implementation is a non-atomic pass-through: `tx.SetDB(db)` then `fn(tx)`, where `Tx.Create/Update/Delete` issue **independent writes** (`internal/theorydb/theorydb.go:245-252`, `pkg/core/interfaces.go:302-329`). A consumer reasonably expecting atomicity gets N separate `PutItem`s. A comment in the code admits it "doesn't support full transaction features."

**Fix path (semver-aware):** this cannot silently change behavior post-1.0. Recommend: (a) immediately correct the doc comment and add a loud deprecation notice pointing to `Transact()` / `TransactWrite`, (b) in the next major, either make it atomic via `TransactWriteItems` or remove it. Ship a P0 contract scenario for generic transactional atomicity at the same time (see Part III) so all three runtimes pin the corrected semantics.

### I.2 Lambda cold-start init poisoning (Go) — severity: high

`NewLambdaOptimized` (`internal/theorydb/lambda.go:89-101`): if `createLambdaDB()` errors, the error is returned to the *first* caller only. `lambdaOnce.Do` will not re-run, and subsequent invocations receive **`(nil, nil)`** — a nil DB with no error, deferring failure to a nil dereference in the handler. The once-guarded init must cache and re-return the error (or reset the `sync.Once` to allow retry on next cold path).

Related: `OptimizeForMemory()` writes `ldb.db.lambdaTimeoutBuffer` without the mutex (`lambda.go:307-309`) while `WithLambdaTimeout` reads it under `RLock` (`lambda.go:218-224`) — a data race under concurrent invocations that `-race` would flag.

### I.3 Numeric precision loss (TypeScript) — severity: high, parity-relevant

`unmarshalScalar` for `N` always coerces through `Number(av.N)` (`ts/src/marshal.ts:299`; document values at `:556`). DynamoDB numbers carry up to 38 digits; values beyond 2^53 silently lose precision **in TS only** — Go and Python preserve exactness. This is latent cross-runtime data divergence in a framework whose whole thesis is that the runtimes never diverge. Fix by preserving `N` as string/bigint above the safe-integer bound (opt-in strategy flag if needed for compatibility), and pin the behavior with a big-number contract scenario (currently impossible to express — see Part III on the runner's `asInt64`-only number assertion).

### I.4 Client-side aggregations masquerading as query features (TS + Python) — severity: medium-high

TS `query.sum/average/min/max/aggregate/countDistinct/groupBy` all call `.all()` and materialize **every matching item** into memory (`ts/src/query.ts:544-590`, `ts/src/aggregates.ts`); Python's `aggregates.py` operates the same way, and Python additionally has **no lazy pagination at all** — `query_all`/`scan_all` build full lists (`py/src/theorydb_py/table.py:311-344, 436-467`). On large partitions this is an OOM and cost footgun that reads like server-side capability. Fixes: native `Select=COUNT` count paths, streaming iterators (`query_iter`/`scan_iter` in Python; async-iterator page streams in TS), and explicit "client-side, loads all rows" labeling in docs and doc comments.

### I.5 Silent type fallbacks (Python) — severity: medium

`unwrap_optional` only unwraps exactly-two-member unions (`py/src/theorydb_py/attr_types.py:23`), so `int | str | None` silently falls through to `"S"` storage. Additionally `streams.py:118` resolves storage type via `attr_def.storage_type or "S"` while `table.py:159-168` uses the richer `_attribute_storage_type` — two read paths that can disagree. Make unsupported unions a **model error** (fail loudly at `from_dataclass`), and unify storage-type resolution.

### I.6 Smaller traps worth closing in the same pass

- `DefaultDBFactory.CreateDB` returns `nil, nil` (`pkg/testing/factory.go:27-33`) — a public factory that silently yields a nil DB.
- Go mocks panic on type mismatches rather than failing assertions (`pkg/mocks/query.go:15-76`).
- The contract harness itself diverges: TS raw reads use `ConsistentRead: true` (`contract-tests/runners/ts/src/runner.ts:328`) while Go raw reads default to eventual consistency (`runner.go:329-332`). Benign on DynamoDB Local, but the "same assertion everywhere" premise should hold literally.
- `examples/README.md:240-263` documents a non-existent API (`tabletheory.New(tabletheory.WithLambdaOptimizations())` — neither the option nor the single-return signature exists; the real surface is `NewLambdaOptimized()` / `LambdaInit()`, `tabletheory.go:100,120`).
- Python's `docs/api-reference.md` documents a `put(..., if_not_exists=False)` parameter that does not exist (`table.py:810-817`), the wrong `transact_write` parameter name, and 4 of 9 real `query` parameters.

---

## Part II — The type-safety gap (the biggest competitive weakness)

All three runtimes expose dynamically-typed surfaces in languages whose ecosystems have moved to inference-first data layers. This is the most common first impression an evaluating engineer will form, and right now it loses to ElectroDB/dynamodb-toolbox (TS) and PynamoDB/pydantic idioms (Python) on day one.

**Everything in this part can be additive (no major bump):** new generic layers alongside the existing API, deprecation later if desired.

### II.1 Go: an `any`-typed API on Go 1.26

- Zero generics anywhere in `pkg/` or `tabletheory.go`. Every read requires `var x []Model` + pointer, checked by reflection at runtime (`"destination must be a pointer to a struct"`, `pkg/query/query.go:645-651`).
- Options parameters that document their real types in comments: `AutoMigrateWithOptions(model any, opts ...any)` ("opts should be of type schema.AutoMigrateOption", `pkg/core/interfaces.go:43`), `CreateTable(model any, opts ...any)` (`:51`), `DescribeTable(model any) (any, error)` ("Returns *types.TableDescription", `:61`), `TransactionFunc(fn func(tx any) error)` (`:71`). These should take concrete types — that alone restores IDE autocomplete.
- Stringly-typed operators: `Where(field, op string, value any)` with validity decided at execution by a runtime switch (`internal/expr/builder.go:583-691`). Exported typed operator constants (or `.WhereBeginsWith(...)`-style methods) move typos to compile time. `BETWEEN` requiring `value` as `[]any{lo, hi}` (`builder.go:626-635`) deserves a two-argument form.
- Field references are Go field names as strings; a struct rename silently breaks queries.

**Recommendation:** an additive generic layer — a `tabletheory.ModelOf` handle parameterized by `T` and bound to `db`, returning `Query[T]` with `First() (T, error)` / `All() ([]T, error)`, typed operator constants, concrete option types. Existing `any` API stays for compatibility.

### II.2 TypeScript: `defineModel` erases the type it just described

`defineModel(schema)` returns a non-generic `Model` (`ts/src/model.ts:80-88, 101`), so `get()` returns `Record<string, unknown>` (`ts/src/client.ts:193`), `filter(field: string, op: string, ...)` accepts any string (`ts/src/query.ts:343`), and `.set(field: string, value: unknown)` type-checks nothing (`ts/src/update-builder.ts:292`). The README's "typed DynamoDB access layer" claim (`ts/README.md:15`) is not yet true in the way TS users will read it.

**Recommendation (highest single TS lever):** `defineModel<const S extends ModelSchema>(schema: S): Model<InferItem<S>>` deriving the item type from the attributes array; typed registration handles (`db.model(User)` → typed repository) so the string-keyed registry (`ts/src/client.ts:100`) stops erasing types; operator unions instead of `op: string` (the runtime already enumerates them at `ts/src/query.ts:160-252`).

### II.3 Python: runtime-only strings where type checkers could help

- Roles are plain strings (`roles=["pk"]`) validated only at model build (`py/src/theorydb_py/model.py:249-261`); `theorydb_field()` returns `Any` (`model.py:122-173`), so the checker sees nothing.
- The client boundary is `client: Any` (`table.py:111,125`) even though `boto3-stubs[dynamodb]` is already a dev dependency (`py/pyproject.toml:23`).

**Recommendation:** typed role constants (e.g., `Role.PK`) accepted alongside strings, `DynamoDBClient`-typed client parameter with `boto3-stubs` as a runtime extra, and earlier role validation. Note the generics that already exist (`Table[T]`, `Page[T]`) are good — this is about the model-declaration boundary.

### II.4 Contract implication

Typed layers are runtime-local ergonomics (no DMS change needed), **but** any observable behavior they introduce (e.g., a new error for invalid operators at build time vs execution time) must stay consistent with the shared error taxonomy. Plan the three typed layers together so error timing/codes don't drift.

---

## Part III — Contract coverage vs. the parity promise

The marketing claim is "verified on every commit." The verified surface is 9 P0 fixtures (`contract-tests/scenarios/p0/01-…09-…`): CRUD, omit-empty, lifecycle timestamps, optimistic locking, TTL, and four release-state fixtures. Everything else is per-runtime tests that can silently disagree. This is the gap most likely to produce the exact failure mode the framework exists to prevent.

### III.1 Unpinned observable behavior (ranked by drift risk)

| Surface | Status |
|---|---|
| **Query/scan semantics** | No `query`/`scan` op exists in any runner (`contract-tests/runners/go/internal/scenario/scenario.go:89-128`). Operators, sort direction, limit, consistent read: all unpinned. |
| **GSI behavior** | GSIs are declared in fixtures (`user.yml:212`, `order.yml:50`) and physically created by runners, but **no scenario ever reads through an index**. The `Order` model exists with zero scenarios referencing it. |
| **Pagination/cursors** | One golden pair only (`contract-tests/golden/cursor/`) covering string keys + one index + ASC. No `N`/`B` keys, no DESC, no no-index case. `cursor_equals` is documented (`theorydb-contract-tests-suite-outline.md:209`) but **not implemented**. |
| **Batch limits/partial failure** | Nothing. 100-item get / 25-item write / unprocessed retry semantics unpinned (listed as P2 in the outline, never built). |
| **Generic transactions** | Only the specialized `transition_append_event`. No generic TransactWrite scenario, no cancellation-reason mapping. |
| **Encryption** | No shared scenario at all; per-language tests diverge; Go's driver doesn't even map the encryption error codes (`runners/go/internal/driver/driver.go:59-80`) that the TS driver declares (`driver.ts:16-18`). No cross-runtime interop check (Go-encrypt → TS/Py-decrypt). |
| **Naming strategies** | DMS defines `camelCase | snake_case | dynamorm` (`theorydb-spec-dms-v0.1.md:175-178`); every contract model uses `camelCase`. Also: `pkg/dms/dms.go:374-386` emits a `pascalCase` convention **not in the spec enum** — spec/code drift today. |
| **Type matrix** | `NS`/`BS` never used anywhere; binary `B` never declared; `BOOL`/`NULL`/`L` never round-tripped; non-integer numbers **cannot be asserted** because runners coerce `N` through `asInt64` (`runner.go:236-239`, `runner.ts:255-256`); no empty-string persistence case; `item_equals` (exact whole-item match) documented but unimplemented, so unexpected extra attributes are never caught. |
| **Error taxonomy** | Only 5 of the declared error codes are ever scenario-asserted. |

### III.2 Structural weaknesses in the harness

- **Triplicated harness logic.** Fixtures are shared, but `recreateTable`/`runStep`/`assertExpectation` are reimplemented in Go, TS, and a single 624-line Python file. Adding one op (e.g., `query`) is a 3× parallel change; the harness itself is a drift surface (the `ConsistentRead` divergence in Part I.6 is the proof).
- **Models are hand-authored three times** (Go struct `driver.go:385-396`, Python dataclass `test_contract_p0.py:63-73`, TS via DMS). Only TS consumes the DMS directly.
- **KEY-M1 has no Python implementation.** The derived-key contract ships a Go evaluator + TS evaluator + generated TS + fixture tests, but no `evaluate_derived_key` exists anywhere under `py/src` — a straight parity hole in the newest contract surface.
- No Makefile target for contract tests despite `docs/reference/contract-scenarios.md:87` instructing `make contract-tests`; the `extend-contract-scenarios` workflow exists only as prose.

### III.3 Recommendations (ordered)

1. **Add the `query` op to all three harnesses and ship the P1 query/GSI/projection scenarios.** This is the one expensive-but-foundational harness change; everything else in the P1 list from the outline (`theorydb-contract-tests-suite-outline.md:276-290`) becomes cheap YAML afterward.
2. **Fix the runner number assertion** (drop `asInt64`-only coercion) then close the type-matrix holes with YAML-only scenarios: `NS`/`BS`, `B`, `BOOL`/`NULL`/`L`, floats/big numbers, empty-string persistence.
3. **Implement `item_equals` and `cursor_equals`**, reconcile `ConsistentRead`, and widen the cursor golden corpus (N/B keys, DESC, no-index, single-key).
4. **Add a cross-runtime encryption interop scenario** (encrypt in one runtime, decrypt in the other two against shared KMS-stubbed material) and map encryption error codes in the Go driver.
5. **Bring Python to KEY-M1 parity** (evaluator + fixture test); extend the key-contract transform vocabulary (lowercase, URL-encode — `CanonicalBindingKey` already pre-encodes `%2F%2A` by hand in fixtures, `theorymcp-derived-keys.yml:350`, which is demand evidence).
6. **Pin batch and generic-transaction semantics** as P2 scenarios once query lands.
7. Resolve the `pascalCase` spec/code drift through the `evolve-dms` discipline (either add it to the DMS enum or stop emitting it).

---

## Part IV — DMS: from verification artifact to product engine

Today the DMS is: consumed at runtime by TS only (`ts/src/dms.ts:42-44`), reverse-verified against code in Go only (`pkg/dms/dms.go:128-227`, exercised for exactly one example model by `scripts/internal/dms_first_workflow/main.go:116-149`), and not verified at all in Python. Meanwhile the only CLI in the repo (`cmd/tabletheory-contract`, single `generate-ts` subcommand) is internal key-contract tooling.

This is the largest untapped strategic asset. Concrete build-out, in order:

1. **Forward model codegen.** `tabletheory gen` emitting Go structs, TS `defineModel` schemas, and Python dataclasses from a DMS file. Immediately eliminates the triple-hand-authored contract models (Part III.2), and for consumers it *is* the drift-prevention story made tangible: author the model once, generate three runtimes. The generator scaffold already exists for key contracts.
2. **DMS validation as a consumer command.** `tabletheory validate mymodels.yml` — the validator already exists in Go (`pkg/dms/dms.go:229-245, 518-634`); it needs a CLI wrapper and error messages aimed at consumers.
3. **Port `FromMetadata`/`AssertModelsEquivalent` to TS and Python** so all three runtimes can gate "code matches DMS," not just Go.
4. **Infra generation.** The DMS→`CreateTable` translation (keys, GSIs, projections, TTL) is already implemented twice inside the contract runners (`runner.go:368-483`, `runner.ts:422-496`). Productize it: `tabletheory gen --cdk` emitting a CDK `Table` construct (and/or Terraform). The `examples/cdk-multilang` demo's hand-written tables are only *asserted* equivalent today.
5. **Scaffolding.** `tabletheory init` — working model + DMS file + DynamoDB-Local docker-compose + one CRUD program per chosen language. This collapses time-to-first-success (Part VI) and is the natural home for the generative-coding artifacts (Part VI.4).

DMS v0.2 planning should also close the spec silences that consumers can observe: general `N` precision/canonicalization for stored items (the derived-key sidecar already has the rigorous rule, `tabletheory-model-contract-v0.1.md:99-103` — item storage doesn't), query/filter semantics, batch/transaction limits, and the dead `index_pk:<name>`/`index_sk:<name>` roles that no tooling emits.

---

## Part V — Adoption and distribution friction

### V.1 Platform floors (pure wins, no policy conflict)

- **Python `requires-python = ">=3.14"` (`py/pyproject.toml:9`) is the single highest-ROI change in the repo.** The actual language floor in the code is 3.12 (PEP 695 generics); nothing needs 3.13+. Pinning to 3.14 excludes essentially every production Python deployment in 2026. Lower to `>=3.12` (keep CI on 3.14 + add a 3.12 test lane).
- **TS Node `>=24` engines floor + ESM-only exports** (no `require` condition in `ts/package.json`) exclude Node 20/22 LTS users and CJS codebases. Lower the floor to 20 and add a `require` export condition or dual build.
- **Move `@aws-sdk/*` to `peerDependencies`** so consumers dedupe their own SDK version; note the `overrides` block pinning transitive deps is not inherited by consumers of the tarball.

### V.2 Distribution model — reduce the cost without breaking the policy

GitHub-Releases-only distribution is a deliberate, identity-level decision (single path, immutability, cross-language version alignment). Proposals to "also publish to npm/PyPI for convenience" remain refused by default. But the policy's costs are real and currently unmitigated:

- No semver ranges; upgrades are manual URL edits.
- **No update automation:** no `dependabot.yml`/`renovate.json` exists in this repo, and URL-tarball deps are invisible to standard bots in consumer repos.
- Docs show literal `vX.Y.Z` placeholders with no version-discovery help (`ts/docs/getting-started.md`, `docs/runtimes/typescript.md`).
- **Python installs `tabletheory-py` but imports `theorydb_py`** (`py/pyproject.toml:6` vs `py/src/theorydb_py/`) — a hard stop for newcomers and a discovery problem forever.

**Within-policy mitigations (recommended):**

1. Ship a documented **Renovate custom-datasource config snippet** (github-releases datasource) that consumers copy into their repos so TableTheory tarball/wheel pins update automatically. This restores bot coverage without a registry.
2. Add a **"copy latest install command" widget** (or at minimum a small redirect endpoint / `latest` asset alias) on the docs runtime pages.
3. Publish a **`--find-links` style index page** for Python generated from Releases, enabling `pip install tabletheory-py --find-links <url>` with real version resolution — still GitHub-hosted, still immutable.
4. **Reconcile the Python package/import name** — either rename the import package to `tabletheory_py` through a deprecation cycle (re-export shim from `theorydb_py`, remove in next major) or, at minimum, put the mismatch in a callout at the top of every Python install doc.
5. Add a `dependabot.yml` to **this** repo (gomod, npm, pip, github-actions) so dependency updates stop arriving as reactive security-alert batches.

If external adoption becomes the top strategic goal, registry publication is a maintainer-level policy revisit — the honest framing is that it trades the single-path alignment invariant for standard ecosystem mechanics. This document does not recommend it; it recommends items 1–5, which capture most of the value.

### V.3 Public API surface hygiene (TS)

`ts/src/index.ts:22-24` exports FaceTheory ISR stores, release-state transitions, and lease management from the root namespace. For an external consumer these read as unexplained coupling. Move to subpath exports (`@theory-cloud/tabletheory-ts/facetheory`, `/release-state`) — additive, with root re-exports deprecated until the next major.

---

## Part VI — Documentation and onboarding

### VI.1 Structural: one published tree, two shadow trees

`docs/` (published via Jekyll) is Go-centric; `ts/docs/` and `py/docs/` are unrendered GitHub-blob shadow trees at a fraction of the depth (API reference: 547 lines Go vs 141 TS vs 101 Py; troubleshooting: 286 vs 37 vs 32). A TS/Python consumer falls off the docs site the moment they need runtime detail. **Fix:** publish the runtime trees into the site (per-runtime nav sections or runtime tabs), and bring TS/Py troubleshooting + API reference toward parity. Longer-term, generate API references from source (godoc/TypeDoc/pdoc) — all three are hand-written today, which is the root cause of the drift catalogued in Parts I and V.

### VI.2 Staleness with first-impression damage (cheap, do immediately)

- `CONTRIBUTING.md` points at the retired org everywhere (`theorydb.git`, `conduct@theorydb.io`, `discord.gg/theorydb`, `github.com/theorydb/theorydb/issues`), sets up **only Go** for a three-runtime repo, and says Go 1.21+ (repo is 1.26). The accurate contributor doc is `AGENTS.md`; port its reality into `CONTRIBUTING.md`.
- `examples/README.md`: wrong clone path (`cd theorydb/examples`), Go 1.21+, quick-start commands (`make run`) that fail because `basic/todo|notes|contacts` have no Makefiles, and the non-existent `WithLambdaOptimizations` API.
- Go toolchain version is stated four different ways across `go.mod` (1.26/1.26.4), `docs/getting-started.md:18` (1.25+), `CONTRIBUTING.md:70` and `examples/README.md:108` (1.21+).

### VI.3 Time-to-first-success

TS has the best story (2 commands: `make docker-up` + `npm --prefix ts run example:local`). Python has a runnable example (`py/examples/local_crud.py`) that its getting-started never tells you to run. **Go — the primary runtime — has no single documented command to a working CRUD write:** `docs/getting-started.md` never mentions `make docker-up`, its verification snippet is a fragment requiring a table that nothing created, and no `make example` target exists. Add a Go `example:local` equivalent + one documented command. The examples collection (7 Go app-shaped examples) is invisible from the docs site — `docs/_data/nav.yml` has no Examples group — and TS/Python have no app-shaped examples at all.

Also: the flagship `cdk-multilang` demo — the only place the cross-language no-drift claim is demonstrable — requires a paid AWS account (KMS + S3 Glacier). A DynamoDB-Local variant would make the headline claim evaluable for free.

### VI.4 The generative-coding claim needs shipped artifacts

The README leans on "generative-coding friendly," and the constrained single-path API genuinely is — but nothing concrete ships for a *consumer's* AI tooling: no `llms.txt`/`llms-full.txt`, no machine-readable tag/role vocabulary, no copy-in rules template (`CLAUDE.md`/`.cursorrules` for consumer projects), and the LLM FAQ (`docs/llm-faq/module-faq.md`) is Go-only. The repo's own rich agent artifacts are all steward-facing. Ship: `llms.txt` linking into the docs; a machine-readable DMS + tag vocabulary export; a consumer rules template; prompt recipes ("generate a TableTheory model + handler"). These pair naturally with `tabletheory init` (Part IV).

---

## Part VII — Consumer testing story

The weakest usability area in all three runtimes, and the most common reason a downstream team "reaches around" an ORM.

- **Go:** consumers cannot inject a DynamoDB client — `New(config)` builds a concrete `*dynamodb.Client` (`internal/theorydb/theorydb.go:132-155`) with no constructor accepting the already-defined `DynamoDBAPI` seam (`pkg/query/executor.go:27-41`). Interface mocks (`pkg/mocks`) replay canned returns without executing query semantics, so tests assert mock wiring, not behavior. **Highest-leverage fix: `NewWithClient(DynamoDBAPI)` + a state-backed in-memory fake** honoring keys/conditions/filters.
- **TypeScript:** the `testkit` subpath is genuinely good (strict typed command mocks, deterministic clock + encryption provider, `ts/src/testkit/index.ts:40-221`) but stops short of a stateful table fake, so tests assert marshaled command input rather than data behavior.
- **Python:** the only mock is a strict FIFO ordered-expectation fake (`py/src/theorydb_py/mocks.py:69-86`) — every call must be pre-scripted in order; `testkit.py` is 33 lines; the testing guide never mentions moto or offers a stateful alternative.

**Recommendation:** build the in-memory fake once as a shared behavioral spec (it can itself be validated against the P0 scenarios — the fake passing the contract suite is the quality bar) and implement per runtime. This converts the contract corpus into a consumer-facing asset.

---

## Part VIII — Feature parity matrix and gaps

Peer runtimes, but the surfaces have drifted in shape and coverage (Go ~44.6k LOC vs TS ~8.8k vs Py ~6.4k):

| Capability | Go | TS | Python |
|---|---|---|---|
| Schema migration/transforms (`AutoMigrate`, field ops, backup/data-copy) | ✅ (`pkg/schema`) | ❌ (create/ensure/delete table only, `ts/src/schema.ts`) | partial |
| Key-contract / derived keys (KEY-M1) | ✅ evaluator + generator | ✅ evaluator + generated helpers | ❌ **absent** |
| Query optimizer | ✅ real index selector (`pkg/index`) | static string hints misleadingly named `QueryOptimizer` (`ts/src/optimizer.ts:54`) | `QueryOptimizer` exported, undocumented |
| Lazy result iteration | ✅ | partial (pages) | ❌ (lists only) |
| Async model | n/a (goroutines) | native | ❌ no async client (sync boto3 + threads only) |
| Stream helpers | unmarshal-only | unmarshal-only (55 lines) | unmarshal + example handler |
| `TransactGetItems` | ❌ | ❌ | ❌ |
| PartiQL | ❌ | ❌ | ❌ |
| Native `count()` (Select=COUNT) | ❌ | ❌ | ❌ |
| `get_or_none` / optional get | ❌ | ❌ | ❌ (`get` raises `NotFoundError`, `table.py:857`) |
| DMS code↔spec equivalence gate | ✅ (one model) | ❌ | ❌ |

Parity decisions to make explicitly (rather than by accretion): does TS/Py get the migration story? Does the framework want PartiQL and TransactGet at all (a legitimate "no" is fine — but write it down)? Each "yes" is a contract change: DMS coverage + scenario + three implementations in the same release. Also rename or realize the TS `QueryOptimizer` — a name that over-promises is drift in vocabulary.

Internal code-health items (no behavior change, reduces future drift):

- Go: two full execution stacks — `pkg/query/executor.go` (`MainExecutor`, ~45 KB) is referenced only by its own coverage tests while production uses `internal/theorydb/query_executor.go`; two Go↔AttributeValue converter stacks (`pkg/types/converter.go` vs `internal/expr/converter.go`); `anonymous_embed_encoding.go` triplicated across three packages; `pkg/query/query.go` at 68 KB; `cov4_/cov5_/cov6_/cov7_*` coverage-chasing test sprawl anchoring the dead executor in place.
- TS: operator-switch and pagination/aggregation logic duplicated between QueryBuilder/ScanBuilder and between query/update builders (`ts/src/query.ts:154-254` vs `ts/src/update-builder.ts:140-238`; `query.ts:544-651` vs `:930-1152`); ESLint is not type-aware (`recommended` not `recommended-type-checked`).
- Python: `_coerce_value`/`unwrap_optional` copy-pasted in four modules; AWS-error branches excluded from unit coverage via `# pragma: no cover`.
- Go tag DX: validation errors omit the offending field name (`pkg/model/registry.go:355-358`); naive default pluralization for table names (`:711-722`); two mechanisms for LSI detection (`lsi:` tag and `lsi-` name prefix, `:725-730`); a stray `naming:` on any field silently changes table-wide attribute naming (`:781-812`).
- Go error taxonomy: version-conflict indistinguishable from any condition failure (both `ErrConditionFailed`, `pkg/query/executor.go:390-391`); no `ErrThrottled`/`ErrTransactionConflict`; string-matching classification (`strings.Contains(err.Error(), "ConditionalCheckFailed")`, `executor.go:524`, `pkg/transaction/transaction.go:424-430`) while `pkg/transaction/builder.go:857-913` correctly uses `errors.As` — two classification regimes in one package tree. Adding error codes is contract-relevant: coordinate with the taxonomy in the DMS and pin with scenarios.

---

## Part IX — Process weight and repo health

The numbers: ~11,600 LOC of governance/CI/release machinery vs ~59,800 LOC of product source; 75 shell scripts (7,764 LOC), 52 of them `verify-*`; 13 workflows; **254 of 508 commits (~50%) touch the release/hygiene surface**; the 1.10.1 patch consumed rc.1→rc.4 and ~8 PRs, with duplicate repair commits in the log; the changelog's recent "bug fixes" are almost entirely release-machinery repairs. The two-party maintainer/steward loop is spending half its throughput feeding the release lane instead of the product.

Recommended, in order of leverage:

1. **De-duplicate CI on staging PRs.** `quality-gates.yml`'s rubric re-runs the full Go/TS/Py unit+integration matrix that `python.yml`, `typescript.yml`, and `unit-cover.yml` already ran — each language suite executes ~2× and DynamoDB Local is stood up ~3× per PR. Retire the standalone language workflows as PR gates (keep on push), or make the rubric skip what they cover.
2. **Simplify the release lane.** The dual cross-wired release-please configs (stable config writes the premain manifest as an extra-file) + 5 hand-synced version-alignment points + ~3,600 LOC of bash guards is the machine that keeps breaking. Direction: single manifest with release-please's native prerelease handling; make the tag/`go.mod` the sole version source and *generate* `ts/package.json`/`py/version.json` versions at release-build time instead of hand-syncing five copies; prefer a merge queue over human-promotion provenance guards. This is a project of its own — but every month deferred costs ~half the commit budget.
3. **Consolidate the verifier sprawl:** 5 coverage scripts, 4 file-size scripts, `verify-formatting.sh` vs `fmt-check.sh`, `verify-version-alignment` vs `verify-branch-version-sync`, `verify-release-cycle-state` vs `watch-release-cycle` (512 LOC) — fold into parameterized helpers.
4. **Give contributors a fast path.** `make rubric` requires the entire internal toolchain (Go 1.26, Py 3.14, Node 24, uv, golangci-lint, govulncheck, gosec, ripgrep, Docker) and plausibly runs 15–30 min; `SKIP_INTEGRATION` exists in the scripts but is surfaced nowhere. Add `make rubric-fast` (lint + unit + doc gates, no integration/subtree) and document it in the rewritten `CONTRIBUTING.md`. Today an external contributor cannot realistically go green locally.
5. **Repo hygiene:** the tracked `gov-infra/evidence/gov-rubric-report.json` embeds machine-specific absolute paths (`/home/aron/...`) and a timestamp, re-diffing on every local rubric run — store repo-relative paths or untrack it. Add `dependabot.yml` (Part V.2). Add a `make clean` sweep for the ~48 MB of stale local coverage artifacts. Make DynamoDB Local a `services:` container with a healthcheck instead of `docker run` + a 10-attempt curl poll.

---

## Part X — What this document does not recommend (identity guardrails)

For completeness, changes that superficially fit "stronger and more usable" but are refused because they dissolve what the product is:

- **No second database backend.** Multi-backend support multiplies the contract surface by backends × languages and destroys the parity guarantee. DynamoDB-specificity is the value.
- **No SQL idioms** (joins, lazy relationship loading, generic query planners). The framework exists to make DynamoDB's shape learnable, not to hide it.
- **No softening of fail-closed encryption** — not for dev experience, not for local testing. Explicit caller-level test configuration is the only flexibility channel.
- **No hiding of DynamoDB limits** (batch sizes, transaction caps, throttling). Expose and handle; don't silently abstract.
- **No single-runtime feature shipping.** Every contract-visible capability lands in Go + TS + Python with a scenario in the same release — the KEY-M1 Python gap (Part III) is a standing violation to repair, not a precedent.
- **No registry publishing by default** (Part V.2) — mitigations first; a registry is a deliberate maintainer-level policy revisit, not a convenience patch.

---

## Prioritized roadmap

Sequenced for dependency order and leverage. "Contract-visible" items require DMS coverage + scenario + all three runtimes per release discipline.

### P0 — Correctness and unblocking (target: next 1–2 minor releases)

| # | Item | Why first | Breaking? |
|---|---|---|---|
| 1 | Fix/deprecate Go `DB.Transaction` non-atomicity; fix Lambda `(nil,nil)` init poisoning + timeout-buffer race | Active correctness traps in the foundation | Doc fix + deprecation now; semantics change is major |
| 2 | TS big-number precision preservation | Silent cross-runtime data divergence | Additive w/ strategy flag |
| 3 | Lower Python floor to `>=3.12`; TS Node floor to 20 + `require` condition; AWS SDK → peerDeps | Converts both SDKs from internal-only to installable | No |
| 4 | Truth-up all three API references; fix `CONTRIBUTING.md` / `examples/README.md` staleness; one Go-toolchain version string | First-impression damage, near-zero effort | No |
| 5 | Python package/import name reconciliation (or loud documentation) | Hard newcomer stop | Shim now; rename at major |
| 6 | `dependabot.yml`; fix tracked rubric-report absolute paths | Hygiene, minutes of work | No |

### P1 — The competitive core (target: 1–2 quarters)

| # | Item | Depends on |
|---|---|---|
| 7 | Typed API layers: Go generics layer, TS `defineModel` inference, Python typed roles + typed client | — (additive, coordinate error-timing semantics) |
| 8 | Contract harness `query` op + P1 query/GSI/projection/pagination scenarios; fix `N` assertion; `item_equals`/`cursor_equals`; `ConsistentRead` reconciliation | — |
| 9 | Type-matrix scenarios (NS/BS/B/BOOL/NULL/L/floats/empty-string) | 8 |
| 10 | KEY-M1 Python evaluator + tests | — |
| 11 | Consumer testing: Go `NewWithClient` + in-memory fake validated against the P0 corpus; port fake to TS/Py; moto guidance | 8 (fake validated by scenarios) |
| 12 | Go one-command local quickstart; publish ts/docs + py/docs into the site; Examples nav group | — |
| 13 | Error taxonomy enrichment (`ErrVersionConflict`, `ErrThrottled`, `ErrTransactionConflict`; `errors.As` everywhere) — contract-coordinated | 8 |
| 14 | CI de-duplication + `make rubric-fast` | — |

### P2 — Strategic build-out (target: 2–4 quarters)

| # | Item | Depends on |
|---|---|---|
| 15 | `tabletheory` consumer CLI: `init` scaffold, DMS `validate`, `gen` (models ×3 languages) | DMS codegen design |
| 16 | DMS → CDK/Terraform generation; DynamoDB-Local variant of `cdk-multilang` | 15 |
| 17 | Generative-coding artifacts: `llms.txt`, machine-readable tag/DMS vocabulary, consumer rules template, prompt recipes | 15 (pairs with `init`) |
| 18 | Encryption interop scenario + Go driver error-code mapping | 8 |
| 19 | Release-lane simplification (single manifest, generated version files, merge queue) | Its own project; plan via scope-need |
| 20 | Explicit parity decisions: TS/Py migration story, PartiQL/TransactGet yes-or-no, lazy iteration + async Python, native `count()`/`get_or_none`, streams helpers | Each is contract-visible if "yes" |
| 21 | Code-health consolidation: retire dead Go `MainExecutor`, unify converters, de-dup TS builders / Py helpers, decompose `query.go` | After 7 to avoid double churn |
| 22 | Generated API references (godoc/TypeDoc/pdoc) replacing hand-written | 12 |

### Sequencing rationale

P0 removes traps and unblocks installation — everything a prospective consumer hits in the first hour. P1 is where the product either matches or beats its competitors: types + contract depth + testability are the three things an evaluating team checks before committing a production workload. P2 converts TableTheory's unique architecture (DMS + contract corpus) into features nobody else can copy cheaply, and pays down the process debt that currently consumes half the maintainer/steward loop's throughput.

---

*Prepared by the TableTheory steward, 2026-07-02, from six parallel repository investigations (Go runtime, TypeScript runtime, Python runtime, contract/DMS layer, docs/onboarding, CI/release machinery). File references are to the `staging` tree at `74a9eb7`.*
