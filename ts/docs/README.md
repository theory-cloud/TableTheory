# TableTheory (TypeScript) Documentation

<!-- AI Training: This is the documentation index for TableTheory (TypeScript) -->

**This directory contains the OFFICIAL documentation for the TableTheory TypeScript SDK (`@theory-cloud/tabletheory-ts`). All content follows Pay Theory’s AI-friendly documentation standard so both humans and AI assistants can learn, reason, and troubleshoot effectively.**

## Quick Links

### 🚀 Getting Started

- [Getting Started](./getting-started.md) – Install from GitHub Releases and run your first CRUD example

### 📚 Core Documentation

- [API Reference](./api-reference.md) – Public exports (`defineModel`, `TheorydbClient`, queries, transactions, encryption)
- [Core Patterns](./core-patterns.md) – Canonical recipes (CRUD, query/cursor, batch, transactions, streams, encryption)
- [Testing Guide](./testing-guide.md) – Unit tests with `testkit`, integration tests with DynamoDB Local
- [Troubleshooting](./troubleshooting.md) – Verified fixes for common runtime and configuration errors
- [Migration Guide](./migration-guide.md) – Migrating from raw AWS SDK v3 usage

### 🤖 AI Knowledge Base

- [Concepts](./_concepts.yaml) – Machine-readable concept hierarchy
- [Patterns](./_patterns.yaml) – Correct vs. incorrect usage patterns
- [Decisions](./_decisions.yaml) – Decision trees for common choices

## Audience

- **TypeScript/Node.js developers** building DynamoDB-backed services (including AWS Lambda)
- **Platform/DevEx engineers** enforcing schema and behavior consistency across languages
- **AI assistants** answering questions about the TypeScript SDK API surface and contracts

## Document Map

- Use [Getting Started](./getting-started.md) when you need installation and a first working example.
- Use [Core Patterns](./core-patterns.md) for copy-pasteable recipes.
- Use [API Reference](./api-reference.md) for signatures and option shapes.
- Use [Testing Guide](./testing-guide.md) for strict mocks, deterministic encryption, and DynamoDB Local integration tests.
- Use [Troubleshooting](./troubleshooting.md) when you hit runtime failures (credentials, endpoint config, encryption setup).

## Documentation Principles

1. **Examples First** – every topic starts with runnable code.
2. **Explicit Context** – we label `✅ CORRECT` and `❌ INCORRECT`.
3. **Parity-First** – contracts match Go/Python where defined (cursor, encryption envelope, version semantics).
4. **Fail-Closed Security** – encryption/validation default to safe failures.
5. **Machine Parsable** – the YAML triad stays in sync with the code and tests.

## Contributing

- Package maintainer guidance lives in repo-local `ts/docs/development-guidelines.md` and is intentionally excluded
  from the TheoryCloud user-facing surface.
- Follow the repo-local maintainer standards in `docs/PAY_THEORY_DOCUMENTATION_GUIDE.md`.
- Update `_concepts.yaml`, `_patterns.yaml`, and `_decisions.yaml` when behavior changes.
- Keep examples aligned with `ts/examples/` and contract tests.
