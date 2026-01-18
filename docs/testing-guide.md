# Testing Guide

This guide explains how to write unit and integration tests for applications using TableTheory.

## Quick Links (by SDK)

- Go: this document (mocks in `pkg/mocks`)
- TypeScript: [TypeScript Testing Guide](../ts/docs/testing-guide.md)
- Python: [Python Testing Guide](../py/docs/testing-guide.md)

## Unit Testing with Mocks

To write unit tests without connecting to DynamoDB, use the `core.DB` interface and the provided mocks.

### 1. Define Dependencies via Interface

Don't depend on the concrete `*tabletheory.DB` struct. Use `core.DB`.

```go
import "github.com/theory-cloud/tabletheory/pkg/core"

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
    "github.com/theory-cloud/tabletheory/pkg/mocks"
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
    "github.com/theory-cloud/tabletheory"
    "github.com/theory-cloud/tabletheory/pkg/mocks"
    "github.com/theory-cloud/tabletheory/pkg/session"
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

## Python unit testing

Use `theorydb_py.mocks` for strict fakes and deterministic encryption nonces:

```python
from theorydb_py import Table
from theorydb_py.mocks import FakeDynamoDBClient, FakeKmsClient

fake_ddb = FakeDynamoDBClient()
fake_kms = FakeKmsClient(plaintext_key=b"\x00" * 32, ciphertext_blob=b"edk")

table = Table(model, client=fake_ddb, kms_key_arn="arn:aws:kms:...", kms_client=fake_kms, rand_bytes=lambda n: b"\x01" * n)
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
uv --directory py run pytest -q
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
