# TableTheory Testing Infrastructure

This directory contains the testing infrastructure for TableTheory, including unit tests, integration tests, stress tests, and utilities.

## Overview

The testing infrastructure has been enhanced to support comprehensive testing scenarios including:
- Unit tests for individual components
- Integration tests with DynamoDB Local
- Stress tests for performance and reliability
- Load tests for real-world scenarios

## Prerequisites

### 1. Docker
Integration and stress tests require Docker to run DynamoDB Local:
```bash
# Install Docker from https://www.docker.com/
docker --version
```

### 2. DynamoDB Local
Use the provided setup script to start DynamoDB Local:
```bash
./tests/setup_test_env.sh
```

## Running Tests

### Quick Start
```bash
# Start DynamoDB Local
./tests/setup_test_env.sh

# Run all tests
go test ./... -v

# Run specific test suites
go test ./tests/stress -v
go test ./tests/integration -v
go test ./examples/payment/tests -v
```

### Unit Tests Only
```bash
# Run only unit tests (no DynamoDB Local required)
go test ./... -short
```

### Environment Variables
- `DYNAMODB_ENDPOINT`: DynamoDB endpoint (default: `http://localhost:8000`)
- `AWS_REGION`: AWS region (default: `us-east-1`)
- `SKIP_INTEGRATION`: Skip integration tests (default: `false`)
- `SKIP_MEMORY_TEST`: Skip long-running memory stability tests (default: `false`)

## Test Structure

### `/tests/test_config.go`
Central test configuration utilities:
- `RequireDynamoDBLocal()`: Ensures DynamoDB Local is running
- `GetTestConfig()`: Returns test environment configuration
- `CleanupTestTables()`: Removes all test tables

### `/tests/stress/`
Stress and performance tests:
- **Concurrent Queries**: Tests system behavior under heavy concurrent load
- **Large Item Handling**: Tests handling of items near DynamoDB limits
- **Memory Stability**: Tests for memory leaks under sustained load

### `/tests/integration/`
Integration tests with real DynamoDB operations:
- **Complete Workflow**: Full CRUD operations and transactions
- **Batch Operations**: Batch operations with transactions
- **Table Management**: Table creation, updates, and deletion

### `/examples/*/tests/`
Example-specific tests:
- **Blog Example**: Integration tests for blog functionality
- **Payment Example**: Load tests simulating payment platform scenarios

## Test Utilities

### Setup Script
`setup_test_env.sh` provides:
- Automatic DynamoDB Local setup
- Health check verification
- Environment variable configuration
- Cleanup instructions

### Test Models
Located in `/tests/models/`, providing:
- Consistent test data structures
- Various field types and indexes
- Edge case scenarios

## Writing Tests

### Using Test Utilities
```go
func TestMyFeature(t *testing.T) {
    // Ensure DynamoDB Local is available
    tests.RequireDynamoDBLocal(t)
    
    // Get test configuration
    config := tests.GetTestConfig()
    
    // Initialize TableTheory
    db, err := theorydb.New(theorydb.Config{
        Region:   config.Region,
        Endpoint: config.Endpoint,
    })
    require.NoError(t, err)
    
    // Your test logic here...
}
```

### Best Practices
1. **Use RequireDynamoDBLocal()**: Always check for DynamoDB Local availability
2. **Clean Up Resources**: Delete test tables after tests complete
3. **Unique Table Names**: Use unique names to avoid conflicts in parallel tests
4. **Timeouts**: Set appropriate timeouts for long-running operations
5. **Error Handling**: Check all errors and use meaningful assertions

## Performance Testing

### Stress Tests
```bash
# Run all stress tests
go test ./tests/stress -v

# Run specific stress test
go test ./tests/stress -v -run TestConcurrentQueries

# Skip memory stability test (long-running)
SKIP_MEMORY_TEST=true go test ./tests/stress -v
```

### Load Tests
```bash
# Run payment load tests
go test ./examples/payment/tests -v -run TestRealisticLoad

# Run burst traffic simulation
go test ./examples/payment/tests -v -run TestBurstTraffic
```

## CI/CD Integration

### GitHub Actions Example
```yaml
name: Tests
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    services:
      dynamodb:
        image: amazon/dynamodb-local:3.1.0
        ports:
          - 8000:8000
    
    steps:
    - uses: actions/checkout@v4
    - uses: actions/setup-go@v5
      with:
        go-version-file: go.mod
    
    - name: Run Tests
      env:
        DYNAMODB_ENDPOINT: http://localhost:8000
        AWS_REGION: us-east-1
        AWS_DEFAULT_REGION: us-east-1
        AWS_ACCESS_KEY_ID: dummy
        AWS_SECRET_ACCESS_KEY: dummy
      run: |
        go test ./... -v -race -coverprofile=coverage.out
        go tool cover -html=coverage.out -o coverage.html
```

## Troubleshooting

### DynamoDB Local Not Starting
```bash
# Check if port 8000 is already in use
lsof -i :8000

# Check Docker logs
docker logs dynamodb-local

# Restart DynamoDB Local
docker stop dynamodb-local && docker rm dynamodb-local
./tests/setup_test_env.sh
```

### Tests Timing Out
- Increase test timeout: `go test -timeout 30m`
- Check Lambda timeout settings in tests
- Verify DynamoDB Local is responsive

### Memory Issues
- Monitor memory usage during tests
- Adjust test parameters (concurrent operations, data size)
- Use `-race` flag sparingly for memory-intensive tests

## Contributing

When adding new tests:
1. Follow existing patterns and utilities
2. Document any new test utilities or patterns
3. Ensure tests are deterministic and repeatable
4. Add appropriate skip conditions for CI environments
5. Update this README with new test categories or requirements 
