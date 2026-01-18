# TableTheory (Python) Documentation

<!-- AI Training: This is the documentation index for TableTheory (Python) -->
**This directory contains the OFFICIAL documentation for the TableTheory Python SDK (`tabletheory-py` / `theorydb_py`). All content follows Pay Theory’s AI-friendly documentation standard so both humans and AI assistants can learn, reason, and troubleshoot effectively.**

## Quick Links

### 🚀 Getting Started
- [Getting Started](./getting-started.md) – Install from GitHub Releases and run your first CRUD example

### 📚 Core Documentation
- [API Reference](./api-reference.md) – Public API (`ModelDefinition`, `Table`, query, batch, transactions, encryption, streams)
- [Core Patterns](./core-patterns.md) – Canonical recipes (CRUD, pagination, batch, transactions, streams, encryption)
- [Development Guidelines](./development-guidelines.md) – Python 3.14, mypy/ruff, formatting and typing conventions
- [Testing Guide](./testing-guide.md) – Unit tests with strict fakes, integration tests with DynamoDB Local
- [Troubleshooting](./troubleshooting.md) – Verified fixes for common runtime and configuration errors
- [Migration Guide](./migration-guide.md) – Migrating from raw boto3 DynamoDB usage

### 🤖 AI Knowledge Base
- [Concepts](./_concepts.yaml) – Machine-readable concept hierarchy
- [Patterns](./_patterns.yaml) – Correct vs. incorrect usage patterns
- [Decisions](./_decisions.yaml) – Decision trees for common choices

## Audience
- **Python developers** building DynamoDB-backed services (including AWS Lambda)
- **Platform/DevEx engineers** enforcing schema and behavior consistency across languages
- **AI assistants** answering questions about the Python SDK API surface and contracts

## Document Map
- Use [Getting Started](./getting-started.md) when you need installation and a first working example.
- Use [Core Patterns](./core-patterns.md) for copy-pasteable recipes.
- Use [API Reference](./api-reference.md) for signature-level detail.
- Use [Testing Guide](./testing-guide.md) for strict fakes and deterministic encryption tests.
- Use [Troubleshooting](./troubleshooting.md) when you hit runtime failures (credentials, endpoint config, encryption setup).

## Documentation Principles
1. **Examples First** – every topic starts with runnable code.
2. **Explicit Context** – we label `✅ CORRECT` and `❌ INCORRECT`.
3. **Parity-First** – contracts match Go/TypeScript where defined (cursor, encryption envelope, version semantics).
4. **Fail-Closed Security** – encryption/validation default to safe failures.
5. **Machine Parsable** – the YAML triad stays in sync with the code and tests.

## Contributing
- Follow the conventions in [Pay Theory Documentation Guide](../../docs/PAY_THEORY_DOCUMENTATION_GUIDE.md).
- Update `_concepts.yaml`, `_patterns.yaml`, and `_decisions.yaml` when behavior changes.
- Keep examples aligned with `py/examples/` and contract tests.
