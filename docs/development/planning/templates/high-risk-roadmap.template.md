# Roadmap Template (High-Risk Domains, Rubric v0.1)

This template is intended to be copied and filled per project.

## Current scorecard (Rubric v0.1, YYYY-MM-DD)

| Category | Grade | Blocking rubric items |
| --- | ---: | --- |
| Security | [x/10] | [SEC-*] |
| Privacy | [x/10] | [PRV-*] |
| Compliance Readiness | [x/10] | [CMP-*] |
| Maintainability | [x/10] | [MAI-*] |

Evidence (most recent):

- [commands run and/or artifact links]

## Rubric-to-milestone mapping

| Rubric ID | Status | Milestone |
| --- | --- | --- |
| SEC-1 | ⬜ | [M?] |

## Milestones (map directly to rubric IDs)

### M0 — Freeze rubric + scope (done when versioned)

**Closes:** [CMP-1? CMP-2?]  
**Goal:** prevent goalpost drift and clarify what is in-scope.

**Acceptance criteria**
- Rubric is versioned and deterministic.
- Scope statement exists and is approved.

**Suggested verification**
```bash
[commands]
```

---

### M1 — Implement P0 gates (security regression prevention)

**Closes:** [SEC-4]  
**Goal:** block regressions while high-risk work is landing.

**Acceptance criteria**
- CI fails on new violations of P0 controls.
- Error messages are actionable and point to the failing control.

**Suggested verification**
```bash
[commands]
```

---

### M1.5 — Branch + release supply chain (protected branches + automation)

**Closes:** [SEC-5]  
**Goal:** make releases reproducible and reduce CI/CD supply-chain risk (protected branches, automated release/prerelease).

**Acceptance criteria**
- Protected branches are defined and enforced (required status checks, no direct pushes).
- Automated release/prerelease workflows exist and produce tags/changelogs deterministically.

**Suggested verification**
```bash
[commands or artifact checks]
```

---

### M2 — Close top control gaps (highest risk first)

**Closes:** [SEC-* PRV-* CMP-*]  
**Goal:** deliver concrete control improvements with evidence.

**Acceptance criteria**
- Controls are implemented and verified.
- Evidence artifacts are produced and stored.

**Suggested verification**
```bash
[commands]
```

---

### M3 — Maintainability convergence (keep future work safe)

**Closes:** [MAI-*]  
**Goal:** reduce structural debt that makes high-risk changes hard to review or easy to drift over time.

**Acceptance criteria**
- Maintainability gates are measurable and enforced (file-size budgets, convergence checks).
- Duplicate critical-path implementations are removed or reduced to delegators.

**Suggested verification**
```bash
[commands]
```
