package mocks

import (
	"github.com/stretchr/testify/mock"

	"github.com/theory-cloud/tabletheory/v2/pkg/core"
)

// MockBatchGetBuilder is a mock implementation of core.BatchGetBuilder.
type MockBatchGetBuilder struct {
	mock.Mock
}

// Keys sets the keys to retrieve.
func (m *MockBatchGetBuilder) Keys(keys []any) core.BatchGetBuilder {
	args := m.Called(keys)
	return mockBatchGetBuilder(&m.Mock, "Keys", args.Get(0))
}

// ChunkSize configures the chunk size.
func (m *MockBatchGetBuilder) ChunkSize(size int) core.BatchGetBuilder {
	args := m.Called(size)
	return mockBatchGetBuilder(&m.Mock, "ChunkSize", args.Get(0))
}

// ConsistentRead enables strongly consistent reads.
func (m *MockBatchGetBuilder) ConsistentRead() core.BatchGetBuilder {
	args := m.Called()
	return mockBatchGetBuilder(&m.Mock, "ConsistentRead", args.Get(0))
}

// Parallel configures concurrency.
func (m *MockBatchGetBuilder) Parallel(maxConcurrency int) core.BatchGetBuilder {
	args := m.Called(maxConcurrency)
	return mockBatchGetBuilder(&m.Mock, "Parallel", args.Get(0))
}

// WithRetry overrides the retry policy.
func (m *MockBatchGetBuilder) WithRetry(policy *core.RetryPolicy) core.BatchGetBuilder {
	args := m.Called(policy)
	return mockBatchGetBuilder(&m.Mock, "WithRetry", args.Get(0))
}

// Select limits the projection.
func (m *MockBatchGetBuilder) Select(fields ...string) core.BatchGetBuilder {
	args := m.Called(fields)
	return mockBatchGetBuilder(&m.Mock, "Select", args.Get(0))
}

// OnProgress registers a callback for progress updates.
func (m *MockBatchGetBuilder) OnProgress(callback core.BatchProgressCallback) core.BatchGetBuilder {
	args := m.Called(callback)
	return mockBatchGetBuilder(&m.Mock, "OnProgress", args.Get(0))
}

// OnError registers an error handler.
func (m *MockBatchGetBuilder) OnError(handler core.BatchChunkErrorHandler) core.BatchGetBuilder {
	args := m.Called(handler)
	return mockBatchGetBuilder(&m.Mock, "OnError", args.Get(0))
}

// Execute performs the batch get operation.
func (m *MockBatchGetBuilder) Execute(dest any) error {
	args := m.Called(dest)
	return args.Error(0)
}

// AssertExpectations reports return type mismatches recorded by
// MockBatchGetBuilder in addition to testify/mock expectation failures.
func (m *MockBatchGetBuilder) AssertExpectations(t mock.TestingT) bool {
	return assertMockExpectations(t, &m.Mock)
}
