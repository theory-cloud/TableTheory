# TableTheory CLI

The `tabletheory` command is the consumer-facing DMS tooling surface. It is additive to the older
`tabletheory-contract` helper; existing `tabletheory-contract generate-ts` usage still works, and the same key-contract
helper is also available as `tabletheory contract generate-ts`.

## Validate a DMS document

```bash
go run ./cmd/tabletheory validate ./models.dms.yml
```

A valid document prints a concise pass result:

```text
OK: ./models.dms.yml is valid DMS v0.1 (1 model(s))
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
