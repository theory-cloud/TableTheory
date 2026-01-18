package mocks

import (
	"context"
	"reflect"
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/theory-cloud/tabletheory/pkg/core"
	pkgTypes "github.com/theory-cloud/tabletheory/pkg/types"
)

// MockExtendedDB is a complete mock implementation of core.ExtendedDB.
// It embeds MockDB to inherit the base DB interface methods and adds
// the additional methods required by ExtendedDB.
//
// Example usage:
//
//	mockDB := mocks.NewMockExtendedDB()
//	mockQuery := new(mocks.MockQuery)
//	mockDB.On("Model", &User{}).Return(mockQuery)
//	mockQuery.On("Create").Return(nil)
type MockExtendedDB struct {
	MockDB // Embed MockDB to inherit base methods
}

// Ensure MockExtendedDB implements ExtendedDB at compile time
var _ core.ExtendedDB = (*MockExtendedDB)(nil)

// AutoMigrateWithOptions performs enhanced auto-migration with options
func (m *MockExtendedDB) AutoMigrateWithOptions(model any, opts ...any) error {
	args := m.Called(model, opts)
	return args.Error(0)
}

// RegisterTypeConverter registers a custom converter for a specific type
func (m *MockExtendedDB) RegisterTypeConverter(typ reflect.Type, converter pkgTypes.CustomConverter) error {
	args := m.Called(typ, converter)
	return args.Error(0)
}

// CreateTable creates a DynamoDB table for the given model
func (m *MockExtendedDB) CreateTable(model any, opts ...any) error {
	args := m.Called(model, opts)
	return args.Error(0)
}

// EnsureTable checks if a table exists and creates it if not
func (m *MockExtendedDB) EnsureTable(model any) error {
	args := m.Called(model)
	return args.Error(0)
}

// DeleteTable deletes the DynamoDB table for the given model
func (m *MockExtendedDB) DeleteTable(model any) error {
	args := m.Called(model)
	return args.Error(0)
}

// DescribeTable returns the table description for the given model
func (m *MockExtendedDB) DescribeTable(model any) (any, error) {
	args := m.Called(model)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0), args.Error(1)
}

// WithLambdaTimeout sets a deadline based on Lambda context
func (m *MockExtendedDB) WithLambdaTimeout(ctx context.Context) core.DB {
	args := m.Called(ctx)
	return mustCoreDB(args.Get(0))
}

// WithLambdaTimeoutBuffer sets a custom timeout buffer
func (m *MockExtendedDB) WithLambdaTimeoutBuffer(buffer time.Duration) core.DB {
	args := m.Called(buffer)
	return mustCoreDB(args.Get(0))
}

// TransactionFunc executes a function within a full transaction context
func (m *MockExtendedDB) TransactionFunc(fn func(tx any) error) error {
	args := m.Called(fn)
	return args.Error(0)
}

// Transact returns a transaction builder mock
func (m *MockExtendedDB) Transact() core.TransactionBuilder {
	args := m.Called()
	if builder, ok := args.Get(0).(core.TransactionBuilder); ok {
		return builder
	}
	return nil
}

// TransactWrite executes a function with a transaction builder
func (m *MockExtendedDB) TransactWrite(ctx context.Context, fn func(core.TransactionBuilder) error) error {
	args := m.Called(ctx, fn)
	return args.Error(0)
}

// NewMockExtendedDB creates a new MockExtendedDB with sensible defaults
// for methods that are rarely used in unit tests. This reduces boilerplate
// in tests that only need to mock core functionality.
func NewMockExtendedDB() *MockExtendedDB {
	mockDB := &MockExtendedDB{}

	// Set up default expectations for schema operations
	// These are rarely used in unit tests, so we provide sensible defaults
	mockDB.On("AutoMigrateWithOptions", mock.Anything, mock.Anything).
		Return(nil).Maybe()
	mockDB.On("CreateTable", mock.Anything, mock.Anything).
		Return(nil).Maybe()
	mockDB.On("EnsureTable", mock.Anything).
		Return(nil).Maybe()
	mockDB.On("DeleteTable", mock.Anything).
		Return(nil).Maybe()
	mockDB.On("DescribeTable", mock.Anything).
		Return(nil, nil).Maybe()
	mockDB.On("RegisterTypeConverter", mock.Anything, mock.Anything).
		Return(nil).Maybe()

	// Lambda-specific methods typically return self for chaining
	mockDB.On("WithLambdaTimeout", mock.Anything).
		Return(mockDB).Maybe()
	mockDB.On("WithLambdaTimeoutBuffer", mock.Anything).
		Return(mockDB).Maybe()

	// TransactionFunc default
	mockDB.On("TransactionFunc", mock.AnythingOfType("func(interface {}) error")).
		Return(nil).Maybe()
	mockDB.On("Transact").Return(nil).Maybe()
	mockDB.On("TransactWrite", mock.Anything, mock.Anything).
		Return(nil).Maybe()

	// Set up common base DB method defaults
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()

	return mockDB
}

// NewMockExtendedDBStrict creates a MockExtendedDB without any default
// expectations. Use this when you want to explicitly set all expectations.
func NewMockExtendedDBStrict() *MockExtendedDB {
	return &MockExtendedDB{}
}
