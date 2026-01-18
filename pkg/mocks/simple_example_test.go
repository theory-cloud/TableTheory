package mocks_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/theory-cloud/tabletheory/pkg/mocks"
)

// 🧪 SIMPLE DYNAMORM MOCKING EXAMPLES
//
// These tests show you exactly how to mock TableTheory operations
// in real-world scenarios. Follow along step by step!

// Example 1: Basic Query Mocking - GetUser
func TestUserService_GetUser_Success(t *testing.T) {
	// 🔧 SETUP: Create your mocks
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// 🎯 EXPECTATIONS: Tell the mocks what should be called
	mockDB.On("Model", mock.AnythingOfType("*mocks.User")).Return(mockQuery)
	mockQuery.On("Where", "ID", "=", "user123").Return(mockQuery)
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		// 📥 POPULATE RESULT: Put data into the result pointer
		user, ok := args.Get(0).(*mocks.User)
		if !ok {
			t.Fatalf("expected *mocks.User, got %T", args.Get(0))
		}
		user.ID = "user123"
		user.Name = "John Doe"
		user.Email = "john@example.com"
		user.Age = 25
	}).Return(nil) // Return no error

	// 🚀 EXECUTE: Call your service method
	service := mocks.NewUserService(mockDB)
	result, err := service.GetUser("user123")

	// ✅ VERIFY: Check results and that all expectations were met
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "user123", result.ID)
	assert.Equal(t, "John Doe", result.Name)
	assert.Equal(t, "john@example.com", result.Email)
	assert.Equal(t, 25, result.Age)

	// 🔍 IMPORTANT: Always verify mocks were called as expected
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// Example 2: Testing Error Scenarios
func TestUserService_GetUser_NotFound(t *testing.T) {
	// Setup mocks
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// Setup expectations - this time return an error
	mockDB.On("Model", mock.AnythingOfType("*mocks.User")).Return(mockQuery)
	mockQuery.On("Where", "ID", "=", "nonexistent").Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(errors.New("item not found"))

	// Execute
	service := mocks.NewUserService(mockDB)
	result, err := service.GetUser("nonexistent")

	// Verify error handling
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to get user nonexistent")

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// Example 3: Testing Create Operations
func TestUserService_CreateUser(t *testing.T) {
	// Setup
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// The user we want to create
	newUser := &mocks.User{
		ID:    "user456",
		Name:  "Jane Smith",
		Email: "jane@example.com",
		Age:   30,
	}

	// Expectations
	mockDB.On("Model", newUser).Return(mockQuery)
	mockQuery.On("Create").Return(nil) // Successful creation

	// Execute
	service := mocks.NewUserService(mockDB)
	err := service.CreateUser(newUser)

	// Verify
	assert.NoError(t, err)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// Example 4: Testing Queries with Multiple Conditions and Chaining
func TestUserService_GetActiveUsers(t *testing.T) {
	// Setup
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// Expected results
	expectedUsers := []mocks.User{
		{ID: "user1", Name: "Alice", Email: "alice@example.com", Age: 25},
		{ID: "user2", Name: "Bob", Email: "bob@example.com", Age: 30},
	}

	// Expectations - notice the chaining: Where -> OrderBy -> All
	mockDB.On("Model", mock.AnythingOfType("*mocks.User")).Return(mockQuery)
	mockQuery.On("Where", "Age", ">=", 18).Return(mockQuery) // Chain: returns self
	mockQuery.On("OrderBy", "Name", "ASC").Return(mockQuery) // Chain: returns self
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		// Populate the slice pointer with our expected users
		users, ok := args.Get(0).(*[]mocks.User)
		if !ok {
			t.Fatalf("expected *[]mocks.User, got %T", args.Get(0))
		}
		*users = expectedUsers
	}).Return(nil)

	// Execute
	service := mocks.NewUserService(mockDB)
	result, err := service.GetActiveUsers()

	// Verify
	assert.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Equal(t, "Alice", result[0].Name)
	assert.Equal(t, "Bob", result[1].Name)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// Example 5: Testing Update Operations with UpdateBuilder
func TestUserService_UpdateUserEmail(t *testing.T) {
	// Setup
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)

	// Expectations for the update chain: Model -> Where -> UpdateBuilder -> Set -> Execute
	mockDB.On("Model", mock.AnythingOfType("*mocks.User")).Return(mockQuery)
	mockQuery.On("Where", "ID", "=", "user123").Return(mockQuery)
	mockQuery.On("UpdateBuilder").Return(mockUpdateBuilder)
	mockUpdateBuilder.On("Set", "Email", "newemail@example.com").Return(mockUpdateBuilder)
	mockUpdateBuilder.On("Execute").Return(nil)

	// Execute
	service := mocks.NewUserService(mockDB)
	err := service.UpdateUserEmail("user123", "newemail@example.com")

	// Verify
	assert.NoError(t, err)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
	mockUpdateBuilder.AssertExpectations(t)
}

// 📚 CHEAT SHEET: Common Mock Patterns
func TestMockingCheatSheet(t *testing.T) {
	// This test shows common patterns you'll use

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)

	// ✅ Pattern 1: Don't care about specific arguments
	mockDB.On("Model", mock.Anything).Return(mockQuery)

	// ✅ Pattern 2: Check specific type
	mockDB.On("Model", mock.AnythingOfType("*mocks.User")).Return(mockQuery)

	// ✅ Pattern 3: Method chaining (return self)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)

	// ✅ Pattern 4: Populate output parameter
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		user, ok := args.Get(0).(*mocks.User)
		if !ok {
			t.Fatalf("expected *mocks.User, got %T", args.Get(0))
		}
		user.ID = "test"
		user.Name = "Test User"
	}).Return(nil)

	// ✅ Pattern 5: Populate slice
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		users, ok := args.Get(0).(*[]mocks.User)
		if !ok {
			t.Fatalf("expected *[]mocks.User, got %T", args.Get(0))
		}
		*users = []mocks.User{{ID: "1", Name: "User 1"}}
	}).Return(nil)

	// ✅ Pattern 6: Return errors
	mockQuery.On("Create").Return(errors.New("database error"))

	// ✅ Pattern 7: Update builder pattern
	mockQuery.On("UpdateBuilder").Return(mockUpdateBuilder)
	mockUpdateBuilder.On("Set", mock.Anything, mock.Anything).Return(mockUpdateBuilder)
	mockUpdateBuilder.On("Execute").Return(nil)

	// Note: We're not actually calling anything here, just showing the patterns
	// In real tests, you'd call your service methods and then assert expectations
}

// 🚨 COMMON MISTAKES TO AVOID

// ❌ MISTAKE 1: Forgetting to call AssertExpectations
func TestCommonMistake_ForgottenAssertions(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// Setup expectations
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	// But if you don't call your service method...
	// service := mocks.NewUserService(mockDB)
	// err := service.CreateUser(&mocks.User{}) // <-- This line is commented out!

	// ❌ This will FAIL because the mocked methods were never called
	// mockDB.AssertExpectations(t) // <-- Uncomment to see the failure

	// ✅ LESSON: Always call AssertExpectations AND make sure your service actually calls the mocked methods
}

// ❌ MISTAKE 2: Wrong return types for chaining
func TestCommonMistake_WrongReturnTypes(t *testing.T) {
	mockQuery := new(mocks.MockQuery)

	// ❌ WRONG: Returning nil instead of mockQuery breaks chaining
	// mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	// ✅ CORRECT: Return the mock itself for chaining
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)

	// ✅ LESSON: Chainable methods should return the mock itself
}

/*
🎓 QUICK START GUIDE:

1. Create your mocks:
   mockDB := new(mocks.MockDB)
   mockQuery := new(mocks.MockQuery)

2. Set expectations (what will be called):
   mockDB.On("Model", mock.Anything).Return(mockQuery)
   mockQuery.On("Where", "ID", "=", "123").Return(mockQuery)
   mockQuery.On("First", mock.Anything).Return(nil)

3. Use your service:
   service := NewUserService(mockDB)
   result, err := service.GetUser("123")

4. Verify:
   assert.NoError(t, err)
   mockDB.AssertExpectations(t)
   mockQuery.AssertExpectations(t)

Remember:
- mock.Anything = don't care about the argument
- mock.AnythingOfType("*MyType") = specific type
- Return mockQuery for chaining methods
- Use Run() to populate output parameters
- Always call AssertExpectations(t)
*/
