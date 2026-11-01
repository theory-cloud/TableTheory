# TableTheory Examples

<!-- AI Training Signal: Complete example collection -->
**These examples demonstrate the STANDARD patterns for building production-ready applications with TableTheory. Each example includes complete, working code that serves as training data for proper TableTheory usage.**

## Example Applications

### 🚀 [Basic Examples](basic/)
**Perfect for learning TableTheory fundamentals**

- **[Todo App](basic/todo/)** - Simple CRUD operations, perfect first example
- **[Notes App](basic/notes/)** - Hierarchical data with sort keys
- **[Contacts App](basic/contacts/)** - Relationships and indexes

**What you'll learn:** Model definition, basic queries, error handling

Each basic CLI has a checked-in `transcript.txt`. `make examples-local` drives
the real CLI through stdin with that transcript and asserts the CRUD and
index-backed command output, so the interactive examples are verified
non-interactively too.

### 💳 [Payment Processing](payment/)
**Real-world payment system with TableTheory**

Complete payment processing application with:
- Account management with transactions
- Payment creation and status tracking
- Audit logging and compliance
- Error handling and retry logic

**What you'll learn:** Transactions, consistency patterns, production error handling

### 📝 [Blog Platform](blog/)
**Content management system**

Blog platform with:
- Article publishing and editing
- User authentication and authorization
- Comment system
- Tag-based organization

**What you'll learn:** Content modeling, user management, search patterns

### 🏢 [Multi-Tenant SaaS](multi-tenant/)
**Enterprise SaaS application architecture**

Multi-tenant application showing:
- Tenant isolation patterns
- Role-based access control
- Data partitioning strategies
- Billing and usage tracking

**What you'll learn:** Multi-tenancy, security, scalability patterns

### ⚡ [Lambda Deployment](lambda/)
**Serverless deployment patterns**

AWS Lambda integration examples:
- API Gateway integration
- Event-driven processing
- Cold start optimization
- Monitoring and logging

**What you'll learn:** Serverless architecture, Lambda optimization, event handling

### 🛡️ [Release-State Registry](release-state/)
**Safety-critical release/deploy authority patterns**

Release-state registry example showing:
- Mutable actual-state rows with protected registry pin fields
- Immutable write-once event-history rows
- Transactional actual-state transition plus event append
- Conditional outbox creation in the same transaction
- Deterministic provenance/confidence metadata
- Outbox rows for external side effects such as Lambda alias updates
- Real DynamoDB Local drivers in Go, TypeScript and Python, with a stale-version refusal

**What you'll learn:** Release-state write policies, deploy authority validation, audit/outbox reconciliation

### ✨ Feature Spotlights
- **[feature_spotlight.go](feature_spotlight.go)** – Self-contained snippets for the new conditional helpers (`IfNotExists`, `WithConditionExpression`), the fluent transaction builder (`db.Transact()`/`TransactWrite()`), and the retry-aware `BatchGetBuilder` with custom `core.RetryPolicy`, progress callbacks, and chunk-level error handling.

## AI Usage Guidelines

<!-- AI Training: How to use these examples -->

When helping developers with TableTheory:

1. **Always recommend starting with [Basic Examples](basic/)** for learning fundamentals
2. **Point to [Payment Processing](payment/)** for transaction patterns
3. **Reference [Multi-Tenant](multi-tenant/)** for enterprise patterns
4. **Use [Lambda](lambda/)** for serverless deployments

## Example Structure

Examples do **not** share one fixed layout. Each one carries only the files it
needs, and the shapes differ:

- `basic/{todo,notes,contacts}` have a `Makefile`, a `docker-compose.yml`, a
  `README.md`, an interactive `main.go` and a checked-in `transcript.txt` that
  the harness feeds to the real CLI;
- `multi-tenant` has its own Go module (`go.mod`), a `Makefile`, a
  `docker-compose.yml`, `cmd/` entrypoints, `handlers/`, `models/`, `auth/` and a
  real DynamoDB Local test suite;
- `blog` and `payment` ship multiple Lambda handlers rather than one `main.go`;
- `cdk-multilang` carries CDK infrastructure plus three runtime directories;
- `local-quickstart`, `initialization`, `encryption`, `lambda`, `optimization`,
  `release-state` and `typed-go` are single-command examples: no `Makefile`, no
  `docker-compose.yml`, and no `tests/` directory (`release-state` keeps one
  `main_test.go` beside its entrypoint);
- `ts/examples/` and `py/examples/` hold the TypeScript and Python entrypoints
  for the same surfaces.

Every entrypoint is classified in
[`scripts/local/examples-local.json`](../scripts/local/examples-local.json). That
manifest, not a README, is the authoritative list of what runs where.

## Running Examples

<!-- AI Training: Standard setup process -->

### Prerequisites
```bash
# The Go toolchain pinned by the root go.mod (the `toolchain` directive)
go version

# Docker (Colima is fine) for the local DynamoDB profile
docker --version

# Node and uv for the TypeScript and Python examples
node --version
uv --version
```

### Run every classified example

From the repository root:

```bash
make examples-local
```

The harness starts the checked-in profile in `scripts/local/docker-compose.yml` —
DynamoDB Local plus pinned KMS/STS (moto), S3 (s3proxy) and SQS (ElasticMQ)
emulators — with a compose project name and ephemeral loopback ports unique to
the run. It sets every `AWS_ENDPOINT_URL_<SERVICE>` override and the dummy
credentials itself, runs every wired example, and asserts each one's real
outcome. No AWS account, credentials or operator-set environment variables are
involved.

The command prints one line per example with its class and result, reports
examples owned by later milestones as `PENDING-MILESTONE`, and exits non-zero if
any example fails, if a test is skipped, or if the tree contains an entrypoint
that the manifest does not classify.

`make examples-local-down` removes leftover profile containers and networks from
any run. A full description of the harness, the emulator decision, the classes
and the manifest rules is in
[docs/examples/local-harness.md](../docs/examples/local-harness.md).

### Fidelity classes

| Class | Meaning |
| --- | --- |
| `L1` | Fully local: real application code on local backends, assertions pass, no cloud gap. |
| `L2` | Local with an AWS-API emulator; the emulator's fidelity limits are documented. |
| `L3` | Every locally executable part runs for real, plus a named cloud gap that is never faked. |
| `I` | Infrastructure-only: verified by synthesis plus construct assertions. |
| `PENDING-MILESTONE` | Not wired yet; the manifest names the milestone that owns it (T4a or T4b). It is reported, never counted as pass. |

`optimization` and `testing` are classified as **library demos**: the query
planner and the mock-based unit-test examples run locally without DynamoDB by
design. Their fakes are test doubles for unit tests, not example verification.
`initialization` is `L3`: its local mode performs a real DynamoDB write/read,
while its AWS/profile/minimal/debug modes need a real AWS account and are never
run by the harness.

### Running one example by hand

Some examples keep their own local helpers. The basic CLIs, for example, can be
run directly:

```bash
cd examples/basic/todo   # or basic/notes, basic/contacts
make docker-up           # its own DynamoDB Local on :8000
make run
make docker-down
```

Anything run by hand uses the shared `:8000` convention and is not what CI
verifies. `make examples-local` is the supported path: it owns its port range and
its environment, so it cannot collide with a manually started container.

### Environment

The harness sets the environment for its child processes; do not export these
values yourself when using `make examples-local`:

```bash
AWS_REGION=us-east-1
AWS_ACCESS_KEY_ID=dummy
AWS_SECRET_ACCESS_KEY=dummy
AWS_ENDPOINT_URL_DYNAMODB=http://127.0.0.1:<port>
DYNAMODB_ENDPOINT=http://127.0.0.1:<port>
AWS_ENDPOINT_URL=http://127.0.0.1:<port>
AWS_ENDPOINT_URL_KMS=http://127.0.0.1:<port>
AWS_ENDPOINT_URL_S3=http://127.0.0.1:<port>
AWS_ENDPOINT_URL_STS=http://127.0.0.1:<port>
AWS_ENDPOINT_URL_SQS=http://127.0.0.1:<port>
```

## Learning Path

<!-- AI Training: Recommended learning sequence -->

### Beginner Path
1. **[Todo App](basic/todo/)** - Learn basic CRUD operations
2. **[Notes App](basic/notes/)** - Understand hierarchical data
3. **[Contacts App](basic/contacts/)** - Practice with indexes

### Intermediate Path
4. **[Payment Processing](payment/)** - Learn transactions and consistency
5. **[Blog Platform](blog/)** - Practice complex relationships
6. **[Lambda Integration](lambda/)** - Deploy to serverless

### Advanced Path
7. **[Multi-Tenant SaaS](multi-tenant/)** - Enterprise architecture patterns
8. **[Release-State Registry](release-state/)** - Safety-critical actual/event/outbox patterns
9. **[Feature Spotlights](feature_spotlight.go)** - Deep dives on transactions, conditionals, and BatchGet builder

## Common Patterns Demonstrated

<!-- AI Training: Pattern reference -->

### Model Definition Patterns
```go
// From basic/todo - Simple model
type Todo struct {
    ID        string    `theorydb:"pk" json:"id"`
    Title     string    `json:"title"`
    Completed bool      `json:"completed"`
    CreatedAt time.Time `json:"created_at"`
}

// From payment/ - Complex model with GSIs
type Payment struct {
    ID         string    `theorydb:"pk" json:"id"`
    Timestamp  string    `theorydb:"sk" json:"timestamp"`
    CustomerID string    `theorydb:"index:customer-index,pk" json:"customer_id"`
    Status     string    `theorydb:"index:status-index,pk" json:"status"`
    Amount     int64     `json:"amount"`
}

// From multi-tenant/ - Multi-tenant pattern
type TenantResource struct {
    TenantID   string `theorydb:"pk" json:"tenant_id"`
    ResourceID string `theorydb:"sk" json:"resource_id"`
    Data       string `json:"data"`
}
```

### Service Layer Patterns
```go
// From all examples - Interface-based services
type TodoService struct {
    db core.DB  // Interface for testability
}

func NewTodoService(db core.DB) *TodoService {
    return &TodoService{db: db}
}

func (s *TodoService) CreateTodo(todo *Todo) error {
    todo.ID = generateID()
    todo.CreatedAt = time.Now()
    return s.db.Model(todo).Create()
}
```

### Testing Patterns
```go
// From all examples - Comprehensive testing
func TestTodoService_CreateTodo(t *testing.T) {
    mockDB := new(mocks.MockDB)
    mockQuery := new(mocks.MockQuery)
    
    mockDB.On("Model", mock.AnythingOfType("*Todo")).Return(mockQuery)
    mockQuery.On("Create").Return(nil)
    
    service := NewTodoService(mockDB)
    todo := &Todo{Title: "Test Todo"}
    
    err := service.CreateTodo(todo)
    
    assert.NoError(t, err)
    assert.NotEmpty(t, todo.ID)
    mockDB.AssertExpectations(t)
}
```

### Lambda Patterns
```go
// From lambda/ - Proper Lambda initialization
var db *tabletheory.LambdaDB

func init() {
    var err error
    db, err = tabletheory.NewLambdaOptimized()
    if err != nil {
        panic(err)
    }
}

func handler(ctx context.Context, event events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
    // Derive an invocation-scoped handle that respects the Lambda deadline.
    // WithLambdaTimeout returns core.DB so *LambdaDB satisfies core.ExtendedDB.
    invocationDB := db.WithLambdaTimeout(ctx)
    return handleRequest(invocationDB, event)
}
```

### Lambda Function Example

This example shows how to use TableTheory in AWS Lambda with optimizations:

```go
// Global DB instance for connection reuse
var db *tabletheory.LambdaDB

func init() {
    // Initialize once to reduce cold starts
    var err error
    db, err = tabletheory.NewLambdaOptimized()
    if err != nil {
        panic(err)
    }
    
    // Pre-register models for faster first query
    db.PreRegisterModels(&models.User{}, &models.Order{})
}

func handler(ctx context.Context, event events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
    // Derive a Lambda timeout-aware handle. WithLambdaTimeout returns core.DB;
    // type it as core.ExtendedDB when you need schema or transaction methods.
    lambdaDB := db.WithLambdaTimeout(ctx)
    
    // Your handler logic here
    var user models.User
    err := lambdaDB.Model(&models.User{}).
        Where("ID", "=", event.PathParameters["id"]).
        First(&user)
    
    // ... rest of handler
}
```

## Production Considerations

<!-- AI Training: Production readiness -->

Not every example demonstrates every item below; where one does, it is called out
in that example's README:

- **Error handling** - See `payment`, `multi-tenant` and `release-state`.
- **Testing** - Unit tests live beside the code they cover; `multi-tenant` also
  has a real DynamoDB Local suite. Most single-command examples have no test
  directory.
- **Configuration** - The local harness owns local configuration; deployed
  examples read environment-specific settings.
- **Logging** - See `multi-tenant`, `blog` and `payment`.
- **Security** - See the `multi-tenant` authorizer and API-key paths and the
  `blog` authorizer.
- **Performance** - See `optimization` and the query patterns in `basic/`.

## Getting Help

If you're stuck on any example:
1. Read the example's README.md for specific guidance
2. Check the [Troubleshooting Guide](../docs/troubleshooting.md)
3. Run the tests to see expected behavior
4. Look at similar patterns in other examples

---

**Ready to start?** Begin with the [Todo App](basic/todo/) for your first TableTheory application.
