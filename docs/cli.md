# TableTheory CLI

The `tabletheory` command is the consumer-facing DMS tooling surface. It is additive to the older
`tabletheory-contract` helper; existing `tabletheory-contract generate-ts` usage still works, and the same key-contract
helper is also available as `tabletheory contract generate-ts`.

## Install

Like the language runtimes, the CLI is distributed through **GitHub Releases** — there is no Homebrew tap, npm, or PyPI
package. Every release attaches a static binary per platform alongside the TypeScript tarball and Python wheel/sdist:

| Platform            | Asset                      |
| ------------------- | -------------------------- |
| Linux x86_64        | `tabletheory-linux-amd64`  |
| Linux arm64         | `tabletheory-linux-arm64`  |
| macOS x86_64        | `tabletheory-darwin-amd64` |
| macOS Apple Silicon | `tabletheory-darwin-arm64` |

`tabletheory-SHA256SUMS.txt` lists the SHA-256 of every binary and the TypeScript tarball. Download and install the one for your platform (replace
`vX.Y.Z` and the asset name):

```bash
gh release view --repo theory-cloud/TableTheory --json tagName,url

curl -fsSLo tabletheory \
  https://github.com/theory-cloud/tabletheory/releases/download/vX.Y.Z/tabletheory-linux-amd64
chmod +x tabletheory && sudo mv tabletheory /usr/local/bin/tabletheory
tabletheory help
```

Or build from this repository with `go build -o tabletheory ./cmd/tabletheory` (the examples below use
`go run ./cmd/tabletheory`, which works the same from a source checkout).

## Validate a DMS document

```bash
go run ./cmd/tabletheory validate ./models.dms.yml
```

A valid document prints a concise pass result:

```text
OK: ./models.dms.yml is valid DMS v0.2 (1 model(s))
```

Invalid input fails closed with the file path and, when the YAML parser can report it, line context:

```text
models.dms.yml:5: DMS validation failed: parse DMS YAML/JSON: yaml: line 5: ...
     5 |     keys: [not-a-map]
```

For schema-level validation errors that do not carry parser line metadata, the command still includes the file path and a
hint to re-check the DMS version, models, keys, and attributes.

## Generate runtime models

Generate model declarations from the same DMS source of truth:

```bash
go run ./cmd/tabletheory gen --lang go --package models --out ./models_gen.go ./models.dms.yml
go run ./cmd/tabletheory gen --lang ts --out ./models.gen.ts ./models.dms.yml
go run ./cmd/tabletheory gen --lang py --out ./models_gen.py ./models.dms.yml
```

Use `--model <name>` to emit a single model from a multi-model DMS file. TypeScript generation defaults to importing
`@theory-cloud/tabletheory-ts`; pass `--runtime-import` for repository-local tests or monorepo layouts.

Generated Go structs include `TableName()` and, when the DMS declares non-default write policy or index projection
metadata, the small metadata methods needed for `pkg/dms.FromMetadata` equivalence. Generated TypeScript registers via
`defineModel`, and generated Python creates a `ModelDefinition.from_dataclass(...)` value.

## Generate CDK table constructs

Emit AWS CDK (`aws-cdk-lib`) DynamoDB `Table` constructs from the same DMS, so infrastructure cannot drift from the
runtime model contract:

```bash
go run ./cmd/tabletheory gen --cdk --out ./lib/generated-tables.ts ./models.dms.yml
```

See [Generating CDK Table Constructs from DMS](./cdk/generated-constructs.md) for the full mapping and options.

## Scaffold a new project

Create a runnable quickstart — a DMS file, a `docker-compose.yml` for DynamoDB Local, a CRUD program, and a README:

```bash
go run ./cmd/tabletheory init --lang go --dir my-app
go run ./cmd/tabletheory init --lang ts --dir my-app
go run ./cmd/tabletheory init --lang py --dir my-app
```

The generated project reaches a successful CRUD write against DynamoDB Local with one documented command; see the
scaffold's README.
