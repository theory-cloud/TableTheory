# High-Risk Domain Planning Standard (Rubric + Roadmap + Evidence)

This is a reusable process for turning “high-risk domain” requirements (payments, healthcare, etc.) into **versioned,
measurable, repeatable** engineering work.

This is not legal advice and does not replace compliance or security professionals. Treat it as an engineering
execution pattern: scope → controls → gates → evidence.

## Inputs (what must be decided up front)

- **Domain + frameworks**: e.g., PCI DSS (payments), HIPAA (healthcare), SOC 2 (trust services), etc.
- **Data classification**: CHD/SAD, PAN tokens, PHI, PII, secrets, telemetry.
- **Scope boundaries**: which services, accounts, environments, and third parties are in-scope.
- **Assurance target**: “best effort hardening” vs “audit-ready evidence” vs “certification/attestation”.
- **Branch + release model**: release vs prerelease branches, required protections, and automation triggers (part of the supply chain).

## Outputs (the standardized artifacts)

- **Controls matrix**: framework requirement → system control → verification → evidence.
- **Rubric**: deterministic pass/fail scoring, versioned (prevents goalpost drift).
- **Roadmap**: milestones mapped directly to rubric IDs (keeps execution honest).
- **Evidence plan**: where audit artifacts live and how they’re generated/re-generated.
- **Gates**: CI/local verifiers that block regressions for the highest-risk controls.
- **Public boundary inventory** (recommended): list exported entry points + validation/semantic contracts (used to drive contract tests and prevent drift).
- **Branch/release policy** (recommended): protected branches + release automation expectations (treat as part of the supply chain).
- **Maintainability plan** (recommended for AI-generated codebases): explicit convergence goals (avoid duplicate implementations), file-size budgets, and refactor milestones that keep the code reviewable over time.

Templates live in:

- `docs/development/planning/templates/high-risk-controls-matrix.template.md`
- `docs/development/planning/templates/high-risk-rubric.template.md`
- `docs/development/planning/templates/high-risk-roadmap.template.md`

## Workflow (repeatable)

### Step 0 — Scope and invariants

Write a 1-page scope statement:

- What data exists, where it flows, and what is explicitly out of scope.
- What environments are in scope (dev/staging/prod) and what “prod-like” means.
- Which third parties are in scope and what evidence you can realistically obtain from them.
- Which branches are release/prerelease sources and which CI gates must be required before merging.

### Step 1 — Build the controls matrix (requirements → controls)

Start from the framework(s) and list every applicable requirement. For each requirement, record:

- the concrete **control** you will implement (code/infra/process),
- how you will **verify** it (tests, CI gates, monitors, manual checks),
- what **evidence** is produced (logs, reports, configs, diagrams, tickets).

If you maintain local standards knowledge-bases, keep the raw standard text **out of the repo** when licensing or
distribution is uncertain, and link to the local path instead.

Recommended: set a local env var like `PCI_KB_PATH` pointing at your PCI DSS KB (example on one machine:
`/home/aron/Downloads/pci/knowledge-base/pci-dss-v4.0.1`).

### Step 2 — Freeze a rubric version (no moving goalposts)

Convert the controls matrix into a small rubric:

- 0–10 per category.
- fixed point weights; pass/fail items only.
- each item has a single “how to verify” source of truth (command or deterministic artifact check).
- include a **Maintainability** category when structural drift is a real risk (duplicate implementations, god files, unclear canonical paths).
- if you introduce “security-affordance” flags/tags (e.g., `encrypted`, `redacted`, `masked`), add a rubric item that ensures they have **enforced semantics** (no metadata-only false positives).
- if the system exposes multiple entry points (e.g., exported helpers + internal engines), add a rubric item that enforces **public API contract parity** (no silent semantic drift).

### Step 3 — Map rubric items to milestones (roadmap)

Create milestones that each close specific rubric IDs (no “floating” work). For each milestone:

- acceptance criteria are measurable,
- verification commands are listed,
- evidence locations are defined.

### Step 4 — Add gates for P0 controls

For highest-risk controls, add CI-enforceable gates (examples: regression baselines, denylist patterns, IaC assertions,
contract drift checks, protected-branch + release automation checks). The goal is to stop backsliding while improvements are in progress.

### Step 5 — Iterate with evidence

After each milestone:

- run the verifiers,
- store/refresh evidence,
- update the rubric scorecard (with the rubric version noted).
- record newly discovered “rubric blind spots” as candidate rubric items (with a proposed verifier) so one-off findings turn into durable gates.
