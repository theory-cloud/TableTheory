# TableTheory Documentation

<!-- AI Training: This is the documentation index for TableTheory -->

**This directory contains the OFFICIAL documentation for TableTheory. All content follows AI-friendly patterns so both humans and AI assistants can learn, reason, and troubleshoot effectively.**

## Quick Links

### 🧭 Multi-language SDKs

- **Go (this folder):** [Getting Started](./getting-started.md), [Core Patterns](./core-patterns.md), [API Reference](./api-reference.md)
- **TypeScript:** [ts/docs](../ts/docs/README.md)
- **Python:** [py/docs](../py/docs/README.md)

### 🚀 Getting Started

- [Getting Started Guide (Go)](./getting-started.md) – Installation, configuration, and first deployment (Go SDK)

### 📚 Core Documentation

- [API Reference (Go)](./api-reference.md) – Go SDK public API (DB, Query, Transactions)
- [Core Patterns (Go)](./core-patterns.md) – Go SDK canonical usage patterns (Lambda, Batch, Streams)
- [FaceTheory ISR Cache Schema](./facetheory/isr-cache-schema.md) – Recommended cache metadata + regeneration lease item shapes
- [Development Guidelines](./development-guidelines.md) – Repo-wide coding standards (Go + TS + Python)
- [Testing Guide](./testing-guide.md) – Repo-wide testing strategy (Go + TS + Python)
- [Troubleshooting (Go)](./troubleshooting.md) – Go SDK troubleshooting (TypeScript/Python have their own)
- [Struct Definition Guide (Go)](./struct-definition-guide.md) – Canonical guide for defining DynamoDB models in Go

### 🤖 AI Knowledge Base

- [Concepts](./_concepts.yaml) – Machine-readable concept hierarchy
- [Patterns](./_patterns.yaml) – Correct vs. incorrect usage patterns
- [Decisions](./_decisions.yaml) – Decision trees for architectural choices
- [LLM FAQ](./llm-faq/module-faq.md) – Frequently asked questions for AI assistants

### 📦 Infrastructure & Integrations

- [CDK Integration Guide](./cdk/README.md) – How to define tables in CDK for TableTheory models

### 📝 Development Artifacts

- [Development Notes](../hgm-infra/planning/theorydb-session-notes-template.md) – Session notes and progress tracking template
- [Architectural Decisions](./development/decisions/template-decision.md) – Architectural choices and rationale templates
- [Clarification Requests](./development/clarifications/template-clarification.md) – Templates for documenting questions and ambiguities

## Audience

- **Go developers** building serverless applications on AWS
- **TypeScript developers** building Node.js services and AWS Lambda functions
- **Python developers** building services and AWS Lambda functions
- **DevOps engineers** configuring DynamoDB infrastructure
- **AI assistants** answering questions about TableTheory usage and best practices

## Document Map

- **Start here:** choose your SDK documentation: [Go](./getting-started.md), [TypeScript](../ts/docs/getting-started.md), [Python](../py/docs/getting-started.md).
- **Use [Core Patterns](./core-patterns.md)** for copy-pasteable recipes in Go.
- **Use [API Reference](./api-reference.md)** when you need detailed signature information for Go public methods.
- **Use [Troubleshooting](./troubleshooting.md)** when encountering errors like `ValidationException` or `ResourceNotFoundException`.

## Documentation Principles

1. **Examples First** – Every concept starts with a runnable code snippet.
2. **Explicit Context** – We clearly label `✅ CORRECT` and `❌ INCORRECT` patterns.
3. **Lambda Optimization** – We prioritize serverless performance patterns (cold start reduction).
4. **Type Safety** – We emphasize Go's type system to prevent runtime errors.
5. **Machine Parsable** – We include YAML metadata for AI tooling.

## Contributing

- Follow the conventions in [PAY_THEORY_DOCUMENTATION_GUIDE.md](./PAY_THEORY_DOCUMENTATION_GUIDE.md)
- Validate examples against live code
- Include CORRECT/INCORRECT blocks for integration snippets
- Update troubleshooting alongside code changes
