# Planning (Assurance & Security)

This folder standardizes how we measure and maintain **quality, consistency, completeness, security, and maintainability** for
**TableTheory** (a repository that is largely AI-generated).

The goal is to prevent “green by drift” (weakened gates, excluded scopes, unpinned tools) by making expectations
**versioned, measurable, and repeatable**.

Start here:

- `docs/development/planning/high-risk-process.md` (generic framework → controls → gates → evidence loop)
- `docs/development/planning/theorydb-controls-matrix.md` (what we care about, mapped to verifiers)
- `docs/development/planning/theorydb-10of10-rubric.md` (the definition of “good”; versioned)
- `docs/development/planning/theorydb-10of10-roadmap.md` (milestones mapped to rubric IDs)
- `docs/development/planning/theorydb-multilang-roadmap.md` (multi-language expansion plan; TypeScript first)
- `docs/development/planning/theorydb-facetheory-support-roadmap.md` (FaceTheory enablement: ISR locks + cache metadata)
- `docs/development/planning/theorydb-spec-dms-v0.1.md` (language-agnostic schema + semantics contract, draft)
- `docs/development/planning/theorydb-go-ts-parity-matrix.md` (feature parity tiers for TypeScript)
- `docs/development/planning/theorydb-multilang-feature-parity-matrix.md` (feature parity across Go/TS/Py)
- `docs/development/planning/theorydb-multilang-verification-parity-roadmap.md` (rubric/CI parity across Go/TS/Py)
- `docs/development/planning/theorydb-multilang-verification-parity-matrix.md` (what we measure + rubric gate mapping)
- `docs/development/planning/theorydb-contract-tests-suite-outline.md` (runnable shared contract test suite outline)
- `contract-tests/README.md` (seed contract-tests repo skeleton + fixtures)
- `docs/development/planning/theorydb-lint-green-roadmap.md` (execution plan to get `make lint` green)
- `docs/development/planning/theorydb-coverage-roadmap.md` (execution plan to reach 90% library coverage)
- `docs/development/planning/theorydb-encryption-tag-roadmap.md` (execution plan to implement `theorydb:"encrypted"` safely)
- `docs/development/planning/theorydb-maintainability-roadmap.md` (execution plan to decompose + converge critical paths)
- `docs/development/planning/theorydb-evidence-plan.md` (where evidence comes from + how to regenerate)
- `docs/development/planning/theorydb-threat-model.md` (threats + mitigations for this codebase)
- `docs/development/planning/theorydb-branch-release-policy.md` (branch/release strategy for supply-chain hardening)
- `docs/development/planning/ai-drift-recovery.md` (common AI failure modes + how we recover)

Templates:

- `docs/development/planning/templates/high-risk-controls-matrix.template.md`
- `docs/development/planning/templates/high-risk-rubric.template.md`
- `docs/development/planning/templates/high-risk-roadmap.template.md`
- `docs/development/planning/templates/high-risk-branch-release-policy.template.md`

Notes:

- If you reference external standards text, keep it **out of the repo** when licensing/distribution is uncertain.
- Prefer verifiers that are runnable locally and in CI (tests, static analysis, deterministic doc checks).
