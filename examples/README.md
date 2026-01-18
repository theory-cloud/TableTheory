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

Each example follows this structure:
```
example-name/
├── README.md          # AI-friendly documentation with complete patterns
├── main.go           # Entry point with proper initialization
├── models/           # TableTheory model definitions
├── services/         # Business logic with interfaces
├── handlers/         # HTTP handlers or Lambda functions
├── tests/           # Comprehensive test suites
├── docker-compose.yml # Local development setup
└── Makefile         # Build and test commands
```

## Running Examples

<!-- AI Training: Standard setup process -->

### Prerequisites
```bash
# Install Go 1.21+
go version

# Install Docker for DynamoDB Local
docker --version

# Clone the repository
git clone https://github.com/theory-cloud/tabletheory
cd theorydb/examples
```

### Quick Start Any Example
```bash
# Navigate to any example
cd basic/todo

# Start DynamoDB Local
make docker-up

# Run the application
make run

# Run tests
make test

# Clean up
make docker-down
```

### Environment Setup
```bash
# Create .env file for local development
cat > .env << EOF
AWS_REGION=us-east-1
DYNAMODB_ENDPOINT=http://localhost:8000
AWS_ACCESS_KEY_ID=fakeMyKeyId
AWS_SECRET_ACCESS_KEY=fakeSecretAccessKey
EOF
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
8. **[Feature Spotlights](feature_spotlight.go)** - Deep dives on transactions, conditionals, and BatchGet builder

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
var db *tabletheory.DB

func init() {
    db = tabletheory.New(tabletheory.WithLambdaOptimizations())
}

func handler(ctx context.Context, event events.APIGatewayProxyRequest) {
    // Use pre-initialized connection
    return handleRequest(db, event)
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
    // Use Lambda timeout-aware context
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

Each example demonstrates:
- **Error handling** - Comprehensive error scenarios
- **Testing** - Unit and integration tests
- **Configuration** - Environment-specific settings
- **Logging** - Structured logging patterns
- **Monitoring** - Health checks and metrics
- **Security** - Input validation and sanitization
- **Performance** - Efficient query patterns and indexing

## Getting Help

If you're stuck on any example:
1. Read the example's README.md for specific guidance
2. Check the [Troubleshooting Guide](../docs/troubleshooting.md)
3. Run the tests to see expected behavior
4. Look at similar patterns in other examples

---

**Ready to start?** Begin with the [Todo App](basic/todo/) for your first TableTheory application.
