# Development Guidelines

This guide outlines the coding standards and best practices for developing TableTheory in this multi-language monorepo:

- Go (root module)
- TypeScript (`ts/`)
- Python (`py/`)

## Struct Definition Standards

TableTheory relies heavily on Go struct tags. Follow these rules strictly:

1.  **Primary Keys:** Always tag your partition key with `theorydb:"pk"` and sort key with `theorydb:"sk"`.
2.  **JSON Tags:** Always include `json:"name"` tags matching your attribute names (usually snake_case).
3.  **Types:** Use standard Go types (`string`, `int`, `int64`, `float64`, `bool`, `time.Time`).

```go
// ✅ CORRECT
type Product struct {
    ID    string  `theorydb:"pk" json:"id"`
    Price float64 `json:"price"`
}

// ❌ INCORRECT
type Product struct {
    ID string // Missing tags!
}
```

## TypeScript SDK standards (`ts/`)

- Runtime/toolchain: Node.js **24**
- Must pass:
  - `npm --prefix ts run typecheck`
  - `npm --prefix ts run lint`
  - `npm --prefix ts run test`
- Prefer explicit attribute names in model definitions (`defineModel`) to stay DMS-friendly and avoid drift.
- Do not weaken testkit strictness (`@theory-cloud/tabletheory-ts/testkit`).

See [TypeScript Development Guidelines](../ts/docs/development-guidelines.md).

## Python SDK standards (`py/`)

- Runtime/toolchain: Python **3.14**
- Must pass:
  - `uv --directory py run mypy src` (strict)
  - `uv --directory py run ruff check`
  - `uv --directory py run pytest -q`
- Prefer dataclasses with explicit roles via `theorydb_field(...)`.
- Do not weaken strict fakes (`theorydb_py.mocks`); unit tests must not call real AWS.

See [Python Development Guidelines](../py/docs/development-guidelines.md).

## Error Handling

Always check errors. TableTheory returns typed errors where possible.

- **Validation Errors:** Occur before network calls (invalid struct tags, missing keys).
- **Runtime Errors:** Occur during AWS execution (throughput exceeded, conditional check failed).

```go
if err := db.Model(item).Create(); err != nil {
    if errors.Is(err, customerrors.ErrConditionFailed) {
        // Handle duplicate
    }
    return err
}
```

## Code Style

- **Fluent Chains:** Break long query chains onto multiple lines for readability.
- **Context:** Use `context.TODO()` or `context.Background()` if you aren't passing a request context (though `WithContext` is preferred).

```go
// Readable
db.Model(&Item{}).
    Where("ID", "=", "1").
    Limit(1).
    First(&item)
```

## Contribution Workflow

1.  **Fork & Branch:** Create a feature branch.
2.  **Test:** Run `go test ./...` to ensure no regressions.
3.  **Docs:** Update documentation if you change public APIs.
4.  **PR:** Submit a Pull Request with a clear description.
