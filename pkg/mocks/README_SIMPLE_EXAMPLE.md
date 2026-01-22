# 🧪 TableTheory Mocking - Simple Examples

Having trouble understanding TableTheory mocks? This guide shows you exactly how to use them with real examples!

## 🚀 Quick Start

### 1. The Problem
You want to test your business logic without hitting a real database.

### 2. The Solution
Use TableTheory mocks to simulate database operations in your tests.

### 3. Simple Example

```go
// Your service (the code you want to test)
type UserService struct {
    db core.DB
}

func (s *UserService) GetUser(id string) (*User, error) {
    var user User
    err := s.db.Model(&User{}).Where("ID", "=", id).First(&user)
    return &user, err
}

// Your test (how to mock it)
func TestGetUser(t *testing.T) {
    // 🔧 Create mocks
    mockDB := new(mocks.MockDB)
    mockQuery := new(mocks.MockQuery)
    
    // 🎯 Set expectations (what should be called)
    mockDB.On("Model", mock.AnythingOfType("*User")).Return(mockQuery)
    mockQuery.On("Where", "ID", "=", "123").Return(mockQuery)
    mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
        // 📥 Put fake data into the result
        user := args.Get(0).(*User)
        user.ID = "123"
        user.Name = "John Doe"
    }).Return(nil)
    
    // 🚀 Test your service
    service := NewUserService(mockDB)
    result, err := service.GetUser("123")
    
    // ✅ Verify it worked
    assert.NoError(t, err)
    assert.Equal(t, "123", result.ID)
    assert.Equal(t, "John Doe", result.Name)
    
    // 🔍 Make sure mocks were called as expected
    mockDB.AssertExpectations(t)
    mockQuery.AssertExpectations(t)
}
```

## 📋 Step-by-Step Process

### Step 1: Create Mock Objects
```go
mockDB := new(mocks.MockDB)          // Mock the database
mockQuery := new(mocks.MockQuery)    // Mock the query builder
```

### Step 2: Set Up Expectations
Tell the mocks what methods should be called and what they should return:

```go
// When Model() is called, return mockQuery
mockDB.On("Model", mock.Anything).Return(mockQuery)

// When Where() is called, return mockQuery (for chaining)
mockQuery.On("Where", "ID", "=", "123").Return(mockQuery)

// When First() is called, populate the result and return no error
mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
    user := args.Get(0).(*User)
    user.ID = "123"
    user.Name = "Test User"
}).Return(nil)
```

### Step 3: Execute Your Code
```go
service := NewUserService(mockDB)    // Pass the mock to your service
result, err := service.GetUser("123") // Call the method you're testing
```

### Step 4: Verify Results
```go
assert.NoError(t, err)                    // Check there's no error
assert.Equal(t, "123", result.ID)         // Check the results are correct
mockDB.AssertExpectations(t)              // Verify mocks were called
mockQuery.AssertExpectations(t)
```

## 🔗 Method Chaining

TableTheory uses method chaining like `db.Model().Where().OrderBy().All()`. 

For mocks to work with chaining, **chainable methods must return the mock itself**:

```go
// ✅ CORRECT: Return the mock for chaining
mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
mockQuery.On("All", mock.Anything).Return(nil)

// ❌ WRONG: Returning nil breaks the chain
mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(nil)
```

## 📝 Common Patterns

### Pattern 1: Don't Care About Arguments
```go
mockDB.On("Model", mock.Anything).Return(mockQuery)
```

### Pattern 2: Check Specific Types
```go
mockDB.On("Model", mock.AnythingOfType("*User")).Return(mockQuery)
```

### Pattern 3: Populate Output Parameters
```go
mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
    user := args.Get(0).(*User)
    user.ID = "123"
    user.Name = "John"
}).Return(nil)
```

### Pattern 4: Populate Slices
```go
mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
    users := args.Get(0).(*[]User)
    *users = []User{{ID: "1", Name: "Alice"}, {ID: "2", Name: "Bob"}}
}).Return(nil)
```

### Pattern 5: Return Errors
```go
mockQuery.On("First", mock.Anything).Return(errors.New("not found"))
```

### Pattern 6: Update Operations
```go
mockUpdateBuilder := new(mocks.MockUpdateBuilder)
mockQuery.On("UpdateBuilder").Return(mockUpdateBuilder)
mockUpdateBuilder.On("Set", "Email", "new@email.com").Return(mockUpdateBuilder)
mockUpdateBuilder.On("Execute").Return(nil)
```

### Pattern 7: Transactions (`TransactWrite`)
If your code uses the transaction DSL:

```go
err := db.TransactWrite(ctx, func(tx core.TransactionBuilder) error {
    tx.WithContext(ctx)
    tx.UpdateWithBuilder(&User{ID: "123"}, func(ub core.UpdateBuilder) error {
        ub.Set("Status", "active")
        return nil
    })
    return nil
})
```

You can mock it without boilerplate `.Run(...)` calls by providing a `MockTransactionBuilder`:

```go
mockDB := new(mocks.MockExtendedDB)
mockTx := new(mocks.MockTransactionBuilder)

// Tell the ExtendedDB mock which builder to run the callback with
mockDB.TransactWriteBuilder = mockTx

mockDB.On("TransactWrite", ctx, mock.Anything).Return(nil)
mockTx.On("WithContext", ctx).Return(mockTx)
mockTx.On("UpdateWithBuilder", mock.Anything, mock.Anything, mock.Anything).Return(mockTx)
mockTx.On("Execute").Return(nil)
```

## 🚨 Common Mistakes

### ❌ Mistake 1: Forgetting AssertExpectations
```go
// You set up expectations but forget to verify them
mockDB.On("Model", mock.Anything).Return(mockQuery)
// ... but never call mockDB.AssertExpectations(t)
```

### ❌ Mistake 2: Wrong Return Types
```go
// This breaks chaining because Where() returns nil instead of mockQuery
mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(nil)
```

### ❌ Mistake 3: Not Calling Your Service
```go
// You set up mocks but never actually call the method you're testing
mockDB.On("Model", mock.Anything).Return(mockQuery)
// service.GetUser("123") // <-- This line is missing!
mockDB.AssertExpectations(t) // This will fail because nothing was called
```

## 📚 Complete Examples

Check out these files for complete, working examples:
- `simple_example.go` - Shows a real service that uses TableTheory
- `simple_example_test.go` - Shows how to test it with mocks

## 🎯 Key Takeaways

1. **Mocks simulate your database** - no real DB needed for tests
2. **Set expectations first** - tell mocks what should be called
3. **Chain methods return themselves** - `Return(mockQuery)` for chaining
4. **Use `Run()` to populate results** - put fake data in output parameters  
5. **Always verify expectations** - call `AssertExpectations(t)`
6. **Use `mock.Anything`** when you don't care about specific arguments

Happy testing! 🎉 
