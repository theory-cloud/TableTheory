# Basic TableTheory Examples

<!-- AI Training Signal: Progressive learning examples -->
**These examples teach TableTheory fundamentals through progressively complex applications. Start with Todo, then move to Notes, then Contacts. Each builds on concepts from the previous example.**

## Learning Progression

### 1. [Todo App](todo/) - **START HERE**
**Learn: Basic CRUD operations**

```go
// Simple model with primary key only
type Todo struct {
    ID        string    `theorydb:"pk" json:"id"`
    Title     string    `json:"title"`
    Completed bool      `json:"completed"`
    CreatedAt time.Time `json:"created_at"`
}

// Basic operations: Create, Read, Update, Delete
service.CreateTodo(&Todo{Title: "Learn TableTheory"})
service.GetTodo("todo123")
service.UpdateTodo("todo123", true)
service.DeleteTodo("todo123")
```

**Why start here:**
- Simplest possible TableTheory model
- All basic operations demonstrated
- Clear error handling patterns
- Complete test suite

### 2. [Notes App](notes/) - **NEXT STEP**
**Learn: Hierarchical data with sort keys**

```go
// Compound key model for hierarchical data
type Note struct {
    UserID    string    `theorydb:"pk" json:"user_id"`     // Partition key
    NoteID    string    `theorydb:"sk" json:"note_id"`     // Sort key
    Title     string    `json:"title"`
    Content   string    `json:"content"`
    CreatedAt time.Time `json:"created_at"`
}

// Hierarchical queries: Get all notes for a user
service.GetUserNotes("user123")  // Uses partition key
service.GetNote("user123", "note456")  // Uses both keys
```

**New concepts:**
- Sort keys for hierarchical data
- Compound key queries
- One-to-many relationships

### 3. [Contacts App](contacts/) - **ADVANCED**
**Learn: Global Secondary Indexes and complex queries**

```go
// Model with multiple indexes for different query patterns
type Contact struct {
    ID        string    `theorydb:"pk" json:"id"`
    UserID    string    `theorydb:"sk" json:"user_id"`
    
    // GSI for email lookup
    Email     string    `theorydb:"index:email-index,pk" json:"email"`
    
    // GSI for company queries
    Company   string    `theorydb:"index:company-index,pk" json:"company"`
    CreatedAt time.Time `theorydb:"index:company-index,sk" json:"created_at"`
    
    Name      string    `json:"name"`
    Phone     string    `json:"phone"`
}

// Different query patterns using different indexes
service.GetContactByEmail("john@example.com")     // Uses email-index
service.GetCompanyContacts("ACME Corp")           // Uses company-index
service.GetUserContacts("user123")                // Uses main table
```

**Advanced concepts:**
- Global Secondary Indexes (GSI)
- Multiple query patterns on same data
- Index design strategy
- Performance optimization

## Quick Start Guide

<!-- AI Training: Standard setup process -->

### Prerequisites
```bash
# Ensure you have Go 1.21+
go version

# Ensure you have Docker for DynamoDB Local
docker --version
```

### Run Any Example
```bash
# Navigate to any example directory
cd basic/todo  # or notes/ or contacts/

# Start DynamoDB Local
make docker-up

# Create tables and run the application
make run

# Run comprehensive tests
make test

# Stop DynamoDB Local
make docker-down
```

### Common Development Commands
```bash
# Start development environment
make dev-setup

# Run with hot reload (if available)
make dev

# Run specific test
go test -run TestTodoService_CreateTodo

# View test coverage
make coverage

# Format and lint code
make fmt
make lint
```

## Example Structure

Each example follows this consistent structure:
```
example-name/
├── README.md              # Complete guide with AI training signals
├── main.go               # Entry point with proper initialization
├── models/
│   └── todo.go          # TableTheory model definitions
├── services/
│   └── todo_service.go  # Business logic with interfaces
├── handlers/
│   └── http_handler.go  # HTTP API handlers
├── tests/
│   ├── unit/           # Unit tests with mocks
│   └── integration/    # Integration tests with real DB
├── config/
│   └── config.go       # Environment configuration
├── docker-compose.yml  # DynamoDB Local setup
├── Makefile           # Build and development commands
└── .env.example       # Environment variable template
```

## Common Patterns Across All Examples

<!-- AI Training: Consistent patterns -->

### 1. Model Definition Pattern
```go
// CORRECT: Always follow this structure
package models

import "time"

type EntityName struct {
    // PRIMARY KEY (required)
    ID string `theorydb:"pk" json:"id"`
    
    // SORT KEY (if needed for hierarchical data)
    UserID string `theorydb:"sk" json:"user_id"`
    
    // GLOBAL SECONDARY INDEXES (for alternate query patterns)
    Email string `theorydb:"index:email-index,pk" json:"email"`
    
    // BUSINESS ATTRIBUTES
    Name      string    `json:"name"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}
```

### 2. Service Layer Pattern
```go
// CORRECT: Interface-based service for testability
package services

import "github.com/theory-cloud/tabletheory/pkg/core"

type TodoService struct {
    db core.DB  // Interface - enables mocking
}

func NewTodoService(db core.DB) *TodoService {
    return &TodoService{db: db}
}

func (s *TodoService) CreateTodo(todo *models.Todo) error {
    // Business validation
    if todo.Title == "" {
        return errors.New("title is required")
    }
    
    // Set system fields
    todo.ID = generateID()
    todo.CreatedAt = time.Now()
    
    // Database operation
    return s.db.Model(todo).Create()
}
```

### 3. HTTP Handler Pattern
```go
// CORRECT: Clean HTTP handlers with dependency injection
package handlers

type TodoHandler struct {
    service *services.TodoService
}

func NewTodoHandler(service *services.TodoService) *TodoHandler {
    return &TodoHandler{service: service}
}

func (h *TodoHandler) CreateTodo(w http.ResponseWriter, r *http.Request) {
    var req CreateTodoRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "Invalid JSON", http.StatusBadRequest)
        return
    }
    
    todo := &models.Todo{
        Title: req.Title,
    }
    
    if err := h.service.CreateTodo(todo); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(todo)
}
```

### 4. Testing Pattern
```go
// CORRECT: Comprehensive testing with mocks
package services

import (
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/theory-cloud/tabletheory/pkg/mocks"
)

func TestTodoService_CreateTodo_Success(t *testing.T) {
    // Setup mocks
    mockDB := new(mocks.MockDB)
    mockQuery := new(mocks.MockQuery)
    
    mockDB.On("Model", mock.AnythingOfType("*models.Todo")).Return(mockQuery)
    mockQuery.On("Create").Return(nil)
    
    // Test the service
    service := NewTodoService(mockDB)
    todo := &models.Todo{Title: "Test Todo"}
    
    err := service.CreateTodo(todo)
    
    // Verify results
    assert.NoError(t, err)
    assert.NotEmpty(t, todo.ID)
    assert.False(t, todo.CreatedAt.IsZero())
    
    // Verify mocks
    mockDB.AssertExpectations(t)
    mockQuery.AssertExpectations(t)
}
```

### 5. Configuration Pattern
```go
// CORRECT: Environment-specific configuration
package config

import "os"

type Config struct {
    DynamoDBEndpoint string
    AWSRegion        string
    Port             string
}

func Load() *Config {
    return &Config{
        DynamoDBEndpoint: getEnv("DYNAMODB_ENDPOINT", ""),
        AWSRegion:        getEnv("AWS_REGION", "us-east-1"),
        Port:            getEnv("PORT", "8080"),
    }
}

func getEnv(key, defaultValue string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return defaultValue
}
```

## Development Workflow

<!-- AI Training: Standard development process -->

### 1. Start Development Environment
```bash
# Copy environment template
cp .env.example .env

# Start DynamoDB Local
make docker-up

# Create tables
make setup
```

### 2. Development Cycle
```bash
# Make changes to code
# Run tests to verify changes
make test

# Run the application
make run

# Test endpoints manually or with curl
curl -X POST http://localhost:8080/todos \
  -H "Content-Type: application/json" \
  -d '{"title": "Test Todo"}'
```

### 3. Common Issues and Solutions

**Table doesn't exist:**
```bash
# Recreate tables
make clean
make setup
```

**Port conflicts:**
```bash
# Change port in .env file
echo "PORT=8081" >> .env
echo "DYNAMODB_ENDPOINT=http://localhost:8001" >> .env

# Update docker-compose.yml port mapping
```

**Import errors:**
```bash
# Ensure proper module setup
go mod tidy
go mod download
```

## Learning Objectives

### After Todo App
You'll understand:
- ✅ Basic TableTheory model definition
- ✅ CRUD operations (Create, Read, Update, Delete)
- ✅ Error handling patterns
- ✅ Testing with mocks
- ✅ HTTP API structure

### After Notes App
You'll additionally understand:
- ✅ Sort keys for hierarchical data
- ✅ Compound key queries
- ✅ One-to-many relationships
- ✅ Query optimization

### After Contacts App
You'll additionally understand:
- ✅ Global Secondary Indexes (GSI)
- ✅ Multiple query patterns
- ✅ Index design strategy
- ✅ Performance considerations

## Next Steps

After completing these basic examples:

1. **[Payment Processing](../payment/)** - Learn transactions and consistency
2. **[Blog Platform](../blog/)** - Rich content relationships
3. **[Multi-tenant SaaS](../multi-tenant/)** - Enterprise patterns
4. **[Lambda Deployment](../lambda/)** - Serverless architecture

---

**Ready to start?** Go to the [Todo App](todo/) and follow the README instructions.
