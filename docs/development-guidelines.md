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
- **Joined parallelism is fine; abandoned parallelism is not.** A fan-out may run concurrently only if every worker has finished before the call returns, on every path: success, first error, and caller cancellation. `pkg/query` joins its segment, batch-get, and batch-update workers on all three paths (`TestScanAllSegments_JoinsWorkersOnSegmentError`, `TestScanAllSegments_JoinsWorkersOnContextCancel`, `TestBatchGetParallelJoinsWorkersOnChunkError`, `TestBatchUpdateParallelJoinsWorkersOnBatchError`), the TypeScript runtime's `mapConcurrent` waits for every worker before it rejects, and Python's `Table.scan_all_segments` already joined on every path through its `ThreadPoolExecutor` context manager and now has an error-path test. `Query.ScanAllSegments` used to return on the first segment error while the remaining segment goroutines were still running; that is the shape of bug this rule exists to prevent.
- **Bounded fan-in counts too.** A timeout path that hands work to a helper goroutine must unblock that helper and wait for it before returning. `pkg/protection.SecureBodyReader` closes the request body and joins its reader on the timeout path (`TestSecureBodyReaderTimeoutJoinsReaderGoroutine`).
- **The detached-work guard enforces the rule mechanically.** Three detectors scan the sources we ship or run — the root package, `pkg/`, `internal/`, `cmd/`, `scripts/`, `contract-tests/runners/`, `examples/`, `ts/src`, `ts/examples`, `py/src`, and `py/examples` — and each reports a launch it cannot prove joined. Each detector's proof is exactly this, and no more:
    - **Go** (`tests/detached_work_scan_test.go`, pinned by `tests/detached_work_joins_test.go`) parses with `go/ast` and proves *dominance*: from the statement after the launch it walks the owner's statement lists (blocks, if/else, loops, switch and select clauses) and accepts the launch only when the join runs on every path that reaches a return. A `Wait` behind a branch, a `return` before the `Wait`, a `Wait` inside a never-called closure, a labeled branch or `goto`, and a channel whose sends outnumber the receives are all reported. The recognized joins are a `sync.WaitGroup` (`Add` before the launch, `Done` inside it, `Wait` after it), including a `defer wg.Wait()` registered before any return, because it runs whenever the function returns; an errgroup `Wait`; and an unbuffered close-signal channel drained after the launch. Timers that schedule work past the caller's return (`time.AfterFunc`, `time.NewTimer`, `time.NewTicker`, `time.Tick`) are reported when nothing stops them.
    - **Python** (`tests/detached_work_python_test.go`) reads indentation rather than a parser, so its proof is a deliberately conservative reading of the same rule. A thread, timer, or process bound to a target — a local name, an attribute (`self.worker`), or a container element (`pool["a"]`) — is accepted only when a join it binds (`target.join()` for a thread, timer, or process; `await target` or `asyncio.gather(...)` for an `asyncio` task; `await target` for an offload) appears after the launch, in the same function, inside no block the launch is not also inside (a join behind an `if`, in a sibling branch, or in a nested function does not count), and with no `return`, `raise`, `break`, or `continue` crossed first. An executor `submit` is joined only by staying inside its `with ...Executor(...)` block. Daemon threads, threads built in a comprehension, un-awaited `asyncio` tasks, and un-awaited `asyncio.to_thread` / `loop.run_in_executor` results are reported; a `task.cancel()` is not a join, because cancellation is cooperative and the task can still be running at return. Because the scan does not model branch merging, a join split across both branches of an `if/else` is reported even though both branches join.
    - **TypeScript** (`tests/detached_work_typescript_test.go`) reads braces rather than a parser. It reports a `void` call (including the computed-member form `void obj["method"]()`), a bare `.then`/`.catch`/`.finally` chain that neither awaits, returns, nor assigns on its line, a promise bound to a name whose value chains `.then`/`.catch`/`.finally` and is not awaited or returned on a path that dominates every return, a statement-position async IIFE, `queueMicrotask` and `process.nextTick` (including the computed-member forms `globalThis["queueMicrotask"]` and `process["nextTick"]`), a timer that is neither a resolver delay nor a managed, cleared handle (including `globalThis["setTimeout"]` and friends), and a bare call to a function the file declares `async`. Typed floating-promise detection inside the library package remains the job of `@typescript-eslint/no-floating-promises`; this detector covers the examples and tooling surfaces too.
    - **Test-file suffixes are not scanned.** Go skips `_test.go`; Python skips `test_*.py`, `*_test.py`, and `conftest.py`; TypeScript and JavaScript skip `.test.ts`, `.spec.ts`, `.d.ts`, `.test.mts`, and `.test.js`. Those files exercise the code rather than ship as a handler, so a launch inside one is not an invocation leak; every other path in the surfaces above is scanned.
    - **Allowlists.** The launch allowlists for the library, TypeScript, and Python are empty and must stay empty; the local dev server launch in `examples/multi-tenant/cmd/local/main.go` is the one reviewed exception, because that `main` blocks on a signal before shutting the server down and is never deployed as a Lambda. The TypeScript and JavaScript scan covers every path in those surfaces with no exclusions; the generated-ts verifier under `scripts/` runs as an ES module (`.mts`) precisely so its entrypoint can be awaited instead of excluded.

## Contribution Workflow

1.  **Fork & Branch:** Create a feature branch.
2.  **Test:** Run `go test ./...` to ensure no regressions.
3.  **Docs:** Update documentation if you change public APIs.
4.  **PR:** Submit a Pull Request with a clear description.
