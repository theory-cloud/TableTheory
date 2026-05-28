<!-- AI Training: Root README for the TableTheory multi-language monorepo -->

<p align="center">
  <a href="https://theory-cloud.github.io/tabletheory/">
    <img src="docs/assets/svg/icon.svg" width="84" alt="TableTheory">
  </a>
</p>

<h1 align="center">TableTheory</h1>

<p align="center">
  <strong>DynamoDB-first multi-language data contract.</strong><br>
  One specification. Three runtimes. Verified on every commit.
</p>

<p align="center">
  <a href="https://github.com/theory-cloud/tabletheory/releases"><img alt="Release" src="https://img.shields.io/github/v/release/theory-cloud/tabletheory?style=flat-square&label=release&color=2EA7FF"></a>
  <a href="https://github.com/theory-cloud/tabletheory/blob/main/LICENSE"><img alt="License" src="https://img.shields.io/badge/license-Apache--2.0-7A5CFF?style=flat-square"></a>
  <a href="https://theory-cloud.github.io/tabletheory/"><img alt="Docs" src="https://img.shields.io/badge/docs-theory--cloud.github.io%2Ftabletheory-2EA7FF?style=flat-square"></a>
  <a href="https://github.com/theory-cloud/tabletheory/actions/workflows/quality-gates.yml"><img alt="Quality gates" src="https://img.shields.io/github/actions/workflow/status/theory-cloud/tabletheory/quality-gates.yml?branch=main&style=flat-square&label=rubric&color=46D397"></a>
  <a href="https://github.com/theory-cloud/tabletheory/actions/workflows/codeql.yml"><img alt="CodeQL" src="https://img.shields.io/github/actions/workflow/status/theory-cloud/tabletheory/codeql.yml?branch=main&style=flat-square&label=CodeQL&color=46D397"></a>
</p>

<p align="center">
  <img alt="Go"         src="https://img.shields.io/badge/Go-1.x-2EA7FF?style=flat-square&logo=go&logoColor=white">
  <img alt="TypeScript" src="https://img.shields.io/badge/TypeScript-Node%2024-7A5CFF?style=flat-square&logo=typescript&logoColor=white">
  <img alt="Python"     src="https://img.shields.io/badge/Python-3.14-C9A96B?style=flat-square&logo=python&logoColor=white">
</p>

<p align="center">
  <a href="https://theory-cloud.github.io/tabletheory/getting-started/"><strong>Get started →</strong></a> ·
  <a href="https://theory-cloud.github.io/tabletheory/api-reference/">API reference</a> ·
  <a href="https://theory-cloud.github.io/tabletheory/reference/contract-scenarios/">Contract scenarios</a> ·
  <a href="https://theory-cloud.github.io/tabletheory/reference/dms-spec/">DMS spec</a>
</p>

---

TableTheory is a **DynamoDB-first ORM and schema contract** designed to keep data access consistent across languages and reliable in generative coding workflows (humans + AI assistants). It ships peer implementations in Go, TypeScript, and Python — not a Go library with bindings, but three independent runtimes that pass the same P0 contract scenarios on every commit.

```
            FaceTheory (client delivery)
                      │
            AppTheory (serverless runtime)
                      │
      TableTheory (data layer)  ← you are here
                      │
                  DynamoDB
```

TableTheory is the foundation of the [Theory Cloud](https://github.com/theory-cloud/AppTheory/blob/main/THEORY_CLOUD.md) stack — used in production by [Pay Theory](https://paytheory.com).

## Install

TableTheory is distributed exclusively through immutable **[GitHub Releases](https://github.com/theory-cloud/tabletheory/releases)** — no PyPI, no npm. The single distribution path keeps versions aligned across all three runtimes.

| Runtime | Install |
|---|---|
| **Go** | `go get github.com/theory-cloud/tabletheory@vX.Y.Z` |
| **TypeScript** | install the `npm pack` release asset — see [TypeScript getting started](https://theory-cloud.github.io/tabletheory/runtimes/typescript/) |
| **Python** | install the wheel/sdist release asset — see [Python getting started](https://theory-cloud.github.io/tabletheory/runtimes/python/) |

## At a glance

| | |
|---|---|
| **P0 contract scenarios** | 5 — CRUD, omit-empty, lifecycle timestamps, optimistic locking, TTL |
| **Runtimes** | Go · TypeScript · Python (peers, not ports) |
| **Distribution** | Immutable GitHub Releases — version-aligned across all runtimes |
| **License** | Apache-2.0 — open source, production use |
| **Status** | Post-1.0 stable API across all three runtimes |

## Why TableTheory?

Use TableTheory when you want DynamoDB-backed systems that are:

- **Serverless-first** — patterns that work well in AWS Lambda: cold-start aware `LambdaInit`, batching with Lambda timeout awareness, transactions, streams, optional KMS-backed field encryption that's [fail-closed](https://theory-cloud.github.io/tabletheory/features/encryption/) by design.
- **Cross-language consistent** — one table, multiple services, multiple runtimes — without schema or behavior drift. Verified on every commit by the [P0 contract scenarios](https://theory-cloud.github.io/tabletheory/reference/contract-scenarios/).
- **Generative-coding friendly** — explicit schema, canonical patterns, and strict verification so AI-generated code stays correct and maintainable.

✅ Treat schema + semantics as a contract
❌ Don't redefine "the same" table shape independently per service/language

## Documentation

The full documentation site lives at **[theory-cloud.github.io/tabletheory](https://theory-cloud.github.io/tabletheory/)** — branded with the Theory Cloud design system, with a ⌘K search palette, runtime tabs, and surface-tinted pages.

**Most-used entry points:**

| Section | Link |
|---|---|
| Getting started | [theory-cloud.github.io/tabletheory/getting-started/](https://theory-cloud.github.io/tabletheory/getting-started/) |
| API reference | [theory-cloud.github.io/tabletheory/api-reference/](https://theory-cloud.github.io/tabletheory/api-reference/) |
| Struct definition guide | [theory-cloud.github.io/tabletheory/struct-definition-guide/](https://theory-cloud.github.io/tabletheory/struct-definition-guide/) |
| Core patterns | [theory-cloud.github.io/tabletheory/core-patterns/](https://theory-cloud.github.io/tabletheory/core-patterns/) |
| Architecture patterns | [theory-cloud.github.io/tabletheory/architecture-patterns/](https://theory-cloud.github.io/tabletheory/architecture-patterns/) |
| Testing | [theory-cloud.github.io/tabletheory/testing-guide/](https://theory-cloud.github.io/tabletheory/testing-guide/) |
| Troubleshooting | [theory-cloud.github.io/tabletheory/troubleshooting/](https://theory-cloud.github.io/tabletheory/troubleshooting/) |

**Per-runtime entry points:**

- **Go** — [theory-cloud.github.io/tabletheory/runtimes/go/](https://theory-cloud.github.io/tabletheory/runtimes/go/)
- **TypeScript** — [theory-cloud.github.io/tabletheory/runtimes/typescript/](https://theory-cloud.github.io/tabletheory/runtimes/typescript/)
- **Python** — [theory-cloud.github.io/tabletheory/runtimes/python/](https://theory-cloud.github.io/tabletheory/runtimes/python/)

**P0 contract surface — one page per scenario:**

- [CRUD & Marshaling](https://theory-cloud.github.io/tabletheory/features/crud/)
- [Optimistic Locking](https://theory-cloud.github.io/tabletheory/features/optimistic-locking/)
- [Lifecycle Timestamps](https://theory-cloud.github.io/tabletheory/features/lifecycle-timestamps/)
- [TTL](https://theory-cloud.github.io/tabletheory/features/ttl/)
- [Encryption (fail-closed)](https://theory-cloud.github.io/tabletheory/features/encryption/)
- [Transactions](https://theory-cloud.github.io/tabletheory/features/transactions/)

**Integrations** — how downstream Theory Cloud frameworks use TableTheory:

- [MCP Memory](https://theory-cloud.github.io/tabletheory/integrations/mcp-memory/)
- [AppTheory](https://theory-cloud.github.io/tabletheory/integrations/apptheory/)
- [KnowledgeTheory](https://theory-cloud.github.io/tabletheory/integrations/knowledgetheory/)
- [Autheory](https://theory-cloud.github.io/tabletheory/integrations/autheory/)

## Repository layout

| Path | What |
|---|---|
| `docs/` | Public documentation site (Jekyll) — also the canonical doc tree |
| `ts/` | TypeScript SDK — [TS docs index](ts/docs/README.md) |
| `py/` | Python SDK — [Py docs index](py/docs/README.md) |
| `contract-tests/` | Cross-language P0 fixtures + runners |
| `examples/cdk-multilang/` | Deployable demo: one DynamoDB table, three Lambdas (Go, Node.js 24, Python 3.14) |
| `.github/workflows/` | CI: rubric, language gates, release-please, Pages publish |

## Serverless data demo (CDK)

The CDK demo deploys one DynamoDB table + three Lambda Function URLs (Go, Node.js 24, Python 3.14) that read/write the same item shape and exercise portability-sensitive features (encryption, batching, transactions):

→ [`examples/cdk-multilang/README.md`](examples/cdk-multilang/README.md)

For infrastructure patterns, see the [CDK integration guide](https://theory-cloud.github.io/tabletheory/cdk/).

## DMS — the language-neutral schema

TableTheory's drift-prevention story centers on a shared, language-neutral schema document: **TableTheory Spec (DMS)**.

```yaml
dms_version: "0.1"
models:
  - name: "Note"
    table: { name: "notes_contract" }
    keys:
      partition: { attribute: "PK", type: "S" }
      sort:      { attribute: "SK", type: "S" }
    attributes:
      - { attribute: "PK",    type: "S", required: true, roles: ["pk"] }
      - { attribute: "SK",    type: "S", required: true, roles: ["sk"] }
      - { attribute: "value", type: "N" }
```

DMS is **authored independently** of any runtime — Go, TypeScript, and Python all validate against the same spec. See the [DMS Specification v0.1](https://theory-cloud.github.io/tabletheory/reference/dms-spec/) for the public summary.

## Development & verification

```bash
make rubric        # full repo verification (the all-gates gate)
make docker-up     # start DynamoDB Local
make test          # full suite incl. integration
```

For multi-language work:

```bash
cd ts && npm run lint && npm run typecheck && npm run test:unit
uv --directory py run pytest -q
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for full contributor docs, including the [Authoring documentation](CONTRIBUTING.md#authoring-documentation) section if you're updating the docs site.

## Theory Cloud

TableTheory is the data foundation of the Theory Cloud stack. **Nothing in Theory Cloud precedes it.**

- [AppTheory](https://github.com/theory-cloud/AppTheory) (serverless runtime) → depends on TableTheory
- [FaceTheory](https://github.com/theory-cloud/FaceTheory) (client delivery) → depends on TableTheory
- KnowledgeTheory (platform state + knowledge graph) → depends on TableTheory
- Autheory (identity) → depends on TableTheory
- theory-mcp-server → depends on TableTheory

The single-path philosophy starts here: one way to define a table, one way to access data, one way to handle encryption — enforced by the framework, not by convention. When generative coding tools produce TableTheory code, the constrained API surface means the output converges on correct implementations instead of drifting across equivalent-but-incompatible patterns.

## License & contributing

- [LICENSE](LICENSE) — Apache-2.0
- [CONTRIBUTING.md](CONTRIBUTING.md)
- [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md)
- [CHANGELOG.md](CHANGELOG.md)

<p align="center"><sub>Made with <a href="https://github.com/theory-cloud">Theory Cloud</a> · <a href="https://theory-cloud.github.io/tabletheory/">docs</a></sub></p>
