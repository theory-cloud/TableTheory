package mocks_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
)

// TestMockQueryImplementsInterface verifies MockQuery implements core.Query
func TestMockQueryImplementsInterface(t *testing.T) {
	var _ core.Query = (*mocks.MockQuery)(nil)
}

// TestMockDBImplementsInterface verifies MockDB implements core.DB
func TestMockDBImplementsInterface(t *testing.T) {
	var _ core.DB = (*mocks.MockDB)(nil)
}

// TestMockUpdateBuilderImplementsInterface verifies MockUpdateBuilder implements core.UpdateBuilder
func TestMockUpdateBuilderImplementsInterface(t *testing.T) {
	var _ core.UpdateBuilder = (*mocks.MockUpdateBuilder)(nil)
}

// Example user struct for testing
type User struct {
	ID     string
	Email  string
	Name   string
	Status string
}

// TestBasicQueryChaining demonstrates mocking a chained query
func TestBasicQueryChaining(t *testing.T) {
	// Create mocks
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// Setup expectations
	mockDB.On("Model", &User{}).Return(mockQuery)
	mockQuery.On("Where", "ID", "=", "123").Return(mockQuery)
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		// Populate the result
		user, ok := args.Get(0).(*User)
		if !ok {
			t.Fatalf("expected *User, got %T", args.Get(0))
		}
		user.ID = "123"
		user.Name = "John Doe"
		user.Email = "john@example.com"
	}).Return(nil)

	// Execute the code under test
	db := core.DB(mockDB)
	var user User
	err := db.Model(&User{}).Where("ID", "=", "123").First(&user)

	// Assert results
	assert.NoError(t, err)
	assert.Equal(t, "123", user.ID)
	assert.Equal(t, "John Doe", user.Name)
	assert.Equal(t, "john@example.com", user.Email)

	// Verify all expectations were met
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// TestComplexQueryChaining demonstrates mocking a more complex query
func TestComplexQueryChaining(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// Setup chained method calls
	mockDB.On("Model", &User{}).Return(mockQuery)
	mockQuery.On("Where", "Status", "=", "active").Return(mockQuery)
	mockQuery.On("OrderBy", "CreatedAt", "DESC").Return(mockQuery)
	mockQuery.On("Limit", 10).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		users, ok := args.Get(0).(*[]User)
		if !ok {
			t.Fatalf("expected *[]User, got %T", args.Get(0))
		}
		*users = []User{
			{ID: "1", Name: "Alice", Status: "active"},
			{ID: "2", Name: "Bob", Status: "active"},
		}
	}).Return(nil)

	// Execute
	db := core.DB(mockDB)
	var users []User
	err := db.Model(&User{}).
		Where("Status", "=", "active").
		OrderBy("CreatedAt", "DESC").
		Limit(10).
		All(&users)

	// Assert
	assert.NoError(t, err)
	assert.Len(t, users, 2)
	assert.Equal(t, "Alice", users[0].Name)
	assert.Equal(t, "Bob", users[1].Name)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// TestErrorHandling demonstrates mocking errors
func TestErrorHandling(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	expectedErr := errors.New("user not found")

	mockDB.On("Model", &User{}).Return(mockQuery)
	mockQuery.On("Where", "ID", "=", "999").Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(expectedErr)

	// Execute
	db := core.DB(mockDB)
	var user User
	err := db.Model(&User{}).Where("ID", "=", "999").First(&user)

	// Assert
	assert.ErrorIs(t, err, expectedErr)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// TestUpdateBuilder demonstrates mocking update operations
func TestUpdateBuilder(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)

	// Setup expectations
	mockDB.On("Model", &User{ID: "123"}).Return(mockQuery)
	mockQuery.On("UpdateBuilder").Return(mockUpdateBuilder)
	mockUpdateBuilder.On("Set", "Status", "inactive").Return(mockUpdateBuilder)
	mockUpdateBuilder.On("Set", "Name", "Jane Doe").Return(mockUpdateBuilder)
	mockUpdateBuilder.On("Execute").Return(nil)

	// Execute
	db := core.DB(mockDB)
	err := db.Model(&User{ID: "123"}).
		UpdateBuilder().
		Set("Status", "inactive").
		Set("Name", "Jane Doe").
		Execute()

	// Assert
	assert.NoError(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
	mockUpdateBuilder.AssertExpectations(t)
}

// TestBatchOperations demonstrates mocking batch operations
func TestBatchOperations(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	users := []User{
		{ID: "1", Name: "Alice"},
		{ID: "2", Name: "Bob"},
	}

	mockDB.On("Model", &User{}).Return(mockQuery)
	mockQuery.On("BatchCreate", users).Return(nil)

	// Execute
	db := core.DB(mockDB)
	err := db.Model(&User{}).BatchCreate(users)

	// Assert
	assert.NoError(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// TestWithContext demonstrates mocking context
func TestWithContext(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockDBWithCtx := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDBWithCtx)
	mockDBWithCtx.On("Model", &User{}).Return(mockQuery)
	mockQuery.On("Where", "ID", "=", "123").Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(nil)

	// Execute
	db := core.DB(mockDB)
	var user User
	err := db.WithContext(ctx).Model(&User{}).Where("ID", "=", "123").First(&user)

	// Assert
	assert.NoError(t, err)

	mockDB.AssertExpectations(t)
	mockDBWithCtx.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// TestTypeAliases verifies that type aliases work
func TestTypeAliases(t *testing.T) {
	// These should compile without issues
	_ = new(mocks.Query)
	_ = new(mocks.DB)
	_ = new(mocks.UpdateBuilder)
}
