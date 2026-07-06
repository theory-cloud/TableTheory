<!-- AI Training: Internal planning document — roadmap for the 2026-07 product strengthening program -->

# Roadmap: TableTheory Product Strengthening Program (2026-07)

**Descends from:** `tabletheory-improvement-program-enumerated-changes-2026-07.md` (120 items)
**Scoped need:** `tabletheory-improvement-program-scoped-need-2026-07.md` (maintainer-ratified 2026-07-02)
**Milestone sizing:** 5–9 issues per milestone (maintainer preference) — 18 milestones, all within range, sum = 120.

## Goal

Deliver the full product strengthening program: eliminate the known correctness traps, make TableTheory installable and typed on every current platform, deepen the contract until the parity promise is actually pinned (queries, GSIs, pagination, types, encryption, errors), turn the DMS into a generating engine with a consumer CLI, give consumers a real testing story, consolidate a decade's worth of duplicated internals and release machinery, and close with a coordinated v2.0.0 that completes every deprecation. Six phases, six release checkpoints, ending at v2.0.0.

## Affected runtimes

All three (Go, TypeScript, Python), plus contract harness, DMS/spec, docs site, examples, CLI, CI/release machinery.

## Contract and DMS impact

- New/extended P0 scenarios: number precision (13), type matrix (35), encryption fail-closed + interop (40), version-conflict error (46).
- New P1 tier: query semantics (32), GSI/projection (33), pagination (34), snake_case naming (37), count (51), optional-get (55), transact-get (59).
- New P2 tier: batch semantics (63), generic transactions (64).
- Harness growth: query/scan ops (30), exact-match assertions (31), interop seeding (39), numeric assertions (12), fake-backed execution lane (77), generated models with drift gate (86).
- DMS/spec moves (evolve-dms discipline): naming-convention enum alignment (36), key-contract v0.2 transforms (42).

## Backward compatibility

Additive-first through 1.x with paired deprecations; breaking changes exist only in Phase 6 (items 115–120), each with a 1.x deprecation predecessor (1, 14, 19, 69, 99, 106), a migration guide (114), and downstream coordination. See Deprecation plan.

---

## Phases

### Phase 1: Trust & Floors — the foundation stops lying and starts installing

Runs first; everything here is independent of the contract-depth work and buys immediate credibility. M1–M4 are mutually parallelizable.

- **M1 `correctness-traps`** — No known correctness trap ships in the foundation.
  - Items: 1, 2, 3, 4, 5, 6
  - Dependencies: none (program start)
  - Risks: item 6 converts silent Python behavior to a loud error — watch downstream Theory Cloud Python consumers on the RC; item 1's deprecation text must name the v2 horizon accurately.

- **M2 `docs-truth`** — No published document claims an API or behavior that does not exist.
  - Items: 7, 8, 9, 10, 15
  - Dependencies: none
  - Risks: low; KB re-sync timing (item 7 feeds the theorycloud KB subtree — confirm refresh cadence so stale claims don't linger post-merge).

- **M3 `floors-and-install`** — TableTheory installs on current LTS platforms with an automated consumer update path.
  - Items: 16, 17, 18, 19, 20, 21, 22
  - Dependencies: none
  - Risks: peer-deps (18) needs an RC validated by a real tarball consumer (AppTheory or a sandbox app); Node 20 dual-build (17) can surface ESM/CJS type-resolution edge cases — smoke both module systems in CI before promoting.

- **M4 `contributor-loop`** — A contributor gets a green fast gate without Docker, and CI runs each suite exactly once per staging PR.
  - Items: 23, 24, 25, 26, 27, 28, 29
  - Dependencies: none (item 24 references 23's `rubric-fast` — same milestone, ordered)
  - Risks: 25 changes required checks — branch protection must be updated in the same change or staging PRs wedge; validate on a throwaway PR first.

### Phase 2: Contract Depth — pin what the product promises

The parity promise becomes enforced fact. M5 is the keystone; M6–M8 consume it.

- **M5 `harness-foundations`** — The harness can express queries, exact matches, cross-runtime interop, and full numerics identically in all three runners.
  - Items: 11, 12, 30, 31, 38, 39
  - Dependencies: none hard; 29 (M4) provides the `make contract-tests` entry point
  - Risks: 30 is the single most expensive harness change (three parallel implementations) — the Python runner needs decomposition from its single 624-line file while touched; keep op semantics byte-documented in the suite outline to prevent harness drift.

- **M6 `pin-the-core`** — Numbers, query semantics, GSIs, pagination, and the full type matrix are pinned cross-runtime.
  - Items: 13, 14, 32, 33, 34, 35
  - Dependencies: M5 (ops + numeric assertions)
  - Risks: **[contingency-heavy]** — 33 and 35 are the likeliest to expose real divergences; each becomes an unplanned `debug-parity-drift` fix commit. Budget slack here, not later.

- **M7 `spec-and-key-parity`** — Spec/code drift is resolved and encryption plus derived keys reach full three-runtime parity.
  - Items: 36, 37, 40, 41, 42, 43, 44, 45
  - Dependencies: M5 (interop seeding for 40); 36 and 42 run under evolve-dms; 43→44→45 strictly ordered after 42
  - Risks: 36 needs the consumer check on `pascalCase` before direction is fixed; 40's deterministic providers must stay test-scope-only (fail-closed default untouched — non-negotiable); generated-TS drift gate forces 43 to regenerate in-commit.

- **M8 `error-taxonomy`** — Consumers can programmatically distinguish error causes and read actionable model errors, identically across runtimes.
  - Items: 46, 47, 48, 49, 50, 98
  - Dependencies: M5; 46 lands before 47–49 (parity order); 49 flips the gate to required
  - Risks: `ErrVersionConflict` must keep `errors.Is(err, ErrConditionFailed)` true (additive wrapping) or existing consumer retry logic breaks — this is the milestone's one sharp edge.

### Phase 3: Parity & Types — match the competition where evaluations happen

M9/M10 extend the harness they ride on; M11 can start in parallel with Phase 2 (no hard deps). M12 closes the phase.

- **M9 `count-and-optional-get`** — Native count and no-error optional get exist in all three runtimes, scenario-pinned.
  - Items: 51, 52, 53, 54, 55, 56, 57, 58
  - Dependencies: M5 (query op plumbing); scenarios 51/55 precede their trios
  - Risks: low; Go's existing `Count()` semantics change from paginate-and-count to `Select=COUNT` — verify identical results under filters before advertising.

- **M10 `transactions-batches-iteration`** — The throughput surface is complete and pinned: TransactGet, batch semantics, generic transaction atomicity, lazy iteration.
  - Items: 59, 60, 61, 62, 63, 64, 65, 66
  - Dependencies: M5; 59 precedes 60–62; 64 is the atomicity pin that Phase 6's item 115 depends on
  - Risks: 63/64 **[contingency]** — batch-retry and cancellation-reason mapping are historically divergent areas; Go's extension-interface pattern (60) must not touch existing exported interfaces.

- **M11 `typed-apis`** — Every runtime gains a compile-time-checked API layer with unchanged observable error semantics.
  - Items: 67, 68, 69, 70, 71, 72, 73, 74
  - Dependencies: none hard (parallelizable with Phase 2); soft: before M13 (generated TS models should compile under 70's inference) and before M16 (avoid double churn)
  - Risks: 70 is the hardest single item in the program (type-level inference from a `const` schema; TS compiler-version sensitivity and compile-time cost) — guard with type-test fixtures including `@ts-expect-error` cases; 69's extension-interface design decides what v2 removes (119).

- **M12 `consumer-testing`** — A consumer writes behavioral tests without Docker in every runtime, against a fake validated by the contract corpus.
  - Items: 75, 76, 77, 78, 79
  - Dependencies: 75→76→77 ordered; 77 wants M6's enriched corpus (soft — fake lane grows as the corpus grows); 78/79 mirror 76's behavior
  - Risks: the fake is a long-lived behavioral commitment — 77's contract-lane validation is the mitigation; without it the fake becomes a fourth drifting implementation.

### Phase 4: DMS Engine & Docs — turn the spec into the product

- **M13 `dms-tooling`** — One DMS file validates and generates equivalent models in all three languages, each runtime able to prove code==spec.
  - Items: 80, 81, 82, 83, 84, 85
  - Dependencies: M11 soft (82 emits inference-compatible schemas); 84/85 mirror Go's existing gate
  - Risks: codegen fidelity — generated models must round-trip the equivalence gates, which is the acceptance test; keep generation deterministic (no timestamps) for drift-gating.

- **M14 `cli-productization`** — The consumer tooling story ships: generated contract models, init scaffold, CDK generation, a released CLI, and schema-migration parity.
  - Items: 86, 87, 88, 89, 90, 91, 92
  - Dependencies: M13 (86 consumes 81–83; 87/88 consume the CLI); 90 touches release workflows — coordinate with the release calendar; 91/92 are parallel siblings
  - Risks: 90 modifies release asset expectations — hygiene checks must learn the new assets in the same change or promotions fail; 91/92 are the largest single items after 70 (migration story ×2 runtimes) — timebox and split transforms if needed.

- **M15 `docs-build-out`** — The docs site serves all three runtimes at equal depth, references are generated, and the generative-coding claim is backed by artifacts.
  - Items: 93, 94, 95, 96, 97
  - Dependencies: M11 soft (96 documents the typed APIs); M14 soft (97 links from `init` output)
  - Risks: 96's generator pipeline must not regress the site build — land behind the existing DOC gates; 94 changes site structure, so link-check before merge.

### Phase 5: Consolidation & Lane — pay down the machine

- **M16 `internal-consolidation`** — Duplicated internals exist once, and the public TS surface is clean.
  - Items: 99, 100, 101, 102, 103, 104, 105, 106, 107
  - Dependencies: M11 hard-ish (avoid refactoring surfaces the typed layers just reshaped); behavior locked by the now-deep contract corpus
  - Risks: 100/102 are large behavior-preserving refactors — the contract suite is the lock; run full CT on every commit here without exception.

- **M17 `release-lane-v2`** — One manifest, a merge queue, and a consolidated guard set drive the release lane.
  - Items: 108, 109, 110, 111, 112, 113
  - Dependencies: 110 (design + maintainer sign-off) gates 111–113; **must run between release cycles** — schedule after the Phase 4/5 stable release ships and before the v2 cycle opens
  - Risks: highest operational risk in the program. Mitigations are structural: design-first with a guard disposition map (110), sandbox dry cycle before the first real cycle (111), merge-queue trial on staging only (112), rollback = revert the config PR before any promotion starts. A failed mid-cycle state is the one unrecoverable-by-revert scenario — hence the between-cycles window is a hard constraint, not a preference.

### Phase 6: v2 Capstone — complete the deprecations

- **M18 `v2-capstone`** — v2.0.0 ships with every queued removal completed, a verified migration guide, and coordinated downstream upgrades.
  - Items: 114, 115, 116, 117, 118, 119, 120
  - Dependencies: every deprecation predecessor shipped and soaked (1←M1, 14←M6, 19←M3, 69←M11, 99/106←M16); 64 (M10) pins the atomicity semantics 115 relies on; 114 lands first in the milestone
  - Risks: unknown external pinned consumers — mitigated by ≥2 minor releases of deprecation warnings, the migration guide verified against a real consumer rewrite (115 acceptance), and an extended RC soak with a downstream validation checklist.

---

## Release rollout plan

Six release checkpoints, one per phase, each riding the standard lane (`staging` → `premain` RCs → soak → `main` stable → back-merge):

| Checkpoint | After | Expected version | RC soak | Required RC validation |
|---|---|---|---|---|
| R1 | M1–M4 | v1.11.0 | ~1 week | AppTheory or sandbox app installs the TS tarball with peer deps on Node 20; a Py 3.12 consumer runs the suite; downstream Py consumers confirm item 6's union error surfaces nothing real |
| R2 | M5–M8 | v1.12.0 | ~1 week | All three runners green on the expanded corpus in CI and on one downstream consumer's models; KEY-M1 fixtures validated by theory-mcp-server's steward |
| R3 | M9–M12 | v1.13.0 | ~1–2 weeks | Typed-API smoke by one Go and one TS downstream consumer; fake-based test suite adopted in at least one downstream repo |
| R4 | M13–M15 | v1.14.0 | ~1 week | `tabletheory init`/`gen` exercised end-to-end by a fresh user; CLI assets verified on the RC release |
| R5 | M16–M17 | v1.15.0 | ~2 weeks (lane change soaks here) | One full RC→stable cycle through the NEW lane machinery is itself the validation; watch-release-cycle strict green throughout |
| R6 | M18 | **v2.0.0** | **3–4 weeks** | Every Theory Cloud product upgrades against the RC; Pay Theory validation via maintainer; migration guide walked end-to-end by at least one external-shaped consumer |

Release-please derives bumps from the Conventional Commit stream (feat → minor per checkpoint; the `!` commits in M18 drive the major). Feature commits never touch version manifests. If an RC is bad: land the fix on `staging`, re-promote, next `rc.N` — never retag (immutable releases).

## Version-bump implication

Five minor releases (v1.11.0–v1.15.0), then **major v2.0.0**. Justification: all Phase 1–5 work is additive with deprecations; Phase 6 removes six deprecated surfaces (`BREAKING CHANGE:` footers on items 115–120).

## Cross-phase risks

1. **Contingency fix-commits from new scenarios** (M6, M7, M10) are unplanned by definition — hold slack in those milestones; every divergence runs `debug-parity-drift` before its fix.
2. **Corpus-as-behavior-lock circularity:** M16's refactors trust the corpus built in Phase 2–3; if Phase 2 under-delivers, M16's safety margin shrinks. Do not reorder M16 ahead of M6.
3. **Rubric growth:** items 77, 86, 96 add gates to `gov-verify-rubric.sh` — each must extend the verifier in its own commit, never weaken an existing gate.
4. **Program length vs release cadence:** six checkpoints over multiple quarters; the `main`→`staging` back-merge discipline after every stable is what keeps feature branches honest — enforce it at every checkpoint.
5. **Single maintainer/steward loop capacity:** phases are internally parallelizable but the review loop is serial; milestone WIP should be 1–2 at a time despite the parallel lanes noted.

## Cross-repo coordination

- **theory-mcp-server:** KEY-M1 v0.2 transforms (M7) and Python evaluator (41) touch its derived-key fixtures — its steward should validate the R2 RC.
- **AppTheory / FaceTheory / KnowledgeTheory / Autheory:** RC validation duties per the rollout table; v2 upgrade coordination through the maintainer before R6 stable. FaceTheory additionally cares about item 106/118 (ISR store moves to a subpath).
- **Pay Theory:** coordination via the maintainer only; flag R1 (peer deps), R3 (typed APIs), R6 (v2) as the releases needing their attention.
- **External consumers:** reached through changelog, release notes, deprecation warnings, and the migration guide — no direct channel exists; this is why deprecations ship ≥2 minors before removal.

## Deprecation plan

| Deprecated (release) | Removed (v2.0.0) |
|---|---|
| `DB.Transaction`/`TransactionFunc` — R1 (item 1) | item 115 |
| TS lossy number mode as default — R2 (item 14 ships opt-in) | item 116 flips default |
| `theorydb_py` import — R1 (item 19 adds canonical alias) | item 117 |
| TS root re-exports of domain modules — R5 (item 106) | item 118 |
| Go `any`-typed option variants — R3 (item 69) | item 119 |
| `MainExecutor` — R5 (item 99) | item 120 |

Note: items 99/106 deprecate at R5, giving only one minor of warning before v2. If the maintainer wants a uniform ≥2-minor window, pull 99 and 106 forward into M11's release (R3) — they are independent of the rest of M16. **Flagged as an open question.**

## Open questions

1. **Deprecation window for 99/106** — pull into R3 for a uniform two-minor window, or accept one minor of warning? (Steward lean: pull forward; they are cheap commits.)
2. **M17 scheduling** — confirm the between-cycles window after R4; if the release calendar is hot, M17 can swap after M18's RC opens, but never mid-cycle.
3. **Item 36 consumer check** — the `pascalCase` direction (spec-add vs code-removal) must be answered before M7 starts.
4. **R6 soak length and downstream checklist owners** — 3–4 weeks proposed; needs maintainer confirmation and named validators per product.
5. **Milestone WIP limit** — roadmap assumes 1–2 concurrent milestones given the two-party loop; confirm before Linear creation so the project isn't over-parallelized.
