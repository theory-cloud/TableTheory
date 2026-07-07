---
title: Getting Started with TableTheory
---

# Getting Started with TableTheory

This guide walks you through installing, configuring, and deploying the **Go** implementation of TableTheory.

For the multi-language monorepo:

- TypeScript: `ts/docs/getting-started.md`
- Python: `py/docs/getting-started.md`

## Prerequisites

**Required for the local quickstart:**

- Go toolchain `go1.26.4` (the pinned toolchain in the root `go.mod`)
- Docker, for DynamoDB Local

**Required for real AWS deployments:**

- AWS credentials configured (or an IAM role in Lambda)
- Basic understanding of DynamoDB primary keys and tables

**Recommended:**

- AWS CLI installed for manual verification

## Two-command local quickstart

From a fresh checkout, a Go newcomer can reach a verified CRUD write without an AWS account:

```bash
make docker-up
make example-local
```

`make example-local` runs [`examples/local-quickstart/main.go`](https://github.com/theory-cloud/TableTheory/blob/main/examples/local-quickstart/main.go) against DynamoDB
Local with dummy local credentials. Its output includes the persisted optimistic-lock version after the update:

```text
created note NOTE#local (version 0)
read note: title="Hello TableTheory" value=42 version=0
updated note: title="Hello TableTheory (updated)" persistedVersion=1
deleted note NOTE#local
OK: TableTheory Go quickstart CRUD against DynamoDB Local succeeded
```

The update step re-reads the item before printing `persistedVersion` because TableTheory's `Update` call does not mutate
the caller's struct in memory.

If Docker is unavailable in your environment, use the deterministic fake-backed testing APIs from
[`pkg/testing/fakedb`](https://github.com/theory-cloud/TableTheory/tree/main/pkg/testing/fakedb) for unit tests, but keep this DynamoDB Local proof for onboarding and
contract-shaped smoke validation. The local fake is a test aid, not a replacement for DynamoDB Local when validating
real request shapes.

The checked-in program is intentionally complete:

```go
package main

import (
    "fmt"
    "log"
    "os"
    "strings"
    "time"

    "github.com/theory-cloud/tabletheory/v2"
    "github.com/theory-cloud/tabletheory/v2/pkg/session"
)

type Note struct {
    PK        string    `theorydb:"pk" json:"PK"`
    SK        string    `theorydb:"sk" json:"SK"`
    Title     string    `json:"title,omitempty"`
    Value     int64     `json:"value,omitempty"`
    CreatedAt time.Time `theorydb:"created_at" json:"createdAt"`
    UpdatedAt time.Time `theorydb:"updated_at" json:"updatedAt"`
    Version   int64     `theorydb:"version" json:"version"`
}

func (Note) TableName() string { return "tabletheory_go_quickstart" }

func envOr(key, fallback string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return fallback
}

func main() {
    db, err := tabletheory.New(session.Config{
        Region:   envOr("AWS_REGION", "us-east-1"),
        Endpoint: envOr("DYNAMODB_ENDPOINT", "http://localhost:8000"),
    })
    if err != nil {
        log.Fatalf("connect to DynamoDB: %v", err)
    }

    if err := db.CreateTable(&Note{}); err != nil && !strings.Contains(err.Error(), "ResourceInUseException") {
        log.Fatalf("create table: %v", err)
    }

    note := &Note{PK: "NOTE#local", SK: fmt.Sprintf("run#%d", time.Now().UnixNano()), Title: "Hello TableTheory", Value: 42}
    if err := db.Model(note).IfNotExists().Create(); err != nil {
        log.Fatalf("create: %v", err)
    }
    fmt.Printf("created note %s (version %d)\n", note.PK, note.Version)

    var got Note
    if err := db.Model(&Note{}).Where("PK", "=", note.PK).Where("SK", "=", note.SK).First(&got); err != nil {
        log.Fatalf("read: %v", err)
    }
    fmt.Printf("read note: title=%q value=%d version=%d\n", got.Title, got.Value, got.Version)

    got.Title = "Hello TableTheory (updated)"
    if err := db.Model(&got).Update("title"); err != nil {
        log.Fatalf("update: %v", err)
    }
    var updated Note
    if err := db.Model(&Note{}).Where("PK", "=", note.PK).Where("SK", "=", note.SK).First(&updated); err != nil {
        log.Fatalf("read after update: %v", err)
    }
    fmt.Printf("updated note: title=%q persistedVersion=%d\n", updated.Title, updated.Version)

    if err := db.Model(&Note{}).Where("PK", "=", note.PK).Where("SK", "=", note.SK).Delete(); err != nil {
        log.Fatalf("delete: %v", err)
    }
    fmt.Printf("deleted note %s\n", updated.PK)

    fmt.Println("OK: TableTheory Go quickstart CRUD against DynamoDB Local succeeded")
}
```

## Installation

### Step 1: Add Dependency

```bash
# Add TableTheory to your project
go get github.com/theory-cloud/tabletheory/v2@vX.Y.Z
```

**What this does:**

- Downloads the library and its dependencies (including AWS SDK v2)
- Updates your `go.mod` and `go.sum` files

### Step 2: Define Your Model

Create a struct that represents your DynamoDB item.

```go
package models

import "time"

// CORRECT: Use theorydb tags for keys
type User struct {
    ID        string    `theorydb:"pk" json:"id"`           // Partition Key
    Email     string    `theorydb:"sk" json:"email"`        // Sort Key
    Name      string    `json:"name"`
    CreatedAt time.Time `json:"created_at"`
}
```

**What this does:**

- Defines the data structure
- Tells TableTheory which fields are Primary Keys (`pk`, `sk`)

## First Deployment

### Option A: Lambda Function (Recommended)

Use this for serverless applications to get sub-15ms cold starts.

```go
package main

import (
    "github.com/aws/aws-lambda-go/lambda"
    "github.com/theory-cloud/tabletheory/v2"
    "log"
)

// Global variable for connection reuse
var db *tabletheory.LambdaDB

func init() {
    var err error
    // Initialize once during cold start
    db, err = tabletheory.NewLambdaOptimized()
    if err != nil {
        log.Fatal(err)
    }
}

func handler() (string, error) {
    // Use the pre-warmed connection
    return "Connected!", nil
}

func main() {
    lambda.Start(handler)
}
```

### Option B: Standard Application / Local Dev

Use this for containers, CLI tools, or local testing.

```go
package main

import (
    "github.com/theory-cloud/tabletheory/v2"
    "github.com/theory-cloud/tabletheory/v2/pkg/session"
    "log"
)

func main() {
    // Standard initialization
    db, err := tabletheory.New(session.Config{
        Region: "us-east-1",
        // Uncomment for local DynamoDB:
        // Endpoint: "http://localhost:8000",
    })
    if err != nil {
        log.Fatal(err)
    }
}
```

## Verification

Test your setup by creating an item.

```go
// Create a user
user := &User{
    ID:        "user_123",
    Email:     "test@example.com",
    Name:      "Test User",
    CreatedAt: time.Now(),
}

err := db.Model(user).Create()
if err != nil {
    log.Printf("Error creating user: %v", err)
} else {
    log.Println("User created successfully!")
}
```

## Next Steps

- Read [Core Patterns](./core-patterns.md) for querying and transactions
- See [API Reference](./api-reference.md) for the full interface
- Review [Struct Definition Guide](./struct-definition-guide.md) for advanced modeling

## Troubleshooting

**Issue: "ResourceNotFoundException"**

- **Cause:** The table "users" (derived from `User` struct) does not exist in AWS.
- **Solution:** Create the table in DynamoDB or use `db.CreateTable(&User{})` for development.

[See full troubleshooting guide](./troubleshooting.md)
