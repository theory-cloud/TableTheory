// Package mocks provides mock implementations for TableTheory interfaces.
// These mocks are designed to be used with github.com/stretchr/testify/mock
// for unit testing applications that use TableTheory.
package mocks

import (
	"context"
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/theory-cloud/tabletheory/pkg/core"
)

func mustCoreQuery(v any) core.Query {
	return mockCoreQuery(nil, "", v)
}

func mockCoreQuery(m *mock.Mock, method string, v any) core.Query {
	if v == nil {
		return nil
	}
	if q, ok := typedReturn[core.Query](m, method, 0, "core.Query", v); ok {
		return q
	}
	return nil
}

func mustCoreDB(v any) core.DB {
	return mockCoreDB(nil, "", v)
}

func mockCoreDB(m *mock.Mock, method string, v any) core.DB {
	if v == nil {
		return nil
	}
	if db, ok := typedReturn[core.DB](m, method, 0, "core.DB", v); ok {
		return db
	}
	return nil
}

func mustPaginatedResult(v any) *core.PaginatedResult {
	return mockPaginatedResult(nil, "", v)
}

func mockPaginatedResult(m *mock.Mock, method string, v any) *core.PaginatedResult {
	if v == nil {
		return nil
	}
	if result, ok := typedReturn[*core.PaginatedResult](m, method, 0, "*core.PaginatedResult", v); ok {
		return result
	}
	return nil
}

func mustInt64(v any) int64 {
	return mockInt64(nil, "", v)
}

func mockInt64(m *mock.Mock, method string, v any) int64 {
	if n, ok := typedReturn[int64](m, method, 0, "int64", v); ok {
		return n
	}
	return 0
}

func mustUpdateBuilder(v any) core.UpdateBuilder {
	return mockUpdateBuilder(nil, "", v)
}

func mockUpdateBuilder(m *mock.Mock, method string, v any) core.UpdateBuilder {
	if v == nil {
		return nil
	}
	if builder, ok := typedReturn[core.UpdateBuilder](m, method, 0, "core.UpdateBuilder", v); ok {
		return builder
	}
	return nil
}

func mockTransactionBuilder(m *mock.Mock, method string, v any) core.TransactionBuilder {
	if v == nil {
		return nil
	}
	if builder, ok := typedReturn[core.TransactionBuilder](m, method, 0, "core.TransactionBuilder", v); ok {
		return builder
	}
	return nil
}

func mockBatchGetBuilder(m *mock.Mock, method string, v any) core.BatchGetBuilder {
	if v == nil {
		return nil
	}
	if builder, ok := typedReturn[core.BatchGetBuilder](m, method, 0, "core.BatchGetBuilder", v); ok {
		return builder
	}
	return nil
}

// AssertExpectations reports return type mismatches recorded by MockQuery in
// addition to testify/mock expectation failures.
func (m *MockQuery) AssertExpectations(t mock.TestingT) bool {
	return assertMockExpectations(t, &m.Mock)
}

// MockQuery is a mock implementation of the core.Query interface.
// It can be used for unit testing code that depends on TableTheory queries.
//
// Example usage:
//
//	mockQuery := new(mocks.MockQuery)
//	mockQuery.On("Where", "ID", "=", "123").Return(mockQuery)
//	mockQuery.On("First", mock.Anything).Return(nil)
type MockQuery struct {
	mock.Mock
}

// Where adds a condition to the query
func (m *MockQuery) Where(field string, op string, value any) core.Query {
	args := m.Called(field, op, value)
	return mockCoreQuery(&m.Mock, "Where", args.Get(0))
}

// Index specifies which index to use
func (m *MockQuery) Index(indexName string) core.Query {
	args := m.Called(indexName)
	return mockCoreQuery(&m.Mock, "Index", args.Get(0))
}

// Filter adds a filter expression to the query
func (m *MockQuery) Filter(field string, op string, value any) core.Query {
	args := m.Called(field, op, value)
	return mockCoreQuery(&m.Mock, "Filter", args.Get(0))
}

// OrFilter adds an OR filter expression to the query
func (m *MockQuery) OrFilter(field string, op string, value any) core.Query {
	args := m.Called(field, op, value)
	return mockCoreQuery(&m.Mock, "OrFilter", args.Get(0))
}

// FilterGroup adds a group of filters with AND logic
func (m *MockQuery) FilterGroup(fn func(core.Query)) core.Query {
	args := m.Called(fn)
	return mockCoreQuery(&m.Mock, "FilterGroup", args.Get(0))
}

// OrFilterGroup adds a group of filters with OR logic
func (m *MockQuery) OrFilterGroup(fn func(core.Query)) core.Query {
	args := m.Called(fn)
	return mockCoreQuery(&m.Mock, "OrFilterGroup", args.Get(0))
}

// IfNotExists adds a condition that the item must not exist
func (m *MockQuery) IfNotExists() core.Query {
	args := m.Called()
	return mockCoreQuery(&m.Mock, "IfNotExists", args.Get(0))
}

// IfExists adds a condition that the item must exist
func (m *MockQuery) IfExists() core.Query {
	args := m.Called()
	return mockCoreQuery(&m.Mock, "IfExists", args.Get(0))
}

// WithCondition adds a generic condition expression
func (m *MockQuery) WithCondition(field, operator string, value any) core.Query {
	args := m.Called(field, operator, value)
	return mockCoreQuery(&m.Mock, "WithCondition", args.Get(0))
}

// WithConditionExpression adds a raw condition expression
func (m *MockQuery) WithConditionExpression(expr string, values map[string]any) core.Query {
	args := m.Called(expr, values)
	return mockCoreQuery(&m.Mock, "WithConditionExpression", args.Get(0))
}

// OrderBy sets the sort order
func (m *MockQuery) OrderBy(field string, order string) core.Query {
	args := m.Called(field, order)
	return mockCoreQuery(&m.Mock, "OrderBy", args.Get(0))
}

// Limit sets the maximum number of items to return
func (m *MockQuery) Limit(limit int) core.Query {
	args := m.Called(limit)
	return mockCoreQuery(&m.Mock, "Limit", args.Get(0))
}

// Offset sets the starting position for the query
func (m *MockQuery) Offset(offset int) core.Query {
	args := m.Called(offset)
	return mockCoreQuery(&m.Mock, "Offset", args.Get(0))
}

// Select specifies which fields to retrieve
func (m *MockQuery) Select(fields ...string) core.Query {
	args := m.Called(fields)
	return mockCoreQuery(&m.Mock, "Select", args.Get(0))
}

// First retrieves the first matching item
func (m *MockQuery) First(dest any) error {
	args := m.Called(dest)
	return args.Error(0)
}

// All retrieves all matching items
func (m *MockQuery) All(dest any) error {
	args := m.Called(dest)
	return args.Error(0)
}

// AllPaginated retrieves all matching items with pagination metadata
func (m *MockQuery) AllPaginated(dest any) (*core.PaginatedResult, error) {
	args := m.Called(dest)
	return mockPaginatedResult(&m.Mock, "AllPaginated", args.Get(0)), args.Error(1)
}

// Count returns the number of matching items
func (m *MockQuery) Count() (int64, error) {
	args := m.Called()
	return mockInt64(&m.Mock, "Count", args.Get(0)), args.Error(1)
}

// Create creates a new item
func (m *MockQuery) Create() error {
	args := m.Called()
	return args.Error(0)
}

// CreateOrUpdate creates a new item or updates an existing one (upsert)
func (m *MockQuery) CreateOrUpdate() error {
	args := m.Called()
	return args.Error(0)
}

// Update updates the matching items
func (m *MockQuery) Update(fields ...string) error {
	args := m.Called(fields)
	return args.Error(0)
}

// UpdateBuilder returns a builder for complex update operations
func (m *MockQuery) UpdateBuilder() core.UpdateBuilder {
	args := m.Called()
	return mockUpdateBuilder(&m.Mock, "UpdateBuilder", args.Get(0))
}

// Delete deletes the matching items
func (m *MockQuery) Delete() error {
	args := m.Called()
	return args.Error(0)
}

// Scan performs a table scan
func (m *MockQuery) Scan(dest any) error {
	args := m.Called(dest)
	return args.Error(0)
}

// ParallelScan configures parallel scanning
func (m *MockQuery) ParallelScan(segment int32, totalSegments int32) core.Query {
	args := m.Called(segment, totalSegments)
	return mockCoreQuery(&m.Mock, "ParallelScan", args.Get(0))
}

// ScanAllSegments performs parallel scan across all segments
func (m *MockQuery) ScanAllSegments(dest any, totalSegments int32) error {
	args := m.Called(dest, totalSegments)
	return args.Error(0)
}

// BatchGet retrieves multiple items by their primary keys
func (m *MockQuery) BatchGet(keys []any, dest any) error {
	args := m.Called(keys, dest)
	return args.Error(0)
}

// BatchGetWithOptions retrieves multiple items with custom options
func (m *MockQuery) BatchGetWithOptions(keys []any, dest any, opts *core.BatchGetOptions) error {
	args := m.Called(keys, dest, opts)
	return args.Error(0)
}

// BatchGetBuilder returns a fluent builder for BatchGet
func (m *MockQuery) BatchGetBuilder() core.BatchGetBuilder {
	args := m.Called()
	return mockBatchGetBuilder(&m.Mock, "BatchGetBuilder", args.Get(0))
}

// BatchCreate creates multiple items
func (m *MockQuery) BatchCreate(items any) error {
	args := m.Called(items)
	return args.Error(0)
}

// BatchDelete deletes multiple items by their primary keys
func (m *MockQuery) BatchDelete(keys []any) error {
	args := m.Called(keys)
	return args.Error(0)
}

// Cursor sets the pagination cursor
func (m *MockQuery) Cursor(cursor string) core.Query {
	args := m.Called(cursor)
	return mockCoreQuery(&m.Mock, "Cursor", args.Get(0))
}

// SetCursor sets the cursor from a string
func (m *MockQuery) SetCursor(cursor string) error {
	args := m.Called(cursor)
	return args.Error(0)
}

// WithContext sets the context for the query
func (m *MockQuery) WithContext(ctx context.Context) core.Query {
	args := m.Called(ctx)
	return mockCoreQuery(&m.Mock, "WithContext", args.Get(0))
}

// ConsistentRead enables strongly consistent reads for Query operations
func (m *MockQuery) ConsistentRead() core.Query {
	args := m.Called()
	return mockCoreQuery(&m.Mock, "ConsistentRead", args.Get(0))
}

// WithRetry configures retry behavior for eventually consistent reads
func (m *MockQuery) WithRetry(maxRetries int, initialDelay time.Duration) core.Query {
	args := m.Called(maxRetries, initialDelay)
	return mockCoreQuery(&m.Mock, "WithRetry", args.Get(0))
}

// BatchWrite performs mixed batch write operations
func (m *MockQuery) BatchWrite(putItems []any, deleteKeys []any) error {
	args := m.Called(putItems, deleteKeys)
	return args.Error(0)
}

// BatchUpdateWithOptions performs batch update operations with custom options
func (m *MockQuery) BatchUpdateWithOptions(items []any, fields []string, options ...any) error {
	args := m.Called(items, fields, options)
	return args.Error(0)
}
