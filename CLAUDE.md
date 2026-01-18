# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

TableTheory is a Lambda-native, type-safe ORM for Amazon DynamoDB written in Go. It provides lightweight wrappers around DynamoDB operations while maintaining compatibility with Infrastructure as Code patterns.

## Key Development Commands

### Essential Commands
```bash
make build          # Build the project
make test           # Run unit tests
make lint           # Run golangci-lint
make fmt            # Format code with gofmt
make check          # Check for compilation errors
```

### Testing Commands
```bash
make test           # Run unit tests with race detector
make integration    # Run integration tests (starts DynamoDB Local automatically)
make benchmark      # Run performance benchmarks
make test-all       # Run all tests (unit, integration, benchmarks, stress)
make coverage       # Show test coverage in browser

# Team-specific testing
make team1-test     # Test core, model, types, session, errors packages
make team2-test     # Test query, expr, index packages
make examples-test  # Test example code

# Run a single test
go test -v -run TestFunctionName ./path/to/package
```

### Docker Commands
```bash
make docker-up      # Start DynamoDB Local on port 8000
make docker-down    # Stop DynamoDB Local
```

### Lambda Development
```bash
make lambda-build   # Build Lambda function example
make lambda-test    # Test Lambda functionality
make lambda-bench   # Run Lambda benchmarks
```

## High-Level Architecture

### Package Structure
- **`/pkg/core/`** - Core interfaces (DB, Query, UpdateBuilder) that define the contract
- **`/pkg/model/`** - Model registry and metadata management
- **`/pkg/query/`** - Query building and execution logic
- **`/pkg/session/`** - AWS session management
- **`/pkg/types/`** - Type conversion between Go and DynamoDB
- **`/pkg/marshal/`** - Optimized marshaling/unmarshaling
- **`/pkg/transaction/`** - Transaction support
- **`/pkg/mocks/`** - Pre-built mocks for testing (v1.0.1+)
- **`/internal/expr/`** - Expression building for DynamoDB queries

### Key Design Patterns

1. **Interface-Driven Design**: All major components are defined as interfaces in `/pkg/core/`. This enables:
   - Easy mocking for tests (mocks provided in `/pkg/mocks/`)
   - Clean dependency injection
   - Testable code without DynamoDB

2. **Builder Pattern**: Fluent API for query construction with chainable methods:
   ```go
   db.Model(&User{}).Where("ID", "=", "123").OrderBy("CreatedAt", "DESC").All(&users)
   ```

3. **Repository Pattern**: Each model acts as its own repository through the Model() method

4. **Lazy Evaluation**: Queries are built up and only executed on terminal operations (First, All, Create, Update, Delete)

### Lambda Optimizations

The codebase includes specific optimizations for Lambda in `lambda.go`:
- Connection reuse across invocations
- Memory optimization
- Cold start reduction (11ms vs 127ms for standard SDK)
- Use `theorydb.WithLambdaOptimizations()` when initializing

### Testing Strategy

- **Unit Tests**: Alongside source files (`*_test.go`)
- **Integration Tests**: `/tests/integration/` (require DynamoDB Local)
- **Benchmarks**: `/tests/benchmarks/`
- **Test Models**: `/tests/models/` contains test model definitions
- **Mocking**: Use interfaces from `/pkg/core/` and mocks from `/pkg/mocks/`

### Important Conventions

1. **Struct Tags**: DynamoDB configuration via struct tags:
   - `theorydb:"pk"` - Partition key
   - `theorydb:"sk"` - Sort key
   - `theorydb:"created_at"` - Custom attribute name
   - `theorydb:"index:gsi1,pk"` - GSI partition key
   - `theorydb:"index:gsi1,sk"` - GSI sort key

2. **Embedded Structs**: TableTheory supports embedded structs for code reuse:
   ```go
   type BaseModel struct {
       PK        string    `theorydb:"pk"`
       SK        string    `theorydb:"sk"`
       UpdatedAt time.Time `theorydb:"updated_at"`
   }
   
   type Customer struct {
       BaseModel  // Embedded fields are recognized
       Name string
   }
   ```

3. **Error Handling**: Custom error types in `/pkg/errors/` with retry strategies

4. **Performance**: Focus on reducing allocations and reusing resources

5. **Multi-Account**: Built-in support via `WithMultiAccount()` option

## Development Workflow

1. Before making changes, run `make test` to ensure tests pass
2. Use `make fmt` and `make lint` before committing
3. Write tests for new functionality
4. Run `make integration` for integration testing
5. Use interfaces from `/pkg/core/` for testable code
6. Follow existing patterns in similar files

## Key Files to Understand

- `theorydb.go` - Main DB implementation and entry point
- `/pkg/core/interfaces.go` - Core interfaces that define the API
- `/pkg/query/builder.go` - Query builder implementation
- `/pkg/model/registry.go` - Model metadata and registration