---
title: Development Guidelines
---

# Development Guidelines

This guide outlines the coding standards and best practices for developing TableTheory in this multi-language monorepo:

- Go (root module)
- TypeScript (`ts/`)
- Python (`py/`)

## Struct Definition Standards

TableTheory relies heavily on Go struct tags. Follow these rules strictly:

1.  **Primary Keys:** Always tag your partition key with `theorydb:"pk"` and sort key with `theorydb:"sk"`. For legacy DynamORM-compatible models, add `theorydb:"naming:dynamorm"` on a marker field so the actual DynamoDB key attributes stay `PK` and `SK`.
2.  **JSON Tags:** Always include `json:"name"` tags matching your attribute names. For new models this is usually snake_case; for legacy DynamORM models use JSON names that match the legacy wire format you need to preserve.
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

```go
// ✅ LEGACY DYNAMORM COMPATIBLE
type LegacyUser struct {
    _ struct{} `theorydb:"naming:dynamorm"`

    UserID    string `theorydb:"pk" json:"PK"`
    Entity    string `theorydb:"sk" json:"SK"`
    FirstName string `json:"firstName"`
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
  - `uv --directory py run pytest -q tests/unit`
- Prefer dataclasses with explicit roles via `theorydb_field(...)`.
- Do not weaken strict fakes (`tabletheory_py.mocks`); unit tests must not call real AWS.

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

## Lambda and Concurrency

No work may outlive the invocation or init that started it. Lambda freezes the execution environment as soon as the handler returns, so a goroutine (Go), timer or promise (TypeScript), or thread or future (Python) started from an init path or a handler is not guaranteed to run: it can be frozen mid-flight and later resume against an invocation that has already completed.

- **Keep init synchronous.** Every TableTheory init path finishes its work before it returns. `LambdaInit`, `OptimizeForColdStart`, `NewLambdaOptimized`, and `NewMultiAccount` start no background work.
- **No init-time network probe.** A cold-start pre-warm only pays off when it completes, and it needs IAM permissions beyond the operations the handler already performs. The removed `OptimizeForColdStart` pre-warm issued `ListTables`; do not reintroduce an equivalent call.
- **Keep the leak checks green.** `internal/theorydb/goroutine_leak_test.go` fails if a Lambda init path leaves a goroutine behind, and the TypeScript and Python runtime suites assert the equivalents. Extend them when you add an init path.
- **Joined parallelism is fine.** Parallel work that is fully joined before the call returns — `pkg/query` batch and segment workers, `Table.scan_all_segments` — stays inside its invocation and does not violate the rule.

## Contribution Workflow

1.  **Fork & Branch:** Create a feature branch.
2.  **Test:** Run `go test ./...` to ensure no regressions.
3.  **Docs:** Update documentation if you change public APIs.
4.  **PR:** Submit a Pull Request with a clear description.
