---
title: Go
description: TableTheory for Go — installation, Lambda init, and the canonical model shape.
---

# Go runtime

TableTheory's Go runtime is the root module at `github.com/theory-cloud/tabletheory`. It targets the AWS SDK for Go v2, is pinned to the toolchain declared in `go.mod`, and is the runtime the rest of the Theory Cloud stack consumes by default.

## Install

```bash
go get github.com/theory-cloud/tabletheory
```

Pin to a specific release tag for reproducibility:

```bash
go get github.com/theory-cloud/tabletheory@v1.x.y
```

Releases are published as immutable [GitHub Releases](https://github.com/theory-cloud/tabletheory/releases). Never depend on a moving `latest`.

## Lambda init

`tabletheory.LambdaInit` is the blessed entry point. Construct once at cold start, reuse across invocations:

```go
package main

import (
    "context"

    "github.com/aws/aws-lambda-go/lambda"
    "github.com/theory-cloud/tabletheory"
)

type Note struct {
    _  struct{} `theorydb:"naming:snake_case"`
    PK string   `theorydb:"pk"       json:"pk"`
    SK string   `theorydb:"sk"       json:"sk"`

    Body      string `theorydb:"omitempty"   json:"body,omitempty"`
    Version   int64  `theorydb:"version"     json:"version"`
    CreatedAt int64  `theorydb:"created_at"  json:"created_at"`
    UpdatedAt int64  `theorydb:"updated_at"  json:"updated_at"`
}

var db = tabletheory.LambdaInit(context.Background(),
    tabletheory.WithTable("notes"),
    tabletheory.WithModels(&Note{}),
)

func handler(ctx context.Context, evt Note) error {
    return db.Put(ctx, &evt)
}

func main() { lambda.Start(handler) }
```

> Constructing the client inside the handler — rather than at module scope — defeats Lambda's connection reuse and burns ~50 ms per cold invocation. `LambdaInit` exists specifically to make the right pattern the easiest one.

## Model shape

A TableTheory Go model is an ordinary struct decorated with the `theorydb:` tag vocabulary:

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
| `theorydb:"naming:…"`      | Apply naming strategy via the leading `_ struct{}` field     |

Every `theorydb` tag must be accompanied by a matching `json` tag (see [Development guidelines](../development-guidelines/)).

## Where to go next

- [Getting Started](../getting-started/) — full step-by-step walkthrough
- [Struct Definition Guide](../struct-definition-guide/) — every tag, every shape
- [API Reference](../api-reference/) — exported types and methods
- [Core Patterns](../core-patterns/) — single-table query, GSI, and transaction recipes
- [Features → CRUD & Marshaling](../features/crud/) — the canonical P0 contract behavior

## Stability and support

The Go runtime is **GA** (post-1.0) and the reference for cross-runtime contract parity. Breaking changes follow [semver](https://semver.org/) and are coordinated with downstream Theory Cloud products. See [Contract Scenarios](../reference/contract-scenarios/) for the P0 specification all three runtimes are verified against.
