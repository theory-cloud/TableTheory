// Package mocks provides mock implementations for TableTheory interfaces.
//
// This package solves the common issue of having to implement all 26+ methods
// of the core.Query interface when writing unit tests. Instead of discovering
// missing methods through trial and error, you can use these pre-built mocks.
//
// # Installation
//
// Import the mocks package in your test files:
//
//	import "github.com/theory-cloud/tabletheory/pkg/mocks"
//
// # Basic Usage
//
// The most common use case is mocking database queries:
//
//	func TestUserService(t *testing.T) {
//	    // Create mocks
//	    mockDB := new(mocks.MockDB)
//	    mockQuery := new(mocks.MockQuery)
//
//	    // Setup expectations
//	    mockDB.On("Model", &User{}).Return(mockQuery)
//	    mockQuery.On("Where", "ID", "=", "123").Return(mockQuery)
//	    mockQuery.On("First", mock.Anything).Return(nil)
//
//	    // Use in your service
//	    service := NewUserService(mockDB)
//	    user, err := service.GetUser("123")
//
//	    // Assert expectations were met
//	    mockDB.AssertExpectations(t)
//	    mockQuery.AssertExpectations(t)
//	}
//
// # Chaining Methods
//
// Query methods typically return themselves to allow chaining:
//
//	mockQuery.On("Where", "Status", "=", "active").Return(mockQuery)
//	mockQuery.On("OrderBy", "CreatedAt", "DESC").Return(mockQuery)
//	mockQuery.On("Limit", 10).Return(mockQuery)
//	mockQuery.On("All", mock.Anything).Return(nil)
//
// # Working with Results
//
// To return data from queries, use mock.Run to populate the destination:
//
//	users := []User{{ID: "1", Name: "Alice"}, {ID: "2", Name: "Bob"}}
//	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
//	    dest := args.Get(0).(*[]User)
//	    *dest = users
//	}).Return(nil)
//
// # Error Handling
//
// To simulate errors:
//
//	mockQuery.On("First", mock.Anything).Return(errors.New("not found"))
//
// # Update Operations
//
// For update operations with the builder pattern:
//
//	mockUpdateBuilder := new(mocks.MockUpdateBuilder)
//	mockQuery.On("UpdateBuilder").Return(mockUpdateBuilder)
//	mockUpdateBuilder.On("Set", "Status", "completed").Return(mockUpdateBuilder)
//	mockUpdateBuilder.On("Execute").Return(nil)
//
// # AWS SDK Level Mocking
//
// For testing infrastructure code that directly uses the AWS SDK:
//
//	mockClient := new(mocks.MockDynamoDBClient)
//	mockWaiter := new(mocks.MockTableExistsWaiter)
//
//	// Mock table creation
//	mockClient.On("CreateTable", mock.Anything, mock.Anything, mock.Anything).
//		Return(mocks.NewMockCreateTableOutput("test-table"), nil)
//
//	// Mock waiting for table to be ready
//	mockWaiter.On("Wait", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
//		Return(nil)
//
// # Tips
//
// 1. Use mock.Anything when you don't need to assert on specific arguments
// 2. Use mock.MatchedBy for custom argument matching
// 3. Always assert expectations were met with AssertExpectations
// 4. Return the mock itself for chainable methods
// 5. Use Run to modify output parameters before returning
// 6. Use the helper functions (NewMockCreateTableOutput, etc.) for realistic responses
// 7. AWS SDK mocks complement TableTheory interface mocks for complete test coverage
package mocks

// Helper type aliases for convenience
type (
	// Query is an alias for MockQuery to allow shorter declarations
	Query = MockQuery

	// DB is an alias for MockDB to allow shorter declarations
	DB = MockDB

	// TransactionBuilder is an alias for MockTransactionBuilder to simplify usage in tests.
	TransactionBuilder = MockTransactionBuilder

	// UpdateBuilder is an alias for MockUpdateBuilder to allow shorter declarations
	UpdateBuilder = MockUpdateBuilder

	// BatchGetBuilder is an alias for MockBatchGetBuilder to simplify usage in tests.
	BatchGetBuilder = MockBatchGetBuilder

	// ExtendedDB is an alias for MockExtendedDB to allow shorter declarations
	ExtendedDB = MockExtendedDB
)
