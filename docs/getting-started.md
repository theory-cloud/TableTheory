# Getting Started with TableTheory

This guide walks you through installing, configuring, and deploying the **Go** implementation of TableTheory.

For the multi-language monorepo:

- TypeScript: `ts/docs/getting-started.md`
- Python: `py/docs/getting-started.md`

## Prerequisites

**Required:**

- Go 1.25 or higher
- AWS Credentials configured (or IAM role in Lambda)
- Basic understanding of DynamoDB (Primary Keys, Tables)

**Recommended:**

- AWS CLI installed for verification
- Docker (if using DynamoDB Local)

## Installation

### Step 1: Add Dependency

```bash
# Add TableTheory to your project
go get github.com/theory-cloud/tabletheory
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
    "github.com/theory-cloud/tabletheory"
    "log"
)

// Global variable for connection reuse
var db *theorydb.LambdaDB

func init() {
    var err error
    // Initialize once during cold start
    db, err = theorydb.NewLambdaOptimized()
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
    "github.com/theory-cloud/tabletheory"
    "github.com/theory-cloud/tabletheory/pkg/session"
    "log"
)

func main() {
    // Standard initialization
    db, err := theorydb.New(session.Config{
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
