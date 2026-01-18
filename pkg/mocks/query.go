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
	if v == nil {
		return nil
	}
	q, ok := v.(core.Query)
	if !ok {
		panic("unexpected type: expected core.Query")
	}
	return q
}

func mustCoreDB(v any) core.DB {
	if v == nil {
		return nil
	}
	db, ok := v.(core.DB)
	if !ok {
		panic("unexpected type: expected core.DB")
	}
	return db
}

func mustPaginatedResult(v any) *core.PaginatedResult {
	if v == nil {
		return nil
	}
	result, ok := v.(*core.PaginatedResult)
	if !ok {
		panic("unexpected type: expected *core.PaginatedResult")
	}
	return result
}

func mustInt64(v any) int64 {
	n, ok := v.(int64)
	if !ok {
		panic("unexpected type: expected int64")
	}
	return n
}

func mustUpdateBuilder(v any) core.UpdateBuilder {
	if v == nil {
		return nil
	}
	builder, ok := v.(core.UpdateBuilder)
	if !ok {
		panic("unexpected type: expected core.UpdateBuilder")
	}
	return builder
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
	return mustCoreQuery(args.Get(0))
}

// Index specifies which index to use
func (m *MockQuery) Index(indexName string) core.Query {
	args := m.Called(indexName)
	return mustCoreQuery(args.Get(0))
}

// Filter adds a filter expression to the query
func (m *MockQuery) Filter(field string, op string, value any) core.Query {
	args := m.Called(field, op, value)
	return mustCoreQuery(args.Get(0))
}

// OrFilter adds an OR filter expression to the query
func (m *MockQuery) OrFilter(field string, op string, value any) core.Query {
	args := m.Called(field, op, value)
	return mustCoreQuery(args.Get(0))
}

// FilterGroup adds a group of filters with AND logic
func (m *MockQuery) FilterGroup(fn func(core.Query)) core.Query {
	args := m.Called(fn)
	return mustCoreQuery(args.Get(0))
}

// OrFilterGroup adds a group of filters with OR logic
func (m *MockQuery) OrFilterGroup(fn func(core.Query)) core.Query {
	args := m.Called(fn)
	return mustCoreQuery(args.Get(0))
}

// IfNotExists adds a condition that the item must not exist
func (m *MockQuery) IfNotExists() core.Query {
	args := m.Called()
	return mustCoreQuery(args.Get(0))
}

// IfExists adds a condition that the item must exist
func (m *MockQuery) IfExists() core.Query {
	args := m.Called()
	return mustCoreQuery(args.Get(0))
}

// WithCondition adds a generic condition expression
func (m *MockQuery) WithCondition(field, operator string, value any) core.Query {
	args := m.Called(field, operator, value)
	return mustCoreQuery(args.Get(0))
}

// WithConditionExpression adds a raw condition expression
func (m *MockQuery) WithConditionExpression(expr string, values map[string]any) core.Query {
	args := m.Called(expr, values)
	return mustCoreQuery(args.Get(0))
}

// OrderBy sets the sort order
func (m *MockQuery) OrderBy(field string, order string) core.Query {
	args := m.Called(field, order)
	return mustCoreQuery(args.Get(0))
}

// Limit sets the maximum number of items to return
func (m *MockQuery) Limit(limit int) core.Query {
	args := m.Called(limit)
	return mustCoreQuery(args.Get(0))
}

// Offset sets the starting position for the query
func (m *MockQuery) Offset(offset int) core.Query {
	args := m.Called(offset)
	return mustCoreQuery(args.Get(0))
}

// Select specifies which fields to retrieve
func (m *MockQuery) Select(fields ...string) core.Query {
	args := m.Called(fields)
	return mustCoreQuery(args.Get(0))
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
	return mustPaginatedResult(args.Get(0)), args.Error(1)
}

// Count returns the number of matching items
func (m *MockQuery) Count() (int64, error) {
	args := m.Called()
	return mustInt64(args.Get(0)), args.Error(1)
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
	return mustUpdateBuilder(args.Get(0))
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
	return mustCoreQuery(args.Get(0))
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
	if builder, ok := args.Get(0).(core.BatchGetBuilder); ok {
		return builder
	}
	return nil
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
	return mustCoreQuery(args.Get(0))
}

// SetCursor sets the cursor from a string
func (m *MockQuery) SetCursor(cursor string) error {
	args := m.Called(cursor)
	return args.Error(0)
}

// WithContext sets the context for the query
func (m *MockQuery) WithContext(ctx context.Context) core.Query {
	args := m.Called(ctx)
	return mustCoreQuery(args.Get(0))
}

// ConsistentRead enables strongly consistent reads for Query operations
func (m *MockQuery) ConsistentRead() core.Query {
	args := m.Called()
	return mustCoreQuery(args.Get(0))
}

// WithRetry configures retry behavior for eventually consistent reads
func (m *MockQuery) WithRetry(maxRetries int, initialDelay time.Duration) core.Query {
	args := m.Called(maxRetries, initialDelay)
	return mustCoreQuery(args.Get(0))
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
