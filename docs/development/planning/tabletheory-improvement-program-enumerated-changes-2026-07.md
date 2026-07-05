<!-- AI Training: Internal planning document — enumerated change list for the 2026-07 product strengthening program -->

# Enumerated Changes: TableTheory Product Strengthening Program (2026-07)

**Descends from:** `tabletheory-improvement-program-scoped-need-2026-07.md` (decisions ratified by maintainer 2026-07-02)
**Assessment source:** `tabletheory-product-improvement-assessment-2026-07.md`
**Baseline:** `staging`@74a9eb7, v1.10.0 stable / v1.10.1-rc.1

## Conventions used in this list

- **Capability-gating pattern.** Every new contract scenario lands first (parity rule) but capability-gated, so runners that don't yet advertise the capability skip it and the commit is green. Runtime commits then implement + advertise. Where a runtime already conforms, its advertisement rides the scenario commit. This satisfies both the scenario-first rule and the every-commit-green rule.
- **Validation shorthand:** `GO` = `make fmt && make lint && make test-unit`; `TS` = `cd ts && npm run check`; `PY` = `uv --directory py run pytest -q tests/unit` (+ ruff/mypy via `bash scripts/verify-python-build.sh`); `CT` = `make docker-up && bash scripts/verify-contract-tests.sh`; `RUBRIC` = `make rubric`; `HYG` = `bash scripts/test-release-hygiene-policy.sh` + `bash scripts/verify-release-cycle-state.sh`.
- Version manifests (`.release-please-manifest*.json`, `ts/package.json` version, `py/.../version.json`) are **never** touched by these commits — release-please owns them at release time. Semver implications are carried by the Conventional Commit subjects.
- Items marked **[contingency]** may surface follow-up fix commits that cannot be pre-enumerated (e.g., a type-matrix scenario exposing a real divergence); those follow the debug-parity-drift discipline when they appear.

---

## A. Correctness repairs and truth-in-docs (1–15)

### 1. Correct and deprecate the non-atomic `DB.Transaction`

- **Paths**: `pkg/core/interfaces.go`, `internal/theorydb/theorydb.go`, `tabletheory.go`, `docs/features/transactions*` (published transactions page), `docs/api-reference.md`
- **Runtime scope**: go
- **Contract impact**: docs-only (behavior unchanged; claims corrected)
- **Backward compat**: additive (adds `// Deprecated:` markers pointing to `Transact()`; no signature change)
- **Acceptance**: no shipped doc or doc comment claims `DB.Transaction`/`TransactionFunc` is atomic; both carry deprecation notices naming `Transact()` as the replacement and v2 as the removal horizon.
- **Validation**: GO
- **Conventional Commit subject**: `fix(transaction): document non-atomic DB.Transaction and deprecate in favor of Transact()`

### 2. Fix Lambda cold-start init poisoning

- **Paths**: `internal/theorydb/lambda.go` (+ new unit test)
- **Runtime scope**: go
- **Contract impact**: internal
- **Backward compat**: additive (error paths only)
- **Acceptance**: after a failed `createLambdaDB()`, every subsequent `NewLambdaOptimized` call returns the cached error (never `(nil, nil)`); covered by a unit test simulating init failure then retry.
- **Validation**: GO
- **Conventional Commit subject**: `fix(lambda): return cached init error instead of (nil, nil) after failed cold start`

### 3. Fix the `lambdaTimeoutBuffer` data race

- **Paths**: `internal/theorydb/lambda.go`
- **Runtime scope**: go
- **Contract impact**: internal
- **Backward compat**: additive
- **Acceptance**: `OptimizeForMemory` writes under the same mutex `WithLambdaTimeout` reads under; a `-race` test exercising both concurrently passes.
- **Validation**: GO (race detector is on in `test-unit`)
- **Conventional Commit subject**: `fix(lambda): synchronize lambdaTimeoutBuffer access`

### 4. Make `DefaultDBFactory` fail loudly

- **Paths**: `pkg/testing/factory.go` (+ test)
- **Runtime scope**: go
- **Contract impact**: internal
- **Backward compat**: additive-leaning behavior fix (was returning `nil, nil`; now returns a descriptive error — flag for backward-compat review; silent nil was unusable, not relied-upon behavior)
- **Acceptance**: `DefaultDBFactory.CreateDB` never returns a nil DB with nil error.
- **Validation**: GO
- **Conventional Commit subject**: `fix(testing): return explicit error from DefaultDBFactory instead of nil DB`

### 5. Stop Go mocks panicking on type mismatches

- **Paths**: `pkg/mocks/query.go`, `pkg/mocks/aws_dynamodb.go`, siblings
- **Runtime scope**: go
- **Contract impact**: internal
- **Backward compat**: additive (panics become test failures)
- **Acceptance**: a wrong `.Return(...)` type produces a failed assertion with a diagnostic, not a process crash.
- **Validation**: GO
- **Conventional Commit subject**: `fix(mocks): fail assertions instead of panicking on type mismatches`

### 6. Reject unsupported Python unions and unify storage-type resolution

- **Paths**: `py/src/theorydb_py/attr_types.py`, `streams.py`, `table.py`, `model.py` (+ tests)
- **Runtime scope**: py
- **Contract impact**: internal
- **Backward compat**: **flag for review** — `int | str | None` previously silently stored as `"S"`; now a model error at `from_dataclass`. Converts silent corruption into loud failure; justified as a fix.
- **Acceptance**: >2-member unions raise a model-definition error; `streams.py` and `table.py` resolve storage types through one shared helper.
- **Validation**: PY
- **Conventional Commit subject**: `fix(py): reject unsupported union annotations and unify storage-type resolution`

### 7. Correct published type-safety and generics over-claims

- **Paths**: `docs/core-patterns.md`, `docs/_decisions.yaml` (sdk_selection), `docs/migration-guide.md` (these feed the theorycloud KB)
- **Runtime scope**: none (docs only)
- **Contract impact**: docs-only
- **Backward compat**: n/a
- **Acceptance**: no published page claims Go generics or compile-time attribute-name checking until item 73 makes it true; claims re-strengthened later by WS-B docs.
- **Validation**: docs link check (rubric DOC gates)
- **Conventional Commit subject**: `docs: correct type-safety and generics claims in published docs`

### 8. Truth-up the TypeScript API reference and README

- **Paths**: `ts/docs/api-reference.md`, `ts/README.md`, `ts/docs/getting-started.md`
- **Runtime scope**: none (docs only)
- **Contract impact**: docs-only
- **Backward compat**: n/a
- **Acceptance**: no documented signature differs from `ts/src`/`ts/dist` reality; the stale "not yet implemented" list (filter expressions, update builder, type surface — all shipped) is deleted.
- **Validation**: TS (docs snippets compile where tested), rubric DOC gates
- **Conventional Commit subject**: `docs(ts): correct API reference signatures and remove stale capability list`

### 9. Truth-up the Python API reference

- **Paths**: `py/docs/api-reference.md`, `py/docs/getting-started.md`
- **Runtime scope**: none (docs only)
- **Contract impact**: docs-only
- **Backward compat**: n/a
- **Acceptance**: `put` no longer documents `if_not_exists`; `transact_write` documents `actions`; `query` documents all 9 real parameters; batch defaults match source.
- **Validation**: rubric DOC gates
- **Conventional Commit subject**: `docs(py): correct API reference signatures`

### 10. Repair examples/README and basic-example runnability

- **Paths**: `examples/README.md`, `examples/basic/{todo,notes,contacts}/Makefile` (new), toolchain version strings in `docs/getting-started.md` + `CONTRIBUTING.md`
- **Runtime scope**: go (examples)
- **Contract impact**: docs-only
- **Backward compat**: n/a
- **Acceptance**: the documented quick-start commands succeed in each basic example; no reference to `WithLambdaOptimizations` or single-return `New`; one Go toolchain version string (from `go.mod`) everywhere.
- **Validation**: `cd examples/basic/todo && make run` (against DynamoDB Local); GO
- **Conventional Commit subject**: `docs(examples): repair quick-start commands and remove non-existent APIs`

### 11. Reconcile raw-read consistency across contract runners

- **Paths**: `contract-tests/runners/go/internal/runner/runner.go`, `contract-tests/runners/py/test_contract_p0.py` (TS already uses `ConsistentRead: true`)
- **Runtime scope**: all (harness)
- **Contract impact**: scenario-change (harness semantics)
- **Backward compat**: n/a (test harness)
- **Acceptance**: all three runners perform raw-item assertions with consistent reads; documented in the suite outline.
- **Validation**: CT
- **Conventional Commit subject**: `fix(contract): use consistent reads for raw-item assertions in all runners`

### 12. Fix runner numeric assertions to canonical decimal strings

- **Paths**: `contract-tests/runners/go/internal/runner/runner.go`, `runners/ts/src/runner.ts`, `runners/py/test_contract_p0.py`, suite outline doc
- **Runtime scope**: all (harness)
- **Contract impact**: scenario-change (harness capability — prerequisite for numeric scenarios)
- **Backward compat**: n/a
- **Acceptance**: scenario expectations can assert non-integer and >2^53 `N` values; the `asInt64` coercion is gone; existing 9 fixtures still pass.
- **Validation**: CT
- **Conventional Commit subject**: `fix(contract): assert DynamoDB numbers as canonical decimal strings`

### 13. Add the number-precision P0 scenario (capability-gated)

- **Paths**: `contract-tests/scenarios/p0/10-number-precision.yml`, `contract-tests/dms/v0.1/models/` (numeric fixture attributes), Go + Py driver capability advertisement (both already exact)
- **Runtime scope**: all (scenario) — Go/Py advertise in this commit; TS follows in 14
- **Contract impact**: scenario-change
- **Backward compat**: additive
- **Acceptance**: scenario pins round-trip of >2^53 integers and non-integer decimals; passes in Go and Python; skipped (capability-gated) in TS.
- **Validation**: CT
- **Conventional Commit subject**: `feat(contract): add number-precision scenario`

### 14. Preserve large-number precision in TypeScript unmarshal (opt-in)

- **Paths**: `ts/src/marshal.ts`, `ts/src/model.ts` or client options (strategy flag), `ts/docs/`, `contract-tests/runners/ts/src/driver.ts` (advertise capability, enable flag)
- **Runtime scope**: ts
- **Contract impact**: additive-runtime (pinned by scenario 13)
- **Backward compat**: additive — opt-in `numberMode` flag in 1.x; default flips at v2 (item 121)
- **Acceptance**: with the flag enabled, scenario 13 passes in TS; without it, existing behavior is unchanged and documented as lossy.
- **Validation**: TS, CT
- **Conventional Commit subject**: `feat(ts): add precision-safe number unmarshaling mode`

### 15. Label client-side aggregations and add memory warnings

- **Paths**: `ts/src/query.ts` + `ts/src/aggregates.ts` doc comments, `py/src/theorydb_py/aggregates.py` doc comments, `ts/docs/`, `py/docs/`, `docs/` feature pages
- **Runtime scope**: ts, py
- **Contract impact**: docs-only
- **Backward compat**: n/a
- **Acceptance**: every client-side aggregation API states it materializes the full result set; docs point to native `count()` (items 53–56) once it exists.
- **Validation**: TS, PY
- **Conventional Commit subject**: `docs: label client-side aggregations as full-result-set operations`

---

## B. Distribution & platform floors (16–22)

### 16. Lower the Python floor to 3.12

- **Paths**: `py/pyproject.toml` (`requires-python`), `.github/workflows/python.yml` (add 3.12 matrix lane), `py/docs/getting-started.md`
- **Runtime scope**: py
- **Contract impact**: internal (packaging)
- **Backward compat**: additive (widens support)
- **Acceptance**: full Python suite green on 3.12 and 3.14 in CI; no 3.13+-only stdlib usage remains (audited).
- **Validation**: PY on both interpreters; RUBRIC
- **Conventional Commit subject**: `feat(py): support Python 3.12+`

### 17. Support Node 20 LTS and CommonJS consumers

- **Paths**: `ts/package.json` (engines, exports map / dual build), `ts/tsconfig*.json`, `.github/workflows/typescript.yml` (Node 20 lane), `ts/docs/getting-started.md`
- **Runtime scope**: ts
- **Contract impact**: internal (packaging)
- **Backward compat**: additive (widens support)
- **Acceptance**: `require('@theory-cloud/tabletheory-ts')` resolves with types on Node 20; ESM path unchanged; CI runs both Node 20 and 24.
- **Validation**: TS + a CJS smoke test; RUBRIC
- **Conventional Commit subject**: `feat(ts): support Node 20 LTS and CommonJS consumers`

### 18. Move AWS SDK clients to peer dependencies

- **Paths**: `ts/package.json`, `ts/package-lock.json` (dep graph only, not version), `ts/docs/getting-started.md`
- **Runtime scope**: ts
- **Contract impact**: internal (packaging)
- **Backward compat**: **flag for review** — npm ≥7 auto-installs peers, so tarball consumers are unaffected; documented range must match tested SDK versions
- **Acceptance**: `@aws-sdk/client-dynamodb`/`client-kms`/`client-sts` are `peerDependencies` with ranges; install docs state the requirement; the `overrides` caveat is documented.
- **Validation**: TS; fresh-install smoke test from a packed tarball
- **Conventional Commit subject**: `feat(ts): declare AWS SDK clients as peer dependencies`

### 19. Add the `tabletheory_py` canonical import alias

- **Paths**: `py/src/tabletheory_py/` (re-export package), `py/pyproject.toml` (packages), `py/README.md`, `py/docs/*` (imports switched), `py/tests/` (one alias test)
- **Runtime scope**: py
- **Contract impact**: additive-runtime
- **Backward compat**: additive — `theorydb_py` keeps working, emits `DeprecationWarning` from 1.x docs perspective only (warning wired in item 121's precursor; this commit is pure alias + docs switch)
- **Acceptance**: `import tabletheory_py` exposes the full public surface; all docs import the canonical name; wheel contains both packages.
- **Validation**: PY; `bash scripts/verify-python-build.sh`
- **Conventional Commit subject**: `feat(py): add tabletheory_py canonical import alias`

### 20. Add scheduled dependency automation

- **Paths**: `.github/dependabot.yml` (new)
- **Runtime scope**: none
- **Contract impact**: internal
- **Backward compat**: n/a
- **Acceptance**: gomod, npm (ts/ + examples), pip (py/), and github-actions ecosystems all have scheduled update PRs targeting `staging`.
- **Validation**: YAML lint; dependabot dry parse
- **Conventional Commit subject**: `chore(ci): add dependabot configuration`

### 21. Ship version-discovery and consumer update automation guidance

- **Paths**: `docs/runtimes/typescript.md`, `docs/runtimes/python.md`, `ts/docs/getting-started.md`, `py/docs/getting-started.md`, new `docs/guides/consumer-updates.md` (Renovate github-releases datasource snippets)
- **Runtime scope**: none (docs)
- **Contract impact**: docs-only
- **Backward compat**: n/a
- **Acceptance**: every install page shows how to find the current version and includes a copyable Renovate config that tracks TableTheory release assets.
- **Validation**: rubric DOC gates
- **Conventional Commit subject**: `docs(install): add version discovery and Renovate automation guidance`

### 22. Publish a pip find-links index from releases

- **Paths**: `.github/workflows/pages.yml` (or a small new workflow step), `scripts/` (index generator), `py/docs/getting-started.md`
- **Runtime scope**: py (distribution)
- **Contract impact**: internal
- **Backward compat**: additive — GitHub-hosted, immutable-release-backed; not a registry
- **Acceptance**: `pip install tabletheory-py --find-links <published index URL>` resolves released wheels with version selection.
- **Validation**: workflow dry run; manual pip install against the generated index
- **Conventional Commit subject**: `feat(release): publish pip find-links index for Python releases`

---

## C. Contributor loop & CI hygiene (23–29)

### 23. Add `make rubric-fast`

- **Paths**: `Makefile`, `scripts/verify-rubric.sh` (or a sibling fast path honoring `SKIP_INTEGRATION`)
- **Runtime scope**: none (tooling)
- **Contract impact**: internal
- **Backward compat**: n/a
- **Acceptance**: `make rubric-fast` runs lint + unit + doc gates for all three runtimes without Docker and completes as the documented contributor loop.
- **Validation**: run both `make rubric-fast` and full RUBRIC
- **Conventional Commit subject**: `feat(make): add rubric-fast contributor gate`

### 24. Rewrite CONTRIBUTING.md for the three-runtime reality

- **Paths**: `CONTRIBUTING.md`
- **Runtime scope**: none (docs)
- **Contract impact**: docs-only
- **Backward compat**: n/a
- **Acceptance**: zero `theorydb`-org references; Go+TS+Py dev setup with real commands; the rubric and `rubric-fast` explained; authoring-docs section preserved.
- **Validation**: rubric DOC gates
- **Conventional Commit subject**: `docs(contributing): rewrite for three-runtime contribution`

### 25. De-duplicate CI suites on staging PRs

- **Paths**: `.github/workflows/{python,typescript,unit-cover}.yml`, `.github/workflows/quality-gates.yml`
- **Runtime scope**: none (CI)
- **Contract impact**: internal
- **Backward compat**: n/a
- **Acceptance**: a staging PR executes each language suite exactly once; push-triggered coverage unchanged; branch-protection required checks updated in the same change.
- **Validation**: CI run on a test PR; RUBRIC
- **Conventional Commit subject**: `chore(ci): deduplicate language suites on staging PRs`

### 26. Run DynamoDB Local as a service container with a healthcheck

- **Paths**: `.github/workflows/quality-gates.yml` (and integration-running workflows), `Makefile`/`scripts` poll-loop removal where CI-owned
- **Runtime scope**: none (CI)
- **Contract impact**: internal
- **Backward compat**: n/a
- **Acceptance**: CI integration jobs use a `services:` container with a healthcheck; local `make docker-up` unchanged.
- **Validation**: CI run; CT locally
- **Conventional Commit subject**: `chore(ci): run DynamoDB Local as a service container`

### 27. Store repo-relative evidence paths in the rubric report

- **Paths**: `gov-infra/verifiers/gov-verify-rubric.sh`, `gov-infra/evidence/gov-rubric-report.json`
- **Runtime scope**: none (gov)
- **Contract impact**: internal
- **Backward compat**: n/a (report schema field values change shape; schema version noted)
- **Acceptance**: two rubric runs on different machines produce identical reports when checks are identical; no absolute home paths in tracked files.
- **Validation**: RUBRIC twice; `git diff` clean on second run
- **Conventional Commit subject**: `fix(gov): store repo-relative evidence paths in rubric report`

### 28. Sweep local artifacts in `make clean`

- **Paths**: `Makefile`
- **Runtime scope**: none (tooling)
- **Contract impact**: internal
- **Backward compat**: n/a
- **Acceptance**: `make clean` removes `coverage*.out` and other gitignored heavy artifacts it generates.
- **Validation**: `make clean` leaves `git status` clean and removes the ~48 MB of stale coverage files
- **Conventional Commit subject**: `chore(make): sweep local coverage artifacts in make clean`

### 29. Add the promised `make contract-tests` target

- **Paths**: `Makefile`, `docs/reference/contract-scenarios.md`
- **Runtime scope**: none (tooling)
- **Contract impact**: internal
- **Backward compat**: n/a
- **Acceptance**: `make contract-tests` runs the three-runtime suite as `docs/reference/contract-scenarios.md` promises.
- **Validation**: CT via the new target
- **Conventional Commit subject**: `chore(make): add contract-tests target`

---

## D. Contract harness & coverage expansion (30–50)

### 30. Add query/scan ops to the scenario schema and all three runners

- **Paths**: `contract-tests/runners/go/internal/{scenario,driver,runner}/`, `runners/ts/src/{types,driver,runner}.ts`, `runners/py/test_contract_p0.py` (split harness module while here), suite outline doc
- **Runtime scope**: all (harness)
- **Contract impact**: scenario-change (harness foundation; one shared intent)
- **Backward compat**: n/a
- **Acceptance**: scenarios can express `query`/`scan` steps (key conditions, operators, index, sort, limit, projection) with typed result assertions; capability keys `query.basic`/`scan.basic` defined; existing fixtures unaffected.
- **Validation**: CT
- **Conventional Commit subject**: `feat(contract): add query and scan ops to the scenario harness`

### 31. Implement `item_equals` and `cursor_equals` assertion primitives

- **Paths**: all three runners, suite outline doc
- **Runtime scope**: all (harness)
- **Contract impact**: scenario-change
- **Backward compat**: n/a
- **Acceptance**: the two documented-but-missing primitives work identically in all runners; at least one existing fixture upgraded from `item_contains` to `item_equals` to prove exact-match catches extra attributes.
- **Validation**: CT
- **Conventional Commit subject**: `feat(contract): implement item_equals and cursor_equals assertions`

### 32. Add P1 query-semantics scenarios

- **Paths**: `contract-tests/scenarios/p1/` (new tier), all three drivers advertise `query.basic`
- **Runtime scope**: all
- **Contract impact**: scenario-change
- **Backward compat**: additive
- **Acceptance**: operators (`=`, `begins_with`, `<`, `>`, `between`), sort direction, and limit pinned; all three runtimes pass. **[contingency]**
- **Validation**: CT
- **Conventional Commit subject**: `feat(contract): add P1 query semantics scenarios`

### 33. Add GSI and projection scenarios

- **Paths**: `contract-tests/scenarios/p1/`, using existing `gsi-email` and the orphaned `Order`/`gsi-status` fixtures
- **Runtime scope**: all
- **Contract impact**: scenario-change
- **Backward compat**: additive
- **Acceptance**: index queries, GSI-consistent-read rejection, and `KEYS_ONLY`/`INCLUDE` projection behavior asserted cross-runtime. **[contingency]**
- **Validation**: CT
- **Conventional Commit subject**: `feat(contract): add GSI and projection scenarios`

### 34. Add pagination scenarios and widen the cursor golden corpus

- **Paths**: `contract-tests/scenarios/p1/`, `contract-tests/golden/cursor/` (N/B keys, DESC, no-index, single-key, empty lastKey), per-runtime cursor tests
- **Runtime scope**: all
- **Contract impact**: scenario-change
- **Backward compat**: additive
- **Acceptance**: multi-page query walk with cursor hand-off pinned in scenarios; golden corpus covers the enumerated shapes byte-exactly in all runtimes.
- **Validation**: CT
- **Conventional Commit subject**: `feat(contract): expand cursor goldens and add pagination scenarios`

### 35. Add type-matrix round-trip scenarios

- **Paths**: `contract-tests/scenarios/p0/` (new fixtures), `contract-tests/dms/v0.1/models/` (model with NS/BS/B/BOOL/NULL/L/M attributes + a non-omitempty optional string)
- **Runtime scope**: all
- **Contract impact**: scenario-change
- **Backward compat**: additive
- **Acceptance**: NS/BS/B/BOOL/NULL/L round-trips, empty-set→NULL rule, and intentional `""` persistence pinned cross-runtime. **[contingency — most likely place to surface real divergence]**
- **Validation**: CT
- **Conventional Commit subject**: `feat(contract): add type-matrix round-trip scenarios`

### 36. Resolve the `pascalCase` naming drift (evolve-dms)

- **Paths**: `pkg/dms/dms.go` **or** `docs/development/planning/theorydb-spec-dms-v0.1.md` + `docs/reference/dms-spec.md` — direction per consumer check
- **Runtime scope**: go (+ spec)
- **Contract impact**: **dms-change** — runs through the evolve-dms discipline
- **Backward compat**: flag for review; steward default is code-removal (stop emitting an unspec'd convention) after verifying no consumer DMS uses it
- **Acceptance**: the spec enum and every emitter/validator agree on the exact naming-convention set, in the same release.
- **Validation**: GO, CT, RUBRIC
- **Conventional Commit subject**: `fix(dms): align naming-convention enum between spec and emitters`

### 37. Add a snake_case naming-strategy scenario

- **Paths**: `contract-tests/scenarios/p1/`, new DMS model fixture with `snake_case`
- **Runtime scope**: all
- **Contract impact**: scenario-change
- **Backward compat**: additive
- **Acceptance**: attribute-name resolution under `snake_case` pinned cross-runtime (raw item attribute names asserted).
- **Validation**: CT
- **Conventional Commit subject**: `feat(contract): add snake_case naming-strategy scenario`

### 38. Map encryption error codes in the Go contract driver

- **Paths**: `contract-tests/runners/go/internal/driver/driver.go`
- **Runtime scope**: go (harness)
- **Contract impact**: scenario-change (harness parity with TS driver's error union)
- **Backward compat**: n/a
- **Acceptance**: `ErrEncryptionNotConfigured`, `ErrEncryptedFieldNotQueryable`, `ErrInvalidEncryptedEnvelope` mappable in Go scenario assertions.
- **Validation**: CT
- **Conventional Commit subject**: `fix(contract): map encryption error codes in Go driver`

### 39. Add cross-runtime interop support to the harness

- **Paths**: all three runners + scenario schema (seed-phase/read-phase manifest so one runtime writes and the others read the same physical items)
- **Runtime scope**: all (harness)
- **Contract impact**: scenario-change
- **Backward compat**: n/a
- **Acceptance**: a scenario can declare `seed_runtime` steps whose written items are asserted by the other two runners in the shared DynamoDB Local instance.
- **Validation**: CT
- **Conventional Commit subject**: `feat(contract): add cross-runtime interop scenario support`

### 40. Add encryption fail-closed and interop scenarios

- **Paths**: `contract-tests/scenarios/p0/` (fail-closed reads without KMS config → error; deterministic-provider encrypt-in-Go → decrypt-in-TS/Py and rotations of the direction), deterministic test providers in Go/Py test scope to match TS testkit's
- **Runtime scope**: all
- **Contract impact**: scenario-change
- **Backward compat**: additive — fail-closed default untouched; deterministic providers are explicit test-scope configuration (per security policy: caller-level, never a softened default)
- **Acceptance**: fail-closed behavior and envelope interop pinned cross-runtime; no scenario weakens the production default.
- **Validation**: CT
- **Conventional Commit subject**: `feat(contract): add encryption fail-closed and interop scenarios`

### 41. Add the Python derived-key evaluator (KEY-M1 parity)

- **Paths**: `py/src/theorydb_py/key_contract.py` (new), `contract-tests/runners/py/test_contract_key_contract.py` (new, against the shared v0.1 fixtures)
- **Runtime scope**: py
- **Contract impact**: additive-runtime (pinned by existing shared fixtures)
- **Backward compat**: additive
- **Acceptance**: Python evaluates all 18 fixture keys byte-identically to Go/TS.
- **Validation**: PY, CT
- **Conventional Commit subject**: `feat(py): add derived-key contract evaluator`

### 42. Extend the key-contract spec with lowercase and url_encode transforms (v0.2)

- **Paths**: `docs/development/planning/tabletheory-model-contract-v0.1.md` → v0.2 section, `contract-tests/key-contracts/v0.2/` fixtures (not yet consumed)
- **Runtime scope**: none (spec first)
- **Contract impact**: dms-change-adjacent (sidecar spec) — spec lands before any evaluator
- **Backward compat**: additive (new transforms; v0.1 contracts unchanged)
- **Acceptance**: transform semantics defined with explicit codepoint/encoding rules (same rigor as the trim set); fixtures authored.
- **Validation**: rubric DOC gates
- **Conventional Commit subject**: `feat(keycontract): specify v0.2 transforms lowercase and url_encode`

### 43. Implement v0.2 transforms in the Go evaluator and generator

- **Paths**: `pkg/keycontract/`, `cmd/` generator, regenerated `contract-tests/generated/key-contracts/` (drift gate requires same-commit regeneration), `contract-tests/runners/go/key_contract_test.go`
- **Runtime scope**: go
- **Contract impact**: additive-runtime
- **Backward compat**: additive
- **Acceptance**: Go evaluates v0.2 fixtures; `scripts/verify-generated-ts-key-contract.sh` green.
- **Validation**: GO, CT
- **Conventional Commit subject**: `feat(keycontract): implement v0.2 transforms in Go evaluator and generator`

### 44. Implement v0.2 transforms in the TypeScript evaluator

- **Paths**: `ts/src/key-contract.ts`, `contract-tests/runners/ts/test/key-contract.contract.test.ts`
- **Runtime scope**: ts
- **Contract impact**: additive-runtime
- **Backward compat**: additive
- **Acceptance**: TS evaluates v0.2 fixtures byte-identically to Go.
- **Validation**: TS, CT
- **Conventional Commit subject**: `feat(ts): support key-contract v0.2 transforms`

### 45. Implement v0.2 transforms in the Python evaluator

- **Paths**: `py/src/theorydb_py/key_contract.py`, runner test
- **Runtime scope**: py
- **Contract impact**: additive-runtime
- **Backward compat**: additive
- **Acceptance**: Python evaluates v0.2 fixtures byte-identically to Go/TS.
- **Validation**: PY, CT
- **Conventional Commit subject**: `feat(py): support key-contract v0.2 transforms`

### 46. Add the version-conflict error-distinction scenario (capability-gated)

- **Paths**: `contract-tests/scenarios/p0/` (extends optimistic-lock coverage with a distinct `ErrVersionConflict` code), driver error unions
- **Runtime scope**: all (scenario)
- **Contract impact**: scenario-change (error-taxonomy growth)
- **Backward compat**: additive — new code must still satisfy existing `ErrConditionFailed` matching (Go: wraps it so `errors.Is` holds)
- **Acceptance**: scenario asserts a version-mismatch write yields `ErrVersionConflict` distinguishable from generic condition failure; gated until runtimes advertise.
- **Validation**: CT
- **Conventional Commit subject**: `feat(contract): add version-conflict error scenario`

### 47. Distinguish version conflicts in Go errors

- **Paths**: `pkg/errors/errors.go`, `pkg/query/executor.go`, `internal/theorydb/query_executor.go`, driver advertisement
- **Runtime scope**: go
- **Contract impact**: additive-runtime
- **Backward compat**: additive (`errors.Is(err, ErrConditionFailed)` still true for version conflicts)
- **Acceptance**: scenario 46 passes in Go; also adds `ErrThrottled`/`ErrTransactionConflict` sentinels (runtime-local until pinned).
- **Validation**: GO, CT
- **Conventional Commit subject**: `feat(errors): add ErrVersionConflict and richer error sentinels`

### 48. Distinguish version conflicts in TypeScript

- **Paths**: `ts/src/errors` surface, `ts/src/client.ts`, driver advertisement
- **Runtime scope**: ts
- **Contract impact**: additive-runtime
- **Backward compat**: additive
- **Acceptance**: scenario 46 passes in TS.
- **Validation**: TS, CT
- **Conventional Commit subject**: `feat(ts): add version-conflict error code`

### 49. Distinguish version conflicts in Python

- **Paths**: `py/src/theorydb_py/aws_errors.py`, `table.py`, runner advertisement
- **Runtime scope**: py
- **Contract impact**: additive-runtime
- **Backward compat**: additive
- **Acceptance**: scenario 46 passes in Python; capability gate for 46 flipped to required now that all three advertise.
- **Validation**: PY, CT
- **Conventional Commit subject**: `feat(py): add version-conflict error code`

### 50. Classify Go AWS errors via errors.As everywhere

- **Paths**: `pkg/query/executor.go`, `pkg/transaction/transaction.go`, `pkg/query/batch_operations.go`
- **Runtime scope**: go
- **Contract impact**: internal (behavior-preserving; classification mechanism only)
- **Backward compat**: additive
- **Acceptance**: no `strings.Contains(err.Error(), ...)` classification remains; the two transaction paths classify identically; existing tests green.
- **Validation**: GO, CT
- **Conventional Commit subject**: `fix(errors): classify AWS errors via errors.As instead of string matching`

---

## E. Parity fills (51–66) — each scenario lands before its runtime trio

### 51. Add the native-count scenario (capability-gated)

- **Paths**: `contract-tests/scenarios/p1/`, scenario schema `count` step
- **Runtime scope**: all (scenario)
- **Contract impact**: scenario-change
- **Backward compat**: additive
- **Acceptance**: count-without-materialization semantics pinned (result count + no items fetched observable via harness).
- **Validation**: CT
- **Conventional Commit subject**: `feat(contract): add native count scenario`

### 52. Native Select=COUNT count in Go

- **Paths**: `pkg/query/`, `internal/theorydb/query_executor.go`, driver advertisement
- **Runtime scope**: go — **Acceptance**: existing `Count()` uses `Select=COUNT` end-to-end; scenario 51 passes. — **Backward compat**: additive. — **Validation**: GO, CT
- **Conventional Commit subject**: `feat(query): execute Count via Select=COUNT`

### 53. Native count in TypeScript

- **Paths**: `ts/src/query.ts`, driver advertisement — **Runtime scope**: ts — **Contract impact**: additive-runtime — **Backward compat**: additive — **Acceptance**: `count()` on query/scan builders issues `Select=COUNT`; scenario 51 passes. — **Validation**: TS, CT
- **Conventional Commit subject**: `feat(ts): add native count()`

### 54. Native count in Python

- **Paths**: `py/src/theorydb_py/table.py`/`query.py`, runner advertisement — **Runtime scope**: py — **Contract impact**: additive-runtime — **Backward compat**: additive — **Acceptance**: scenario 51 passes in Py; gate flipped to required. — **Validation**: PY, CT
- **Conventional Commit subject**: `feat(py): add native count()`

### 55. Add the optional-get scenario (capability-gated)

- **Paths**: `contract-tests/scenarios/p1/` — **Runtime scope**: all (scenario) — **Contract impact**: scenario-change — **Backward compat**: additive — **Acceptance**: get-miss returns an empty/none result (not an error) through the new op, pinned identically. — **Validation**: CT
- **Conventional Commit subject**: `feat(contract): add optional-get scenario`

### 56. Optional get in Go

- **Paths**: `pkg/query/` (e.g. `FirstOrNil`/`GetOptional`), driver — **Runtime scope**: go — **Contract impact**: additive-runtime — **Backward compat**: additive — **Acceptance**: scenario 55 passes. — **Validation**: GO, CT
- **Conventional Commit subject**: `feat(query): add optional get returning no-error miss`

### 57. Optional get in TypeScript

- **Paths**: `ts/src/client.ts` (`getOrNull`), driver — **Runtime scope**: ts — **Contract impact**: additive-runtime — **Backward compat**: additive — **Acceptance**: scenario 55 passes. — **Validation**: TS, CT
- **Conventional Commit subject**: `feat(ts): add getOrNull`

### 58. Optional get in Python

- **Paths**: `py/src/theorydb_py/table.py` (`get_or_none`), runner — **Runtime scope**: py — **Contract impact**: additive-runtime — **Backward compat**: additive — **Acceptance**: scenario 55 passes; gate required. — **Validation**: PY, CT
- **Conventional Commit subject**: `feat(py): add get_or_none`

### 59. Add transact-get harness op and scenario (capability-gated)

- **Paths**: scenario schema + three runners (`transact_get` op), `contract-tests/scenarios/p1/`
- **Runtime scope**: all (harness + scenario, one intent) — **Contract impact**: scenario-change — **Backward compat**: additive — **Acceptance**: atomic multi-get semantics and item-count limits pinned; gated. — **Validation**: CT
- **Conventional Commit subject**: `feat(contract): add transact-get op and scenario`

### 60. TransactGetItems in Go

- **Paths**: `pkg/transaction/`, `pkg/core/interfaces.go` (new method on an extension interface, not on existing exported interfaces), driver — **Runtime scope**: go — **Contract impact**: additive-runtime — **Backward compat**: additive (extension interface avoids breaking third-party `DB` implementers) — **Acceptance**: scenario 59 passes. — **Validation**: GO, CT
- **Conventional Commit subject**: `feat(transaction): add TransactGet support`

### 61. TransactGetItems in TypeScript

- **Paths**: `ts/src/client.ts`/`transaction` surface, driver — **Runtime scope**: ts — **Contract impact**: additive-runtime — **Backward compat**: additive — **Acceptance**: scenario 59 passes. — **Validation**: TS, CT
- **Conventional Commit subject**: `feat(ts): add transactGet`

### 62. TransactGetItems in Python

- **Paths**: `py/src/theorydb_py/table.py`, runner — **Runtime scope**: py — **Contract impact**: additive-runtime — **Backward compat**: additive — **Acceptance**: scenario 59 passes; gate required. — **Validation**: PY, CT
- **Conventional Commit subject**: `feat(py): add transact_get`

### 63. Add batch harness ops and batch-semantics scenarios

- **Paths**: scenario schema + three runners (`batch_get`/`batch_write` ops), `contract-tests/scenarios/p2/` (new tier: chunking at 100/25, unprocessed-item retry observable behavior)
- **Runtime scope**: all — **Contract impact**: scenario-change — **Backward compat**: additive — **Acceptance**: batch semantics pinned cross-runtime (all runtimes already implement batching — scenarios should pass; advertise in-commit). **[contingency]** — **Validation**: CT
- **Conventional Commit subject**: `feat(contract): add batch semantics scenarios`

### 64. Add generic-transaction harness ops and scenarios

- **Paths**: scenario schema + three runners (`transact_write` generic op), `contract-tests/scenarios/p2/` (atomicity: all-or-nothing on condition failure; cancellation-reason codes; 100-action cap)
- **Runtime scope**: all — **Contract impact**: scenario-change — **Backward compat**: additive — **Acceptance**: generic TransactWrite atomicity pinned in all runtimes — this is also the atomicity pin that item 1's deprecation and item 119's removal rely on. **[contingency]** — **Validation**: CT
- **Conventional Commit subject**: `feat(contract): add generic transaction scenarios`

### 65. Lazy pagination in TypeScript

- **Paths**: `ts/src/query.ts` (async-iterator `pages()`/`items()`), `ts/docs/` — **Runtime scope**: ts — **Contract impact**: additive-runtime (pagination semantics already pinned by 34) — **Backward compat**: additive — **Acceptance**: iterating stops fetching when the consumer stops; documented as the preferred large-result path. — **Validation**: TS
- **Conventional Commit subject**: `feat(ts): add async-iterator pagination`

### 66. Lazy pagination in Python

- **Paths**: `py/src/theorydb_py/table.py` (`query_iter`/`scan_iter` generators), `py/docs/` — **Runtime scope**: py — **Contract impact**: additive-runtime — **Backward compat**: additive — **Acceptance**: iteration is lazy page-by-page; docs steer large results away from `query_all`. — **Validation**: PY
- **Conventional Commit subject**: `feat(py): add lazy query/scan iterators`

---

## F. Typed API layers (67–74)

### 67. Export typed operator constants in Go

- **Paths**: `pkg/query/` (operator constants), docs — **Runtime scope**: go — **Contract impact**: additive-runtime — **Backward compat**: additive (strings still accepted) — **Acceptance**: `Where(field, query.OpBeginsWith, v)` compiles and typos are compile errors when constants are used; `Between(lo, hi)` two-arg form added. — **Validation**: GO
- **Conventional Commit subject**: `feat(query): add typed operator constants and Between helper`

### 68. Add the Go generic typed layer

- **Paths**: new `pkg/typed/` (or root additions): `ModelOf[T]`, `Query[T]` with `First() (T, error)`, `All() ([]T, error)`, typed keys; docs + example — **Runtime scope**: go — **Contract impact**: additive-runtime — **Backward compat**: additive — **Acceptance**: full CRUD+query flow expressible with zero `any` and zero reflection-error paths at call sites; error codes/timing identical to the untyped layer.
- **Validation**: GO, CT (spot: typed layer behind the same executor)
- **Conventional Commit subject**: `feat(go): add generic typed model and query layer`

### 69. Add concretely-typed option APIs in Go

- **Paths**: `pkg/core/` (new `TypedExtendedDB` extension interface or concrete-type methods), `internal/theorydb/`, deprecation markers on `opts ...any` variants — **Runtime scope**: go — **Contract impact**: additive-runtime — **Backward compat**: additive (new surface; old kept + deprecated; removal at item 122) — **Acceptance**: schema/table/migration options callable without type assertions; `DescribeTable` variant returns `*types.TableDescription` concretely. — **Validation**: GO
- **Conventional Commit subject**: `feat(core): add concretely-typed option and describe APIs`

### 70. Infer item types from defineModel in TypeScript

- **Paths**: `ts/src/model.ts` (generic `defineModel<const S>`), `ts/src/types` — **Runtime scope**: ts — **Contract impact**: additive-runtime — **Backward compat**: additive (existing untyped call sites still compile; default type parameter preserves old shape) — **Acceptance**: a type test file proves inferred item shape (`'S'`→string, optional→`?`) and that misuse fails compilation (`@ts-expect-error` fixtures). — **Validation**: TS
- **Conventional Commit subject**: `feat(ts): infer item types from defineModel schemas`

### 71. Typed model repository handles in TypeScript

- **Paths**: `ts/src/client.ts` (`db.model(User)` typed handle; string lookup kept), docs — **Runtime scope**: ts — **Contract impact**: additive-runtime — **Backward compat**: additive — **Acceptance**: `handle.get(...)` returns the inferred item type; `create`/`update` reject wrong shapes at compile time. — **Validation**: TS
- **Conventional Commit subject**: `feat(ts): add typed model repository handles`

### 72. Operator literal unions in TypeScript

- **Paths**: `ts/src/query.ts`, `ts/src/update-builder.ts` (op params: literal union | escape hatch), type tests — **Runtime scope**: ts — **Contract impact**: additive-runtime — **Backward compat**: additive — **Acceptance**: known operators autocomplete; unknown literals fail compilation; runtime validation unchanged. — **Validation**: TS
- **Conventional Commit subject**: `feat(ts): type query and update operators as literal unions`

### 73. Typed role constants and early validation in Python

- **Paths**: `py/src/theorydb_py/model.py` (`Role` constants/enum accepted alongside strings; validation at field-construction where possible), type tests — **Runtime scope**: py — **Contract impact**: additive-runtime — **Backward compat**: additive — **Acceptance**: `roles=[Role.PK]` type-checks; a typo'd constant is a mypy error; string roles still work. — **Validation**: PY
- **Conventional Commit subject**: `feat(py): add typed role constants and earlier role validation`

### 74. Type the Python client boundary

- **Paths**: `py/src/theorydb_py/table.py`, `py/pyproject.toml` (boto3-stubs as `typing` extra) — **Runtime scope**: py — **Contract impact**: additive-runtime — **Backward compat**: additive (parameter widening to a Protocol) — **Acceptance**: `Table(client=...)` is typed against the DynamoDB client protocol; mypy strict passes internally without `Any` on `_client` call sites. — **Validation**: PY
- **Conventional Commit subject**: `feat(py): type the DynamoDB client boundary`

---

## G. Consumer testing (75–79)

### 75. Add `NewWithClient` to Go

- **Paths**: `tabletheory.go`/`internal/theorydb/theorydb.go` (constructor accepting the `DynamoDBAPI` seam), docs — **Runtime scope**: go — **Contract impact**: additive-runtime — **Backward compat**: additive — **Acceptance**: a consumer can construct a fully-functional `DB` over any `DynamoDBAPI` implementation; documented in the testing guide. — **Validation**: GO
- **Conventional Commit subject**: `feat(go): add NewWithClient for injectable DynamoDB clients`

### 76. Build the Go state-backed in-memory fake

- **Paths**: new `pkg/testing/fakedb/` (in-memory table state honoring keys, conditions, versions, TTL attributes, basic filters), testing-guide docs in the same commit — **Runtime scope**: go — **Contract impact**: additive-runtime — **Backward compat**: additive — **Acceptance**: write-then-query consumer tests pass against the fake without Docker; behavior documented as scenario-validated (item 77).
- **Validation**: GO
- **Conventional Commit subject**: `feat(testing): add state-backed in-memory fake DynamoDB`

### 77. Validate the fake against the contract corpus

- **Paths**: `contract-tests/runners/go/` (a fake-backed run of the P0 scenario set), CI wiring — **Runtime scope**: go (harness) — **Contract impact**: scenario-change (new execution target, no scenario edits) — **Backward compat**: n/a — **Acceptance**: the P0 corpus passes against the fake in CI; divergences are fake bugs by definition. — **Validation**: CT + fake lane
- **Conventional Commit subject**: `feat(contract): validate in-memory fake against P0 scenarios`

### 78. Add a stateful table fake to the TypeScript testkit

- **Paths**: `ts/src/testkit/` (in-memory `send()` backend mirroring the Go fake's behavior), `ts/docs/testing-guide.md` — **Runtime scope**: ts — **Contract impact**: additive-runtime — **Backward compat**: additive — **Acceptance**: consumer tests can seed items and assert query/write behavior without per-command handler scripting. — **Validation**: TS
- **Conventional Commit subject**: `feat(ts): add stateful in-memory table fake to testkit`

### 79. Add a stateful fake and expand the Python testkit

- **Paths**: `py/src/theorydb_py/testkit.py` + new fake module, `py/docs/testing-guide.md` (including moto positioning), top-level re-exports — **Runtime scope**: py — **Contract impact**: additive-runtime — **Backward compat**: additive — **Acceptance**: ordered-expectation mock remains; a stateful fake exists for behavioral tests; the guide covers fake vs moto vs DynamoDB Local. — **Validation**: PY
- **Conventional Commit subject**: `feat(py): add stateful in-memory fake and expand testkit`

---

## H. DMS leverage & consumer CLI (80–90)

### 80. Introduce the `tabletheory` CLI with `validate`

- **Paths**: `cmd/tabletheory/` (new; `tabletheory-contract` becomes an alias or subcommand `contract generate-ts`), wraps `pkg/dms` validation with consumer-grade errors; docs page — **Runtime scope**: go (tooling) — **Contract impact**: internal — **Backward compat**: additive — **Acceptance**: `tabletheory validate <dms.yml>` gives actionable pass/fail with line context; old command path still works. — **Validation**: GO
- **Conventional Commit subject**: `feat(cli): introduce tabletheory CLI with validate subcommand`

### 81. Generate Go models from DMS

- **Paths**: `cmd/tabletheory/` + `pkg/dms` codegen, golden-file tests — **Runtime scope**: go — **Contract impact**: internal (generated output must pass `AssertModelsEquivalent`) — **Backward compat**: additive — **Acceptance**: `tabletheory gen --lang go` emits structs that round-trip `FromMetadata` equivalence against the source DMS. — **Validation**: GO
- **Conventional Commit subject**: `feat(cli): generate Go models from DMS`

### 82. Generate TypeScript models from DMS

- **Paths**: CLI codegen + golden tests — **Runtime scope**: go (tool) emitting ts — **Contract impact**: internal — **Backward compat**: additive — **Acceptance**: emitted `defineModel` schemas type-check under item 70's inference and register cleanly. — **Validation**: GO, TS (compile emitted goldens)
- **Conventional Commit subject**: `feat(cli): generate TypeScript models from DMS`

### 83. Generate Python models from DMS

- **Paths**: CLI codegen + golden tests — **Runtime scope**: go (tool) emitting py — **Contract impact**: internal — **Backward compat**: additive — **Acceptance**: emitted dataclasses build via `ModelDefinition.from_dataclass` and pass mypy strict. — **Validation**: GO, PY (compile emitted goldens)
- **Conventional Commit subject**: `feat(cli): generate Python models from DMS`

### 84. Add the DMS equivalence gate to TypeScript

- **Paths**: `ts/src/dms.ts` (model→DMS extraction + `assertModelsEquivalent`), tests — **Runtime scope**: ts — **Contract impact**: additive-runtime — **Backward compat**: additive — **Acceptance**: a TS model can be verified equivalent to a DMS document, matching Go's gate semantics. — **Validation**: TS
- **Conventional Commit subject**: `feat(ts): add DMS model equivalence gate`

### 85. Add the DMS equivalence gate to Python

- **Paths**: `py/src/theorydb_py/dms.py`, tests — **Runtime scope**: py — **Contract impact**: additive-runtime — **Backward compat**: additive — **Acceptance**: same gate semantics as Go/TS. — **Validation**: PY
- **Conventional Commit subject**: `feat(py): add DMS model equivalence gate`

### 86. Generate the contract-runner models from DMS with a drift gate

- **Paths**: `contract-tests/runners/{go,py}/` (replace hand-written `User`/`Order`/release-state models with generated ones), `scripts/verify-generated-models.sh` (regenerate-and-diff, mirroring the key-contract gate), rubric wiring — **Runtime scope**: all (harness) — **Contract impact**: scenario-change (harness models; observable behavior unchanged) — **Backward compat**: n/a — **Acceptance**: no hand-authored contract model remains; drift gate fails if generated output differs from committed. — **Validation**: CT, RUBRIC
- **Conventional Commit subject**: `feat(contract): generate runner models from DMS with drift gate`

### 87. Add `tabletheory init`

- **Paths**: `cmd/tabletheory/` (scaffold: DMS file + docker-compose + one CRUD program in the chosen language(s) + README), template fixtures, docs — **Runtime scope**: go (tooling) — **Contract impact**: internal — **Backward compat**: additive — **Acceptance**: `tabletheory init --lang <go|ts|py>` produces a directory that reaches a successful CRUD write against DynamoDB Local with one documented command. — **Validation**: GO + scaffold smoke test in CI
- **Conventional Commit subject**: `feat(cli): add init scaffold command`

### 88. Generate CDK table constructs from DMS

- **Paths**: CLI codegen (promoting the runners' DMS→CreateTable translation), golden tests, `docs/cdk/` — **Runtime scope**: go (tool) — **Contract impact**: internal — **Backward compat**: additive — **Acceptance**: `tabletheory gen --cdk` emits a construct whose table/GSI/TTL shape matches `tabletheory validate`'s reading of the DMS. — **Validation**: GO; `cdk synth` smoke in example
- **Conventional Commit subject**: `feat(cli): generate CDK table constructs from DMS`

### 89. Add a DynamoDB-Local variant to cdk-multilang

- **Paths**: `examples/cdk-multilang/` (local compose + run scripts consuming generated constructs where applicable), README — **Runtime scope**: all (example) — **Contract impact**: docs-only — **Backward compat**: n/a — **Acceptance**: the cross-language no-drift demo runs locally without an AWS account; stale paths in its README fixed. — **Validation**: example smoke run
- **Conventional Commit subject**: `feat(examples): add DynamoDB-Local variant to cdk-multilang`

### 90. Ship the CLI as a release asset

- **Paths**: `.github/workflows/release.yml` (+ prerelease), install docs — **Runtime scope**: none (release) — **Contract impact**: internal — **Backward compat**: additive — **Acceptance**: releases carry `tabletheory` binaries (linux/mac, amd64/arm64) alongside existing assets; hygiene checks updated to expect them. — **Validation**: HYG; prerelease dry-run on a test tag path
- **Conventional Commit subject**: `feat(release): publish tabletheory CLI as a release asset`

---

## I. Schema-migration parity (91–92) — maintainer opted in

### 91. Port the schema-migration story to TypeScript

- **Paths**: `ts/src/schema-migration.ts` (AutoMigrate, AddField/RenameField/RemoveField transforms, backup-table/data-copy options mirroring `pkg/schema`), `ts/docs/migration-guide.md` in the same commit — **Runtime scope**: ts — **Contract impact**: additive-runtime (table-shape tooling; no item-semantics scenario) — **Backward compat**: additive — **Acceptance**: the Go migration-guide walkthrough is reproducible in TS against DynamoDB Local. — **Validation**: TS + integration lane
- **Conventional Commit subject**: `feat(ts): add schema migration and transform support`

### 92. Port the schema-migration story to Python

- **Paths**: `py/src/theorydb_py/schema_migration.py`, `py/docs/migration-guide.md` (replacing the 17-line stub) — **Runtime scope**: py — **Contract impact**: additive-runtime — **Backward compat**: additive — **Acceptance**: same walkthrough reproducible in Python. — **Validation**: PY + integration lane
- **Conventional Commit subject**: `feat(py): add schema migration and transform support`

---

## J. Docs & onboarding build-out (93–98)

### 93. One-command Go local quickstart

- **Paths**: `Makefile` (`example-local` target or equivalent), `examples/` runnable Go CRUD program, `docs/getting-started.md` (docker-up + run command + complete program) — **Runtime scope**: go — **Contract impact**: docs-only — **Backward compat**: n/a — **Acceptance**: a newcomer reaches a verified CRUD write with two commands, matching the TS story. — **Validation**: run the target against DynamoDB Local
- **Conventional Commit subject**: `feat(examples): add one-command Go local quickstart`

### 94. Publish runtime doc trees into the site with an Examples nav

- **Paths**: `docs/_config.yml`, `docs/_data/nav.yml`, site plumbing to render `ts/docs` + `py/docs` (or synced copies with a drift check), new Examples nav group — **Runtime scope**: none (docs) — **Contract impact**: docs-only — **Backward compat**: n/a — **Acceptance**: TS/Py getting-started, API reference, troubleshooting are first-class site pages; examples discoverable from nav. — **Validation**: local Jekyll build; link check
- **Conventional Commit subject**: `docs(site): publish runtime doc trees and examples navigation`

### 95. Bring TS/Py troubleshooting and core-patterns toward Go parity

- **Paths**: `ts/docs/troubleshooting.md`, `ts/docs/core-patterns.md`, `py/docs/troubleshooting.md`, `py/docs/core-patterns.md` — **Runtime scope**: none (docs) — **Contract impact**: docs-only — **Backward compat**: n/a — **Acceptance**: each covers the same problem classes as the Go pages (errors, encryption, keys, pagination, testing) with runtime-native examples. — **Validation**: rubric DOC gates
- **Conventional Commit subject**: `docs: expand TypeScript and Python troubleshooting and patterns`

### 96. Generate API references from source

- **Paths**: docs tooling (godoc→md, TypeDoc, pdoc), CI wiring + drift check, replacing hand-written `api-reference.md` cores — **Runtime scope**: all (docs tooling) — **Contract impact**: docs-only — **Backward compat**: n/a — **Acceptance**: API reference regeneration is a build step; hand-drift is structurally impossible for signatures. — **Validation**: docs build; RUBRIC
- **Conventional Commit subject**: `feat(docs): generate API references from source`

### 97. Ship the generative-coding artifact pack

- **Paths**: `docs/llms.txt` (+ `llms-full.txt`), machine-readable tag/role/DMS vocabulary export (JSON), consumer rules template (copy-in `CLAUDE.md`/rules file), prompt recipes; linked from `tabletheory init` output — **Runtime scope**: none (docs/tooling) — **Contract impact**: docs-only — **Backward compat**: n/a — **Acceptance**: the README's generative-coding claim is backed by downloadable artifacts; LLM FAQ gains TS/Py samples. — **Validation**: rubric DOC gates
- **Conventional Commit subject**: `feat(docs): ship generative-coding artifacts (llms.txt, vocabulary, rules template)`

### 98. Include tag-DX fixes in model errors

- **Paths**: `pkg/model/registry.go` (field name in every tag/validation error; deprecation warning on `lsi-` name-prefix mechanism; pluralization documented + explicit-name steering), docs — **Runtime scope**: go — **Contract impact**: additive-runtime (error text; codes unchanged) — **Backward compat**: additive — **Acceptance**: a typo'd tag error names the offending field; LSI prefix mechanism warns toward the `lsi:` tag. — **Validation**: GO
- **Conventional Commit subject**: `fix(model): name the offending field in tag validation errors`

---

## K. Code-health consolidation (99–107) — after WS-F typed layers to avoid double churn

### 99. Deprecate the test-only MainExecutor

- **Paths**: `pkg/query/executor.go` (deprecation markers), docs — **Runtime scope**: go — **Contract impact**: internal — **Backward compat**: additive (removal at item 124) — **Acceptance**: `MainExecutor` marked deprecated with rationale; no production path references it (verified). — **Validation**: GO
- **Conventional Commit subject**: `refactor(query): deprecate test-only MainExecutor`

### 100. Unify the attribute-value conversion stacks

- **Paths**: `pkg/types/converter.go`, `internal/expr/converter.go` (one implementation, one delegating) — **Runtime scope**: go — **Contract impact**: internal — **Backward compat**: additive (public `Converter` surface preserved) — **Acceptance**: single conversion codepath; full suite + contract tests green (behavioral lock). — **Validation**: GO, CT
- **Conventional Commit subject**: `refactor(marshal): unify attribute-value conversion stacks`

### 101. Consolidate the triplicated anonymous-embed encoding

- **Paths**: `pkg/types/`, `pkg/marshal/`, `internal/expr/` — **Runtime scope**: go — **Contract impact**: internal — **Backward compat**: additive — **Acceptance**: one shared helper; three copies gone. — **Validation**: GO, CT
- **Conventional Commit subject**: `refactor: consolidate anonymous embed encoding helpers`

### 102. Decompose pkg/query/query.go

- **Paths**: `pkg/query/` (split 68 KB file along builder/executor/pagination seams; no exported-identifier changes) — **Runtime scope**: go — **Contract impact**: internal — **Backward compat**: additive — **Acceptance**: no file in `pkg/query` exceeds the repo's file-size budget; API surface byte-identical (`go doc` diff empty). — **Validation**: GO, RUBRIC (file-size gates)
- **Conventional Commit subject**: `refactor(query): decompose query.go along builder seams`

### 103. De-duplicate TypeScript builders

- **Paths**: `ts/src/query.ts`, `ts/src/update-builder.ts` (shared expression builder; shared pagination/aggregation helpers for Query/Scan) — **Runtime scope**: ts — **Contract impact**: internal — **Backward compat**: additive — **Acceptance**: operator switch and pagination/aggregation logic exist once; public API unchanged; suite green. — **Validation**: TS, CT
- **Conventional Commit subject**: `refactor(ts): extract shared expression and pagination helpers`

### 104. Enable type-aware ESLint in TypeScript

- **Paths**: `ts/eslint.config.js` (+ resulting fixes) — **Runtime scope**: ts — **Contract impact**: internal — **Backward compat**: additive — **Acceptance**: `recommended-type-checked` active; no-floating-promises clean. — **Validation**: TS
- **Conventional Commit subject**: `chore(ts): enable type-checked lint rules`

### 105. Consolidate Python coercion helpers

- **Paths**: `py/src/theorydb_py/{table,streams,attr_types,schema}.py` (single `_coerce_value`/`unwrap_optional` home) — **Runtime scope**: py — **Contract impact**: internal — **Backward compat**: additive — **Acceptance**: one implementation each; four copies gone; suite green. — **Validation**: PY, CT
- **Conventional Commit subject**: `refactor(py): consolidate coercion and optional-unwrap helpers`

### 106. Add TypeScript subpath exports for domain modules

- **Paths**: `ts/package.json` (exports `/facetheory`, `/release-state`, `/lease`), `ts/src/index.ts` (root re-exports kept + deprecated), docs — **Runtime scope**: ts — **Contract impact**: additive-runtime — **Backward compat**: additive (removal at item 122) — **Acceptance**: domain modules importable via subpaths; root imports warn in docs. — **Validation**: TS
- **Conventional Commit subject**: `feat(ts): add subpath exports for domain modules`

### 107. Implement real index selection in the TypeScript optimizer

- **Paths**: `ts/src/optimizer.ts` (port `pkg/index` selection semantics; keep `explain()` but grounded in actual analysis), tests, docs — **Runtime scope**: ts — **Contract impact**: additive-runtime — **Backward compat**: additive — **Acceptance**: `QueryOptimizer` selects indexes from conditions like Go's `AnalyzeConditions`/`SelectOptimal`; the name no longer over-promises. — **Validation**: TS
- **Conventional Commit subject**: `feat(ts): implement index selection in the query optimizer`

---

## L. Verifier consolidation & release-lane restructure (108–113) — WS-I, maintainer opted in; run between release cycles

### 108. Consolidate coverage/file-size/format verifiers

- **Paths**: `scripts/` (5 coverage → 1 parameterized, 4 file-size → 1, fmt duplicates merged), `gov-infra/verifiers/gov-verify-rubric.sh` call sites — **Runtime scope**: none (tooling) — **Contract impact**: internal — **Backward compat**: n/a — **Acceptance**: rubric output identical (36 checks, same evidence) with materially fewer scripts. — **Validation**: RUBRIC before/after diff
- **Conventional Commit subject**: `refactor(scripts): consolidate coverage, file-size, and format verifiers`

### 109. Merge overlapping release verifiers

- **Paths**: `scripts/verify-version-alignment.sh` + `verify-branch-version-sync.sh` → one; `verify-release-cycle-state.sh` + `watch-release-cycle.sh` share one core; policy self-tests updated — **Runtime scope**: none — **Contract impact**: internal — **Backward compat**: n/a — **Acceptance**: same PASS/WARN/FAIL judgments on a matrix of known-good/known-bad states (captured as policy self-test fixtures first). — **Validation**: HYG; RUBRIC
- **Conventional Commit subject**: `refactor(scripts): merge overlapping release verifiers`

### 110. Author the single-manifest release-lane design

- **Paths**: `docs/development/planning/tabletheory-release-lane-v2-design-2026.md` (new): single release-please manifest with native prerelease handling, tag-driven version source, generated TS/Py version files at release-build time, merge-queue adoption, guard retirement map, rollback plan, dry-run protocol — **Runtime scope**: none (design) — **Contract impact**: internal — **Backward compat**: n/a — **Acceptance**: design names every current guard and disposition (keep/merge/retire), and a between-cycles execution window. **Maintainer sign-off required before 111 executes.**
- **Validation**: rubric DOC gates
- **Conventional Commit subject**: `docs(release): design single-manifest release lane`

### 111. Consolidate release-please to a single manifest

- **Paths**: `release-please-config*.json`, `.github/workflows/{release-pr,release,prerelease-pr,prerelease}.yml`, affected `scripts/` — **Runtime scope**: none (release machinery) — **Contract impact**: internal — **Backward compat**: n/a for consumers; **high operational risk** — executed between release cycles per design 110, validated by a full RC→stable dry cycle on a sandbox repo or dispatch path before the first real cycle — **Acceptance**: one config/manifest drives RC and stable lanes; version alignment points reduced per design; a full staging→premain→main cycle completes green.
- **Validation**: HYG; `bash scripts/watch-release-cycle.sh --strict`; sandbox dry cycle
- **Conventional Commit subject**: `feat(release): consolidate release-please to a single manifest`

### 112. Adopt a merge queue and retire superseded provenance guards

- **Paths**: `.github/` branch-protection-adjacent workflow config, `release-hygiene.yml`, guard scripts per design 110's disposition map — **Runtime scope**: none — **Contract impact**: internal — **Backward compat**: n/a — **Acceptance**: promotions ride the merge queue; each retired guard's threat is documented as covered by queue semantics or a kept check. — **Validation**: HYG; test PR through the queue
- **Conventional Commit subject**: `feat(ci): adopt merge queue for protected branches`

### 113. Retire superseded release-lane scripts

- **Paths**: `scripts/` removals + `AGENTS.md`/docs updates — **Runtime scope**: none — **Contract impact**: internal — **Backward compat**: n/a — **Acceptance**: no orphaned scripts; release documentation matches the new lane end to end. — **Validation**: HYG; RUBRIC
- **Conventional Commit subject**: `refactor(scripts): retire superseded release-lane guards`

---

## M. v2.0.0 capstone (114–120) — WS-J, maintainer opted in; runs as a normal staging→premain→main cycle whose release-please bump is major

### 114. Author the v2 migration guide and deprecation audit

- **Paths**: `docs/migration/v2.md` (new), audit that every removal below shipped its deprecation in a 1.x release — **Runtime scope**: none (docs) — **Contract impact**: docs-only — **Backward compat**: n/a — **Acceptance**: every breaking change lists affected consumer classes (Theory Cloud products, Pay Theory, external), the exact rewrite, and verification steps; downstream coordination checklist included.
- **Validation**: rubric DOC gates
- **Conventional Commit subject**: `docs(v2): author migration guide and deprecation audit`

### 115. Remove the non-atomic Transaction API

- **Paths**: `pkg/core/interfaces.go`, `internal/theorydb/theorydb.go`, mocks, docs — **Runtime scope**: go — **Contract impact**: additive-runtime removal (atomicity semantics remain pinned by scenario 64) — **Backward compat**: **breaking** — justified: API performs non-atomic writes under a transactional name; deprecated since item 1; replacement `Transact()` pinned by scenario 64; `BREAKING CHANGE:` footer required
- **Acceptance**: `DB.Transaction`/`TransactionFunc` gone; migration guide section verified against a real consumer rewrite.
- **Validation**: GO, CT, RUBRIC
- **Conventional Commit subject**: `feat(core)!: remove non-atomic Transaction API in favor of Transact()`

### 116. Default TypeScript to precision-safe numbers

- **Paths**: `ts/src/marshal.ts` (flag default flip; lossy mode still opt-in-able), docs, contract driver — **Runtime scope**: ts — **Contract impact**: scenario-covered (13/14 pin both modes) — **Backward compat**: **breaking** — numeric return types change for out-of-range values; deprecated/announced via 14; `BREAKING CHANGE:` footer
- **Acceptance**: default mode passes the number-precision scenario; migration note covers consumers reading large numbers.
- **Validation**: TS, CT
- **Conventional Commit subject**: `feat(ts)!: default to precision-safe number unmarshaling`

### 117. Make `tabletheory_py` the canonical package

- **Paths**: `py/src/` (real modules under `tabletheory_py`; `theorydb_py` reduced to a warning shim or removed per migration guide), `py/pyproject.toml`, docs — **Runtime scope**: py — **Contract impact**: internal — **Backward compat**: **breaking** (import removal); deprecated since item 19; `BREAKING CHANGE:` footer
- **Acceptance**: canonical import everywhere; wheel/docs/scripts consistent; contract runner imports updated.
- **Validation**: PY, CT, `bash scripts/verify-python-build.sh`
- **Conventional Commit subject**: `feat(py)!: make tabletheory_py the canonical package`

### 118. Remove TypeScript root re-exports of domain modules

- **Paths**: `ts/src/index.ts`, `ts/package.json`, docs — **Runtime scope**: ts — **Contract impact**: internal — **Backward compat**: **breaking**; deprecated since item 106; `BREAKING CHANGE:` footer — **Acceptance**: root namespace is the generic ORM only; subpaths carry domain modules. — **Validation**: TS
- **Conventional Commit subject**: `feat(ts)!: move domain modules exclusively to subpath exports`

### 119. Remove deprecated any-typed option variants in Go

- **Paths**: `pkg/core/interfaces.go`, `internal/theorydb/`, mocks, docs — **Runtime scope**: go — **Contract impact**: internal — **Backward compat**: **breaking**; deprecated since item 69; typed replacements shipped in 1.x; `BREAKING CHANGE:` footer — **Acceptance**: `opts ...any` variants gone; typed surface is the only path. — **Validation**: GO, RUBRIC
- **Conventional Commit subject**: `feat(core)!: remove any-typed option method variants`

### 120. Remove MainExecutor

- **Paths**: `pkg/query/executor.go` and its coverage-test anchors — **Runtime scope**: go — **Contract impact**: internal — **Backward compat**: **breaking** (exported identifier removal); deprecated since item 99; `BREAKING CHANGE:` footer — **Acceptance**: one execution stack remains; coverage targets rebased on the production executor. — **Validation**: GO, CT, RUBRIC
- **Conventional Commit subject**: `feat(query)!: remove deprecated MainExecutor`

---

## Self-check (per skill checklist)

- Every contract-visible change has its scenario ordered first, capability-gated so each commit is green (13→14, 46→47–49, 51→52–54, 55→56–58, 59→60–62, 64→115, 40 before nothing runtime-visible changes encryption defaults).
- DMS-touching items (36, 42) precede dependent implementations (43–45) and run under evolve-dms discipline.
- Runtime items appear in Go → TS → Py order within every trio.
- Breaking items exist **only** in section M, each with a 1.x deprecation predecessor, justification, and `!` subjects with `BREAKING CHANGE:` footers; item 111 is operationally risky but not consumer-breaking and carries a design-first + dry-run requirement.
- Version manifests are never feature-commit targets; semver rides commit types (release-please cuts the v2 major from the `!` commits).
- No item depends on a later item to compile or pass; contingency markers flag where genuine divergences may add unplanned fix commits.
- Together the list satisfies every success criterion in the scoped-need document, including the maintainer's three scope expansions (v2 capstone, release-lane restructure, migration parity).

**Suggested next step:** `plan-roadmap` to sequence these 120 items into phases and milestone candidates (natural seams: A+B+C as "Trust & Floors", D+E as "Contract Depth", F+G as "Typed & Testable", H+I+J as "DMS Engine & Docs", K+L as "Consolidation & Lane", M as "v2").
