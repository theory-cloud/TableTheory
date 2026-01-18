package mocks

import (
	"github.com/stretchr/testify/mock"

	"github.com/theory-cloud/tabletheory/pkg/core"
)

// MockBatchGetBuilder is a mock implementation of core.BatchGetBuilder.
type MockBatchGetBuilder struct {
	mock.Mock
}

// Keys sets the keys to retrieve.
func (m *MockBatchGetBuilder) Keys(keys []any) core.BatchGetBuilder {
	args := m.Called(keys)
	return args.Get(0).(core.BatchGetBuilder)
}

// ChunkSize configures the chunk size.
func (m *MockBatchGetBuilder) ChunkSize(size int) core.BatchGetBuilder {
	args := m.Called(size)
	return args.Get(0).(core.BatchGetBuilder)
}

// ConsistentRead enables strongly consistent reads.
func (m *MockBatchGetBuilder) ConsistentRead() core.BatchGetBuilder {
	args := m.Called()
	return args.Get(0).(core.BatchGetBuilder)
}

// Parallel configures concurrency.
func (m *MockBatchGetBuilder) Parallel(maxConcurrency int) core.BatchGetBuilder {
	args := m.Called(maxConcurrency)
	return args.Get(0).(core.BatchGetBuilder)
}

// WithRetry overrides the retry policy.
func (m *MockBatchGetBuilder) WithRetry(policy *core.RetryPolicy) core.BatchGetBuilder {
	args := m.Called(policy)
	return args.Get(0).(core.BatchGetBuilder)
}

// Select limits the projection.
func (m *MockBatchGetBuilder) Select(fields ...string) core.BatchGetBuilder {
	args := m.Called(fields)
	return args.Get(0).(core.BatchGetBuilder)
}

// OnProgress registers a callback for progress updates.
func (m *MockBatchGetBuilder) OnProgress(callback core.BatchProgressCallback) core.BatchGetBuilder {
	args := m.Called(callback)
	return args.Get(0).(core.BatchGetBuilder)
}

// OnError registers an error handler.
func (m *MockBatchGetBuilder) OnError(handler core.BatchChunkErrorHandler) core.BatchGetBuilder {
	args := m.Called(handler)
	return args.Get(0).(core.BatchGetBuilder)
}

// Execute performs the batch get operation.
func (m *MockBatchGetBuilder) Execute(dest any) error {
	args := m.Called(dest)
	return args.Error(0)
}
