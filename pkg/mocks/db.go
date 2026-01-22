package mocks

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/theory-cloud/tabletheory/pkg/core"
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
	return mustCoreQuery(args.Get(0))
}

// Transaction executes a function within a database transaction
func (m *MockDB) Transaction(fn func(tx *core.Tx) error) error {
	if fn == nil {
		args := m.Called(fn)
		return args.Error(0)
	}

	var (
		callbackInvoked bool
		callbackErr     error
	)

	wrapped := func(tx *core.Tx) error {
		callbackInvoked = true
		callbackErr = fn(tx)
		return callbackErr
	}

	args := m.Called(wrapped)

	if err := args.Error(0); err != nil {
		return err
	}

	if !callbackInvoked {
		tx := &core.Tx{}
		tx.SetDB(m)
		if err := wrapped(tx); err != nil {
			return err
		}
	}

	return callbackErr
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
	return mustCoreDB(args.Get(0))
}
