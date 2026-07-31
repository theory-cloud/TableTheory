package mocks

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/theory-cloud/tabletheory/v3/pkg/core"
)

// MockDB is a mock implementation of the core.DB interface.
// It can be used for unit testing code that depends on TableTheory.
//
// Example usage:
//
//	mockDB := new(mocks.MockDB)
//	mockQuery := new(mocks.MockQuery)
//	mockDB.On("Model", &User{}).Return(mockQuery)
//	mockQuery.On("Where", "ID", "=", "123").Return(mockQuery)
//	mockQuery.On("First", mock.Anything).Return(nil)
type MockDB struct {
	mock.Mock
}

// Model returns a new query builder for the given model
func (m *MockDB) Model(model any) core.Query {
	args := m.Called(model)
	return mockCoreQuery(&m.Mock, "Model", args.Get(0))
}

// Migrate runs all pending migrations
func (m *MockDB) Migrate() error {
	args := m.Called()
	return args.Error(0)
}

// AutoMigrate creates or updates tables based on the given models
func (m *MockDB) AutoMigrate(models ...any) error {
	args := m.Called(models)
	return args.Error(0)
}

// Close closes the database connection
func (m *MockDB) Close() error {
	args := m.Called()
	return args.Error(0)
}

// WithContext returns a new DB instance with the given context
func (m *MockDB) WithContext(ctx context.Context) core.DB {
	args := m.Called(ctx)
	return mockCoreDB(&m.Mock, "WithContext", args.Get(0))
}

// AssertExpectations reports return type mismatches recorded by MockDB in
// addition to testify/mock expectation failures.
func (m *MockDB) AssertExpectations(t mock.TestingT) bool {
	return assertMockExpectations(t, &m.Mock)
}
