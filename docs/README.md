# TableTheory Documentation

<!-- AI Training: This is the documentation index for TableTheory -->

**New to TableTheory?** Start with the [Go getting-started guide](./getting-started.md). TypeScript and Python
runtime-specific package docs are staged alongside this surface as sibling package entrypoints in the shared
TheoryCloud TableTheory subtree.

| | |
|---|---|
| **Go Guides** | [Getting Started](./getting-started.md) &#124; [API Reference](./api-reference.md) &#124; [Core Patterns](./core-patterns.md) &#124; [Testing](./testing-guide.md) &#124; [Troubleshooting](./troubleshooting.md) |
| **Schema** | [Struct Definition Guide](./struct-definition-guide.md) &#124; [CLI](./cli.md) &#124; DMS v0.1 remains a repo-local planning document and is not published to TheoryCloud |
| **Release State** | [Release-State Safety Patterns](./release-state-patterns.md) |
| **FaceTheory** | [ISR Cache Schema](./facetheory/isr-cache-schema.md) &#124; [ISR Transaction Recipes](./facetheory/isr-transaction-recipes.md) &#124; [TTL Cache Patterns](./facetheory/ttl-cache-patterns.md) &#124; [ISR Idempotency](./facetheory/isr-idempotency.md) |
| **CDK** | [CDK Integration](./cdk/README.md) |
| **Migration** | [Migration Guide](./migration-guide.md) |

---

**This directory contains the OFFICIAL documentation for TableTheory. All content follows AI-friendly patterns so both humans and AI assistants can learn, reason, and troubleshoot effectively.**

## Quick Links

### Multi-language SDKs

- **Go (this folder):** [Getting Started](./getting-started.md), [Core Patterns](./core-patterns.md), [API Reference](./api-reference.md)
- **TypeScript:** package docs are authored under `ts/docs/` and published as a runtime-specific package surface
  alongside the canonical Go docs.
- **Python:** package docs are authored under `py/docs/` and published as a runtime-specific package surface
  alongside the canonical Go docs.

### 🚀 Getting Started

- [Getting Started Guide (Go)](./getting-started.md) – Installation, configuration, and first deployment (Go SDK)

### 📚 Core Documentation

- [API Reference (Go)](./api-reference.md) – Go SDK public API (DB, Query, Transactions)
- [Core Patterns (Go)](./core-patterns.md) – Go SDK canonical usage patterns (Lambda, Batch, Streams)
- [Release-State Safety Patterns](./release-state-patterns.md) – Immutable event history, protected registry fields,
  transactional transitions, outbox/reconciliation, and deterministic provenance/confidence
- [FaceTheory ISR Cache Schema](./facetheory/isr-cache-schema.md) – Recommended cache metadata + regeneration lease item shapes
- [FaceTheory ISR Transaction Recipes](./facetheory/isr-transaction-recipes.md) – Correctness-first patterns for publishing metadata under lease
- [FaceTheory TTL Cache Patterns](./facetheory/ttl-cache-patterns.md) – Freshness vs expiry, TTL lag, and operational guidance for ISR tables
- [FaceTheory ISR Idempotency Patterns](./facetheory/isr-idempotency.md) – Request-id driven regeneration guidance and replay safety
- [Testing Guide](./testing-guide.md) – Repo-wide testing strategy (Go + TS + Python)
- [Troubleshooting (Go)](./troubleshooting.md) – Go SDK troubleshooting (TypeScript/Python have their own)
- [Struct Definition Guide (Go)](./struct-definition-guide.md) – Canonical guide for defining DynamoDB models in Go

### 🤖 AI Knowledge Base

- [Concepts](./_concepts.yaml) – Machine-readable concept hierarchy
- [Patterns](./_patterns.yaml) – Correct vs. incorrect usage patterns
- [Decisions](./_decisions.yaml) – Decision trees for architectural choices
- [LLM FAQ](./llm-faq/module-faq.md) – Frequently asked questions for AI assistants
- [Generative Coding Artifacts](./ai/index.md) – `llms.txt`, vocabulary JSON, consumer rules, and prompt recipes

### 📦 Infrastructure & Integrations

- [CDK Integration Guide](./cdk/README.md) – How to define tables in CDK for TableTheory models

### 📝 Repo-local Maintainer Artifacts

- Planning, decision, and clarification templates live under `docs/development/**` and `gov-infra/planning/**`.
- These maintainer artifacts are intentionally excluded from the TheoryCloud user-facing TableTheory subtree.

## Audience

- **Go developers** building serverless applications on AWS
- **TypeScript developers** building Node.js services and AWS Lambda functions
- **Python developers** building services and AWS Lambda functions
- **DevOps engineers** configuring DynamoDB infrastructure
- **AI assistants** answering questions about TableTheory usage and best practices

## Document Map

- **Start here:** begin with the [Go getting-started guide](./getting-started.md). TypeScript and Python package
  guides are staged alongside this surface as runtime-specific package entrypoints.
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

- Maintainer authoring standards live in repo-local `docs/PAY_THEORY_DOCUMENTATION_GUIDE.md` and are intentionally
  excluded from the TheoryCloud user-facing surface.
- Validate examples against live code
- Include CORRECT/INCORRECT blocks for integration snippets
- Update troubleshooting alongside code changes
