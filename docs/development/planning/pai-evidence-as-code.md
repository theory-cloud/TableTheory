# Evidence-as-Code (High-Risk Domains)

This document describes a concept for standardizing “transparent controls” in high-risk domains (payments, healthcare,
etc.) by generating **traceable, reproducible evidence** of controls (or gaps) as part of normal development.

This is **not** a compliance certification system. It is “evidence-as-code”: a repeatable way to answer:

- what controls exist today,
- what do we know,
- what can we prove,
- what is missing or unknown.

## Core problem

High-risk domains fail in predictable ways:

- requirements live in PDFs/spreadsheets and drift from engineering reality,
- evidence is assembled late (audit scramble),
- “we have a control” is asserted without durable proof,
- ownership is ambiguous (controls degrade silently).

Evidence-as-Code turns standards into an engineering execution loop:

1. derive a **controls matrix** from a domain framework (PCI/HIPAA/etc),
2. freeze a **versioned rubric** (deterministic scoring; no moving goalposts),
3. generate a **roadmap** mapped to rubric IDs (work is measurable),
4. install **CI gates + evidence runners** that continuously produce evidence artifacts.

## Principles

- **Evidence beats assertion**: don’t mark controls “implemented” without referenced evidence.
- **Determinism first**: prefer machine-verifiable checks (tests, static analysis, IaC assertions) over manual checklists.
- **Traceability**: every claim points to:
  - a requirement ID,
  - a concrete control definition,
  - a verification mechanism,
  - evidence artifacts produced from a known commit/environment.
- **Versioned definitions**: rubrics must be versioned to prevent goalpost drift.
- **Scoped by design**: controls are meaningful only when scope (data flows, systems, environments) is explicit.

## Key artifacts

These artifacts should be produced in-repo (docs/backlog) while evidence artifacts may be stored in CI artifact storage.

### 1) Controls matrix

A table mapping framework requirements to concrete controls.

Template: `docs/development/planning/templates/high-risk-controls-matrix.template.md`

### 2) Rubric (versioned)

Deterministic, pass/fail scoring by category. The rubric is the definition of “good”, and it is explicitly versioned.

Template: `docs/development/planning/templates/high-risk-rubric.template.md`

### 3) Roadmap mapped to rubric IDs

Milestones that each close specific rubric IDs with measurable acceptance criteria, verification commands, and evidence
locations.

Template: `docs/development/planning/templates/high-risk-roadmap.template.md`

### 4) Evidence bundles

Deterministic outputs produced by CI or a local runner, ideally stored as build artifacts and referenced from the repo.

Examples:

- security scan reports,
- dependency vulnerability reports,
- IaC policy diffs/assertion results,
- configuration snapshots,
- contract verification results,
- regression reports (denylist patterns, log scrubbers).

## Standards text and licensing

If you maintain local standards knowledge-bases, keep the raw standard text **out of the repo** when licensing or
distribution is uncertain. Store only IDs + short titles + links/paths to the local/internal KB.
