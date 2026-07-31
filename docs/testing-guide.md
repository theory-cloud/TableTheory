---
title: Testing Guide
---

# Testing Guide

This guide explains how to write unit and integration tests for applications using TableTheory.

## Quick Links (by SDK)

- Go: this document (mocks in `pkg/mocks`)
- TypeScript: runtime-specific testing guide is staged alongside this document in the shared TableTheory subtree
- Python: runtime-specific testing guide is staged alongside this document in the shared TableTheory subtree

## Go unit testing with mocks and state-backed fakes

To write unit tests without connecting to DynamoDB, use the `core.DB` interface and the provided mocks.

### 1. Define Dependencies via Interface

Don't depend on the concrete `*tabletheory.DB` struct. Use `core.DB`.

```go
import "github.com/theory-cloud/tabletheory/v3/pkg/core"

type UserService struct {
    db core.DB
}

func NewUserService(db core.DB) *UserService {
    return &UserService{db: db}
}
```

### 2. Use Mocks in Tests

TableTheory provides mocks in the `mocks` package (or generate your own with mockery).

```go
import (
    "testing"
    "github.com/stretchr/testify/mock"
    "github.com/theory-cloud/tabletheory/v3/pkg/mocks"
)

func TestCreateUser(t *testing.T) {
    // Setup Mocks
    mockDB := new(mocks.MockDB)
    mockQuery := new(mocks.MockQuery)

    // Expect Model() to be called, return mock query
    mockDB.On("Model", mock.Anything).Return(mockQuery)

    // Expect Create() to be called
    mockQuery.On("Create").Return(nil)

    // Test Service
    service := NewUserService(mockDB)
    err := service.CreateUser("john")

    // Assertions
    if err != nil {
        t.Errorf("Expected no error, got %v", err)
    }
    mockDB.AssertExpectations(t)
}
```

### 3. Encryption + lifecycle determinism (Go)

If you use `theorydb:"encrypted"` fields or lifecycle tags, inject test doubles via `session.Config`:

```go
import (
    "bytes"
    "testing"
    "time"

    "github.com/aws/aws-sdk-go-v2/service/kms"
    "github.com/theory-cloud/tabletheory/v3"
    "github.com/theory-cloud/tabletheory/v3/pkg/mocks"
    "github.com/theory-cloud/tabletheory/v3/pkg/session"
    "github.com/stretchr/testify/mock"
)

func TestEncryptedWrites(t *testing.T) {
    kmsMock := new(mocks.MockKMSClient)
    kmsMock.On("GenerateDataKey", mock.Anything, mock.Anything, mock.Anything).
        Return(&kms.GenerateDataKeyOutput{
            Plaintext:      bytes.Repeat([]byte{0x00}, 32),
            CiphertextBlob: []byte("edk"),
        }, nil)

    db, _ := tabletheory.New(session.Config{
        Region:         "us-east-1",
        KMSKeyARN:      "arn:aws:kms:us-east-1:111111111111:key/test",
        KMSClient:      kmsMock,
        EncryptionRand: bytes.NewReader(bytes.Repeat([]byte{0x01}, 64)),
        Now:            func() time.Time { return time.Unix(0, 0).UTC() },
    })

    _ = db
}
```

### 4. State-backed fake client (Go)

For consumer tests that should exercise TableTheory write/query behavior without Docker, construct a real `DB` over the
deterministic in-memory fake:

```go
import (
    "testing"

    "github.com/theory-cloud/tabletheory/v3"
    "github.com/theory-cloud/tabletheory/v3/pkg/session"
    "github.com/theory-cloud/tabletheory/v3/pkg/testing/fakedb"
)

func TestServiceWritesAndQueries(t *testing.T) {
    fake := fakedb.New()
    db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, fake)
    if err != nil {
        t.Fatal(err)
    }

    if err := db.CreateTable(&User{}); err != nil {
        t.Fatal(err)
    }

    if err := db.Model(&User{PK: "USER#1", SK: "PROFILE", Email: "a@example.com"}).Create(); err != nil {
        t.Fatal(err)
    }

    // Exercise consumer code using db. Use fake.Items("users") for direct assertions when useful.
}
```

`NewWithClient` accepts any implementation of the public `tabletheory.DynamoDBAPI` seam. `pkg/testing/fakedb` honors
TableTheory keys, conditional writes, optimistic-lock version increments, TTL attributes, batches, transactions, and basic
query/scan filters. It is a deterministic local testing aid, not a DynamoDB replacement; behavior beyond the
scenario-validated fake lane should still be covered with DynamoDB Local integration tests.

## TypeScript unit testing

Use `@theory-cloud/tabletheory-ts/testkit` for a strict AWS SDK v3 `send()` mock and deterministic helpers:

```ts
import { PutItemCommand } from "@aws-sdk/client-dynamodb";
import { TheorydbClient } from "@theory-cloud/tabletheory-ts";
import {
  createMockDynamoDBClient,
  fixedNow,
} from "@theory-cloud/tabletheory-ts/testkit";

const mock = createMockDynamoDBClient();
mock.when(PutItemCommand, async () => ({}));

const db = new TheorydbClient(mock.client, {
  now: fixedNow("2026-01-16T00:00:00.000000000Z"),
});
```

For stateful write-then-query tests, use the TypeScript stateful fake:

```ts
import { TheorydbClient } from "@theory-cloud/tabletheory-ts";
import { createStatefulDynamoDBClient } from "@theory-cloud/tabletheory-ts/testkit";

const { client, fake } = createStatefulDynamoDBClient();
const db = new TheorydbClient(client);

// Register models, write through db, then query through db.
// fake.items("users") returns a deterministic snapshot for direct assertions.
```

## Python unit testing

Use `tabletheory_py.mocks` for strict fakes and deterministic encryption nonces:

```python
from tabletheory_py import Table
from tabletheory_py.mocks import FakeDynamoDBClient, FakeKmsClient

fake_ddb = FakeDynamoDBClient()
fake_kms = FakeKmsClient(plaintext_key=b"\x00" * 32, ciphertext_blob=b"edk")

table = Table(model, client=fake_ddb, kms_key_arn="arn:aws:kms:...", kms_client=fake_kms, rand_bytes=lambda n: b"\x01" * n)
```

For stateful Python tests that should exercise TableTheory query/write behavior without scripting every command:

```python
from tabletheory_py import StatefulDynamoDBClient, Table

fake_ddb = StatefulDynamoDBClient()
table = Table(model, client=fake_ddb)

# table.put(...), table.query(...), table.update(...)
# fake_ddb.items("notes") returns a deterministic snapshot for direct assertions.
```

## Integration Testing

For integration tests, connect to a real DynamoDB instance or DynamoDB Local.

### TypeScript integration tests

```bash
make docker-up
npm --prefix ts run test:integration
```

### Python integration tests

```bash
make docker-up
uv --directory py run pytest -q tests/integration
```

### Go integration tests

```go
func TestIntegration(t *testing.T) {
    // Connect to DynamoDB Local
    db, _ := tabletheory.New(session.Config{
        Endpoint: "http://localhost:8000",
        Region:   "us-east-1",
    })

    // Create Table
    db.CreateTable(&User{})

    // Run Test
    err := db.Model(&User{ID: "1"}).Create()
    if err != nil {
        t.Fatal(err)
    }
}
```
