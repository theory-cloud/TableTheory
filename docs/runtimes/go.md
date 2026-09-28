---
title: Go
description: TableTheory for Go — installation, Lambda init, and the canonical model shape.
---

# Go runtime

TableTheory's Go runtime is the root module at `github.com/theory-cloud/tabletheory/v4`. It targets the AWS SDK for Go v2 and uses the toolchain pinned in `go.mod`.

The Go runtime is a peer implementation of the shared contract — not a
reference implementation that TypeScript and Python port. Contract parity is
established by the shared scenarios, not by treating one runtime as canonical.

## Install

TableTheory is distributed exclusively through immutable [GitHub Releases](https://github.com/theory-cloud/tabletheory/releases). Pin to a specific release tag:

```bash
go get github.com/theory-cloud/tabletheory/v4@vX.Y.Z
```

Never depend on a moving `latest`.

## Lambda init

`tabletheory.NewLambdaOptimized()` is the blessed cold-start entry point. Construct once at module init, reuse across invocations:

```go
package main

import (
    "log"

    "github.com/aws/aws-lambda-go/lambda"
    "github.com/theory-cloud/tabletheory/v4"
)

type Note struct {
    PK   string `theorydb:"pk" json:"pk"`
    SK   string `theorydb:"sk" json:"sk"`
    Body string `json:"body"`
}

var db *tabletheory.LambdaDB

func init() {
    var err error
    db, err = tabletheory.NewLambdaOptimized()
    if err != nil {
        log.Fatal(err)
    }
}

func handler() error {
    return db.Model(&Note{
        PK:   "USER#42",
        SK:   "NOTE#welcome",
        Body: "Hello, Theory Cloud.",
    }).Create()
}

func main() { lambda.Start(handler) }
```

> Construct the client at module init, not inside the handler. Lambda reuses module-scope state across warm invocations; rebuilding the client per request burns ~50 ms each cold start.

`tabletheory.LambdaInit(models ...any) (*LambdaDB, error)` exists as the lower-level variant if you want to register specific model types explicitly at cold start. Most consumers should prefer `NewLambdaOptimized()` and use `db.Model(&Foo{})` per request.

### No background work

**Invariant:** no TableTheory init or handler path leaves work running after it returns. Every step is synchronous and joined to the call that started it.

Lambda freezes the execution environment as soon as the handler returns. A goroutine started during init is not guaranteed to run: it can be frozen mid-flight and later resume against an invocation that has already completed, and any network call it made is billed to an invocation that is already over. TableTheory therefore starts no goroutine that outlives its init or its operation.

Parallel fan-out is allowed only when it is fully joined. `Query.ScanAllSegments` cancels the remaining segments as soon as one fails and then waits for every segment worker before it returns the first error; the parallel `BatchGet` and batch-update paths already waited for their workers on the same paths. Nothing in `pkg/query` returns while one of its workers is still running, on success, on error, or when the caller's context is cancelled. The exception-shaped piece is the request-body guard: `pkg/protection.SecureBodyReader` closes the body and joins its reader on the timeout path instead of returning while the read is still blocked.

`MultiAccountDB` follows the same rule: it refreshes expired partner sessions synchronously on the `Partner()` path rather than from a background ticker. That sweep is best-effort — it is not single-flighted, so concurrent calls may refresh the same partner twice, and a failed refresh leaves the expired entry in place to be retried — but it never leaves a refresh running past the call that started it.

`LambdaDB.OptimizeForColdStart()` follows this rule. It keeps its signature and performs only synchronous, local model-metadata work. It deliberately does not pre-warm the connection with a DynamoDB API call: earlier releases issued `ListTables` from a detached goroutine with a 100 ms timeout, which needed `dynamodb:ListTables` on top of the item permissions a handler actually uses, and which could be frozen before it ever completed. If your policies grant `dynamodb:ListTables` only for that old pre-warm, you can remove the grant. Connection reuse across warm invocations still comes from constructing the client once at module scope.

## Model shape

A TableTheory Go model is an ordinary struct decorated with the `theorydb:` tag vocabulary alongside matching `json:` tags:

| Tag                        | Purpose                                                      |
|----------------------------|--------------------------------------------------------------|
| `theorydb:"pk"`            | Partition key                                                |
| `theorydb:"sk"`            | Sort key                                                     |
| `theorydb:"gsi1pk"` etc.   | Global secondary index keys                                  |
| `theorydb:"encrypted"`     | KMS-encrypted field, fail-closed                             |
| `theorydb:"version"`       | Optimistic-lock version field                                |
| `theorydb:"created_at"`    | Lifecycle timestamp populated on first write                 |
| `theorydb:"updated_at"`    | Lifecycle timestamp populated on every write                 |
| `theorydb:"ttl"`           | DynamoDB TimeToLive attribute                                |
| `theorydb:"omitempty"`     | Omit attribute when the field is the zero value              |

Every `theorydb` tag is accompanied by a matching `json` tag per the [Development guidelines](https://tabletheory.theorycloud.ai/development-guidelines/).

## CRUD via the Query builder

The query builder is reached through `db.Model(&model)`:

```go
// Create — fails with ErrConditionFailed if the partition key already exists
db.Model(&Note{PK: "USER#42", SK: "NOTE#welcome", Body: "Hi."}).Create()

// Read (use First with a destination)
var got Note
db.Model(&Note{PK: "USER#42", SK: "NOTE#welcome"}).First(&got)

// Update selected fields
db.Model(&Note{PK: "USER#42", SK: "NOTE#welcome", Body: "Updated."}).Update("body")

// Delete
db.Model(&Note{PK: "USER#42", SK: "NOTE#welcome"}).Delete()

// Intentional overwrite/upsert — the explicit opt-out from Create's guard
db.Model(&Note{PK: "USER#42", SK: "NOTE#welcome", Body: "Replacement."}).CreateOrUpdate()
```

When `Update()` is called without field names, Go treats the model as a sparse
update: zero-valued fields tagged `omitempty` are not selected and therefore do
not overwrite existing attributes. Naming an empty `omitempty` field explicitly
with `Update("Field")` selects it and removes the DynamoDB attribute under DMS
v0.2. To store a present empty value, remove `omitempty` from the model or use
an explicit low-level `UpdateBuilder().Set("Field", zeroValue)`.

For a full working program covering conditional helpers, batches, transactions, and streams, see [`examples/feature_spotlight.go`](https://github.com/theory-cloud/tabletheory/blob/main/examples/feature_spotlight.go).

## Where to go next

- [Getting Started](../getting-started.md) — full step-by-step walkthrough
- [Struct Definition Guide](../struct-definition-guide.md) — every tag, every shape
- [API Reference](../api-reference.md) — exported types and methods
- [Core Patterns](../core-patterns.md) — single-table query, GSI, and transaction recipes
- [Features → CRUD & Marshaling](../features/crud.md) — the canonical P0 contract behavior

## Stability and support

The Go runtime is **GA** (post-1.0). Breaking changes follow [semver](https://semver.org/) and are coordinated with downstream Theory Cloud products and the TypeScript and Python runtimes — never in isolation.
