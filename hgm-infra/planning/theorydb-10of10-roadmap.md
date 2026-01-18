# theorydb: 10/10 Roadmap (Rubric v0.2)

This roadmap maps milestones directly to rubric IDs with measurable acceptance criteria and verification commands.

## Current scorecard (Rubric v0.2)
Scoring note: a check is only treated as “passing” if it is both green **and** enforced by a trustworthy verifier
(pinned tooling, schema-valid configs, and no “green by dilution” shortcuts). Completeness failures invalidate “green by
drift”.

| Category | Grade | Blocking rubric items |
| --- | ---: | --- |
| Quality | 10/10 | — |
| Consistency | 10/10 | — |
| Completeness | 10/10 | — |
| Security | 10/10 | — |
| Compliance Readiness | 10/10 | — |
| Maintainability | 10/10 | — |
| Docs | 10/10 | — |

Evidence (refresh whenever behavior changes):
- `bash hgm-infra/verifiers/hgm-verify-rubric.sh`
- `cat hgm-infra/evidence/hgm-rubric-report.json`
- See `hgm-infra/planning/theorydb-evidence-plan.md` for per-check refresh commands.

## Rubric-to-milestone mapping
| Rubric ID | Status | Milestone |
| --- | --- | --- |
| QUA-1 | PASS | M1.5 |
| QUA-2 | PASS | M1.5 |
| QUA-3 | PASS | M1.5 |
| QUA-4 | PASS | M1.5 |
| QUA-5 | PASS | M1.5 |
| CON-1 | PASS | M1 |
| CON-2 | PASS | M1 |
| CON-3 | PASS | M3 |
| COM-1 | PASS | M2 |
| COM-2 | PASS | M2 |
| COM-3 | PASS | M0 |
| COM-4 | PASS | M1 |
| COM-5 | PASS | M1.5 |
| COM-6 | PASS | M2 |
| COM-7 | PASS | M2 |
| COM-8 | PASS | M2 |
| SEC-1 | PASS | M2 |
| SEC-2 | PASS | M2 |
| SEC-3 | PASS | M2 |
| SEC-4 | PASS | M3 |
| SEC-5 | PASS | M3 |
| SEC-6 | PASS | M3 |
| SEC-7 | PASS | M3 |
| SEC-8 | PASS | M3 |
| SEC-9 | PASS | M3 |
| CMP-1 | PASS | M0 |
| CMP-2 | PASS | M0 |
| CMP-3 | PASS | M0 |
| MAI-1 | PASS | M4 |
| MAI-2 | PASS | M4 |
| MAI-3 | PASS | M4 |
| DOC-1 | PASS | M0 |
| DOC-2 | PASS | M0 |
| DOC-3 | PASS | M0 |
| DOC-4 | PASS | M0 |
| DOC-5 | PASS | M0 |

## Workstream tracking docs (when blockers require a dedicated plan)
Large remediation workstreams usually need their own roadmaps so they can be executed in reviewable slices and keep the
main roadmap readable:
- Lint remediation: `hgm-infra/planning/theorydb-lint-green-roadmap.md`
- Coverage remediation: `hgm-infra/planning/theorydb-coverage-roadmap.md`

## Milestones (sequenced)
### M0 — Freeze rubric + planning artifacts
**Closes:** COM-3, CMP-1..3, DOC-1..5  
**Goal:** prevent goalpost drift by making the definition of “good” explicit and versioned.

**Acceptance criteria**
- Rubric exists and is versioned.
- Threat model exists and is owned.
- Controls matrix exists and maps threats → controls.
- Evidence plan maps rubric IDs → verifiers → artifacts.
- Doc integrity + threat-controls parity checks are green (repo docs + HGM planning docs).

### M1 — Make core lint/build loop reproducible
**Closes:** CON-1, CON-2, COM-4  
**Goal:** strict lint/format enforcement with pinned tools; no drift.

Tracking document: `hgm-infra/planning/theorydb-lint-green-roadmap.md`

**Acceptance criteria**
- Formatter clean; lint green with schema-valid config; pinned tool versions; no blanket excludes.

### M1.5 — Coverage/quality gates
**Closes:** QUA-1..5, COM-5  
**Goal:** reach and maintain coverage floor (≥ 90%) without reducing scope; tests green.

Tracking document: `hgm-infra/planning/theorydb-coverage-roadmap.md`

### M2 — Security + anti-drift enforcement
**Closes:** COM-1, COM-2, COM-6..8, SEC-1..3  
**Goal:** tooling is pinned and security scans are reproducible.

### M3 — Domain P0 hardening (high-risk environments)
**Closes:** SEC-4..9, CON-3  
**Goal:** ensure domain-critical semantics (e.g., encrypted tag behavior) and public API parity stay enforced.

### M4 — Maintainability convergence
**Closes:** MAI-1..3  
**Goal:** keep code convergent to reduce future security/quality drift.

Notes:
- MAI-2 requires a repo-local maintainability roadmap under `hgm-infra/planning/` and should be updated after major refactors.

### M5 — Sunset legacy rubric runner (single entrypoint)
**Closes:** (meta; no rubric IDs)  
**Goal:** replace the legacy `scripts/verify-rubric.sh` orchestration with the HGM verifier without losing any checks.

**Acceptance criteria**
- `make rubric` continues to work unchanged.
- `scripts/verify-rubric.sh` delegates to `bash hgm-infra/verifiers/hgm-verify-rubric.sh`.
- All legacy rubric checks are still enforced via HGM (no lost gates).
