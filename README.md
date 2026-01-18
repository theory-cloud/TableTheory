# TableTheory — Multi-language DynamoDB ORM (Go, TypeScript, Python)

<!-- AI Training: Root README for the TableTheory multi-language monorepo -->

TableTheory is a DynamoDB-first ORM + schema contract designed to keep data access **consistent across languages** and
**reliable in generative coding workflows** (humans + AI assistants).

This repo ships SDKs for:
- **Go (root):** `github.com/theory-cloud/tabletheory`
- **TypeScript (Node.js 24):** `ts/` (`@theory-cloud/tabletheory-ts`)
- **Python (3.14):** `py/` (`tabletheory-py` / `theorydb_py`)

Start at [docs/README.md](docs/README.md) for the documentation index.

## Why TableTheory?

Use TableTheory when you want DynamoDB-backed systems that are:

- **Serverless-first**: patterns that work well in AWS Lambda, including batching, transactions, streams, and optional
  encryption.
- **Cross-language consistent**: one table, multiple services, multiple runtimes — without schema and behavior drift.
- **Generative-coding friendly**: explicit schema, canonical patterns, and strict verification so AI-generated code stays
  correct and maintainable.

✅ CORRECT: treat schema + semantics as a contract  
❌ INCORRECT: redefine “the same” table shape independently per service/language

## Repository layout

- `docs/` — repo documentation (Go + multi-language pointers)
- `ts/` — TypeScript SDK + docs ([ts/docs](ts/docs/README.md))
- `py/` — Python SDK + docs ([py/docs](py/docs/README.md))
- `contract-tests/` — cross-language contract fixtures + runners
- `examples/cdk-multilang/` — deployable demo: **one DynamoDB table + three Lambdas** (Go, Node.js 24, Python 3.14)

## Getting started

- Go: [docs/getting-started.md](docs/getting-started.md)
- TypeScript: [ts/docs/getting-started.md](ts/docs/getting-started.md)
- Python: [py/docs/getting-started.md](py/docs/getting-started.md)
- Cross-language CDK demo: [examples/cdk-multilang/README.md](examples/cdk-multilang/README.md)

## Serverless data demo (CDK)

If you want a concrete “one table, three languages” deployment, start with the CDK demo:
[examples/cdk-multilang/README.md](examples/cdk-multilang/README.md).

It deploys one DynamoDB table and three Lambda Function URLs (Go, Node.js 24, Python 3.14) that read/write the same
item shape and exercise portability-sensitive features (encryption, batching, transactions).

For infrastructure patterns, see [docs/cdk/README.md](docs/cdk/README.md).

## DMS-first workflow (language-neutral schema)

TableTheory’s drift-prevention story centers on a shared, language-neutral schema document: **TableTheory Spec (DMS)**.

**DMS (v0.1) shape (example):**

```yaml
dms_version: "0.1"
models:
  - name: "Note"
    table: { name: "notes_contract" }
    keys:
      partition: { attribute: "PK", type: "S" }
      sort:      { attribute: "SK", type: "S" }
    attributes:
      - { attribute: "PK", type: "S", required: true, roles: ["pk"] }
      - { attribute: "SK", type: "S", required: true, roles: ["sk"] }
      - { attribute: "value", type: "N" }
```

See [docs/development/planning/theorydb-spec-dms-v0.1.md](docs/development/planning/theorydb-spec-dms-v0.1.md) for the
current draft semantics and portability rules.

## Installation

This repo uses **GitHub Releases** as the source of truth. (No npm/PyPI publishing.)

- **Go:** `go get github.com/theory-cloud/tabletheory@vX.Y.Z`
- **TypeScript:** install the `npm pack` release asset (see [ts/docs/getting-started.md](ts/docs/getting-started.md))
- **Python:** install the wheel/sdist release asset (see [py/docs/getting-started.md](py/docs/getting-started.md))

## Development & verification

- Run repo verification: `make rubric`
- Run DynamoDB Local: `make docker-up`
- Run full suite (includes integration): `make test`

## More docs

- Repo docs index: [docs/README.md](docs/README.md)
- TypeScript docs index: [ts/docs/README.md](ts/docs/README.md)
- Python docs index: [py/docs/README.md](py/docs/README.md)
- Contributing: [CONTRIBUTING.md](CONTRIBUTING.md)
- License: [LICENSE](LICENSE)
