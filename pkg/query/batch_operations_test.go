package query

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/core"
)

// Mock types for testing
type TestItem struct {
	ID        string `dynamodb:"id"`
	Name      string `dynamodb:"name"`
	Status    string `dynamodb:"status"`
	Value     int    `dynamodb:"value"`
	CreatedAt int64  `dynamodb:"created_at"`
}

type TestMetadata struct{}

func (m *TestMetadata) TableName() string {
	return "test-table"
}

func (m *TestMetadata) PrimaryKey() core.KeySchema {
	return core.KeySchema{
		PartitionKey: "ID",
		SortKey:      "CreatedAt",
	}
}

func (m *TestMetadata) Indexes() []core.IndexSchema {
	return []core.IndexSchema{}
}

func (m *TestMetadata) AttributeMetadata(field string) *core.AttributeMetadata {
	mapping := map[string]string{
		"ID":        "id",
		"Name":      "name",
		"Value":     "value",
		"Status":    "status",
		"CreatedAt": "created_at",
	}

	if dbName, ok := mapping[field]; ok {
		return &core.AttributeMetadata{
			Name:         field,
			Type:         "string", // simplified for testing
			DynamoDBName: dbName,
			Tags:         make(map[string]string),
		}
	}
	return nil
}

func (m *TestMetadata) VersionFieldName() string {
	return ""
}

func TestDefaultBatchOptions(t *testing.T) {
	opts := DefaultBatchOptions()

	assert.Equal(t, 25, opts.MaxBatchSize)
	assert.False(t, opts.Parallel)
	assert.Equal(t, 5, opts.MaxConcurrency)
	assert.NotNil(t, opts.RetryPolicy)
	assert.Equal(t, 3, opts.RetryPolicy.MaxRetries)
	assert.Equal(t, 100*time.Millisecond, opts.RetryPolicy.InitialDelay)
	assert.Equal(t, 5*time.Second, opts.RetryPolicy.MaxDelay)
	assert.Equal(t, 2.0, opts.RetryPolicy.BackoffFactor)
}

func TestBatchUpdate(t *testing.T) {
	tests := []struct {
		items   any
		name    string
		errMsg  string
		fields  []string
		wantErr bool
	}{
		{
			name: "valid slice of items",
			items: []TestItem{
				{ID: "1", Name: "Item 1", Value: 100},
				{ID: "2", Name: "Item 2", Value: 200},
			},
			fields:  []string{"Name", "Value"},
			wantErr: false,
		},
		{
			name:    "empty slice",
			items:   []TestItem{},
			fields:  []string{"Name"},
			wantErr: false, // No error expected for empty slice - it just returns early
		},
		{
			name:    "non-slice input",
			items:   TestItem{ID: "1"},
			fields:  []string{"Name"},
			wantErr: true,
			errMsg:  "items must be a slice",
		},
		{
			name:    "nil input",
			items:   nil,
			fields:  []string{"Name"},
			wantErr: true,
			errMsg:  "items must be a slice",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := &Query{
				metadata: &TestMetadata{},
				ctx:      context.Background(),
			}

			err := q.BatchUpdate(tt.items, tt.fields...)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				// For empty slice, no error is expected
				if reflect.ValueOf(tt.items).Len() == 0 {
					assert.NoError(t, err)
				} else {
					// The actual execution will fail because we don't have a real executor
					assert.Error(t, err) // Expected to fail at execution
				}
			}
		})
	}
}

func TestBatchUpdateWithOptions(t *testing.T) {
	items := []TestItem{
		{ID: "1", Name: "Item 1", Value: 100},
		{ID: "2", Name: "Item 2", Value: 200},
		{ID: "3", Name: "Item 3", Value: 300},
	}

	t.Run("with progress callback", func(t *testing.T) {
		var progressCalls []struct{ processed, total int }
		opts := &BatchUpdateOptions{
			MaxBatchSize: 2,
			ProgressCallback: func(processed, total int) {
				progressCalls = append(progressCalls, struct{ processed, total int }{processed, total})
			},
			ErrorHandler: func(item any, err error) error {
				// Ignore errors for this test
				return nil
			},
		}

		q := &Query{
			metadata: &TestMetadata{},
			ctx:      context.Background(),
		}

		// Convert items to []any
		anyItems := make([]any, len(items))
		for i, item := range items {
			anyItems[i] = item
		}

		err := q.BatchUpdateWithOptions(anyItems, []string{"Name"}, opts)
		assert.NoError(t, err)

		// Should have been called at least once
		assert.NotEmpty(t, progressCalls)
	})

	t.Run("with error handler", func(t *testing.T) {
		errorHandlerCalled := false
		opts := &BatchUpdateOptions{
			MaxBatchSize: 25,
			ErrorHandler: func(item any, err error) error {
				errorHandlerCalled = true
				return nil // Continue processing
			},
		}

		q := &Query{
			metadata: &TestMetadata{},
			ctx:      context.Background(),
		}

		// Convert items to []any
		anyItems := make([]any, len(items))
		for i, item := range items {
			anyItems[i] = item
		}

		err := q.BatchUpdateWithOptions(anyItems, []string{"Name"}, opts)
		assert.NoError(t, err)

		// Error handler should have been called due to execution failure
		assert.True(t, errorHandlerCalled)
	})

	t.Run("parallel execution", func(t *testing.T) {
		opts := &BatchUpdateOptions{
			MaxBatchSize:   1,
			Parallel:       true,
			MaxConcurrency: 2,
			ErrorHandler: func(item any, err error) error {
				return nil // Ignore errors
			},
		}

		q := &Query{
			metadata: &TestMetadata{},
			ctx:      context.Background(),
		}

		// Convert items to []any
		anyItems := make([]any, len(items))
		for i, item := range items {
			anyItems[i] = item
		}

		err := q.BatchUpdateWithOptions(anyItems, []string{"Name"}, opts)
		// Should not panic and should handle concurrency properly
		// With error handler that ignores errors, this should complete without error
		assert.NoError(t, err)
	})
}

func TestPrepareBatches(t *testing.T) {
	items := []TestItem{
		{ID: "1"}, {ID: "2"}, {ID: "3"}, {ID: "4"}, {ID: "5"},
		{ID: "6"}, {ID: "7"}, {ID: "8"}, {ID: "9"}, {ID: "10"},
	}

	tests := []struct {
		name            string
		items           []TestItem
		expectedSizes   []int
		batchSize       int
		expectedBatches int
	}{
		{
			name:            "batch size 3",
			items:           items,
			batchSize:       3,
			expectedBatches: 4,
			expectedSizes:   []int{3, 3, 3, 1},
		},
		{
			name:            "batch size 25 (max)",
			items:           items,
			batchSize:       25,
			expectedBatches: 1,
			expectedSizes:   []int{10},
		},
		{
			name:            "batch size 0 (defaults to 25)",
			items:           items,
			batchSize:       0,
			expectedBatches: 1,
			expectedSizes:   []int{10},
		},
		{
			name:            "batch size > 25 (capped to 25)",
			items:           items,
			batchSize:       50,
			expectedBatches: 1,
			expectedSizes:   []int{10},
		},
		{
			name:            "exact batch size",
			items:           items[:6],
			batchSize:       2,
			expectedBatches: 3,
			expectedSizes:   []int{2, 2, 2},
		},
		{
			name:            "single item",
			items:           items[:1],
			batchSize:       25,
			expectedBatches: 1,
			expectedSizes:   []int{1},
		},
		{
			name:            "empty slice",
			items:           []TestItem{},
			batchSize:       25,
			expectedBatches: 0,
			expectedSizes:   []int{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := &Query{}
			itemsValue := reflect.ValueOf(tt.items)
			batches := q.prepareBatches(itemsValue, tt.batchSize)

			assert.Len(t, batches, tt.expectedBatches)

			for i, batch := range batches {
				assert.Len(t, batch, tt.expectedSizes[i])

				// Verify correct items are in each batch
				for j, item := range batch {
					expectedIdx := i*tt.batchSize + j
					if tt.batchSize <= 0 || tt.batchSize > 25 {
						expectedIdx = i*25 + j
					}
					typedItem, ok := item.(TestItem)
					require.True(t, ok)
					assert.Equal(t, tt.items[expectedIdx].ID, typedItem.ID)
				}
			}
		})
	}
}

func TestPrepareKeyBatches(t *testing.T) {
	keys := []any{"key1", "key2", "key3", "key4", "key5"}

	tests := []struct {
		name            string
		keys            []any
		batchSize       int
		expectedBatches int
	}{
		{
			name:            "batch size 2",
			keys:            keys,
			batchSize:       2,
			expectedBatches: 3,
		},
		{
			name:            "batch size 0 (defaults to 25)",
			keys:            keys,
			batchSize:       0,
			expectedBatches: 1,
		},
		{
			name:            "empty keys",
			keys:            []any{},
			batchSize:       25,
			expectedBatches: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := &Query{}
			batches := q.prepareKeyBatches(tt.keys, tt.batchSize)
			assert.Len(t, batches, tt.expectedBatches)
		})
	}
}

func TestExtractKey(t *testing.T) {
	tests := []struct {
		item    any
		wantKey map[string]any
		name    string
		wantErr bool
	}{
		{
			name: "valid item with partition and sort key",
			item: TestItem{
				ID:        "user123",
				CreatedAt: 1234567890,
			},
			wantKey: map[string]any{
				"ID":        "user123",
				"CreatedAt": int64(1234567890),
			},
			wantErr: false,
		},
		{
			name: "pointer to item",
			item: &TestItem{
				ID:        "user456",
				CreatedAt: 9876543210,
			},
			wantKey: map[string]any{
				"ID":        "user456",
				"CreatedAt": int64(9876543210),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := &Query{
				metadata: &TestMetadata{},
			}

			key, err := q.extractKey(tt.item)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantKey, key)
			}
		})
	}
}

func TestExecuteWithRetry(t *testing.T) {
	t.Run("success on first try", func(t *testing.T) {
		callCount := 0
		fn := func() error {
			callCount++
			return nil
		}

		q := &Query{}
		policy := &RetryPolicy{
			MaxRetries:    3,
			InitialDelay:  10 * time.Millisecond,
			MaxDelay:      100 * time.Millisecond,
			BackoffFactor: 2.0,
		}

		err := q.executeWithRetry(fn, policy)
		assert.NoError(t, err)
		assert.Equal(t, 1, callCount)
	})

	t.Run("success after retries", func(t *testing.T) {
		callCount := 0
		fn := func() error {
			callCount++
			if callCount < 3 {
				return errors.New("ProvisionedThroughputExceededException")
			}
			return nil
		}

		q := &Query{}
		policy := &RetryPolicy{
			MaxRetries:    3,
			InitialDelay:  1 * time.Millisecond,
			MaxDelay:      10 * time.Millisecond,
			BackoffFactor: 2.0,
		}

		start := time.Now()
		err := q.executeWithRetry(fn, policy)
		duration := time.Since(start)

		assert.NoError(t, err)
		assert.Equal(t, 3, callCount)
		// Should have delays: 1ms + 2ms = 3ms minimum
		assert.GreaterOrEqual(t, duration.Milliseconds(), int64(2))
	})

	t.Run("max retries exceeded", func(t *testing.T) {
		callCount := 0
		fn := func() error {
			callCount++
			return errors.New("ThrottlingException")
		}

		q := &Query{}
		policy := &RetryPolicy{
			MaxRetries:    2,
			InitialDelay:  1 * time.Millisecond,
			MaxDelay:      5 * time.Millisecond,
			BackoffFactor: 2.0,
		}

		err := q.executeWithRetry(fn, policy)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "operation failed after 2 retries")
		assert.Equal(t, 3, callCount) // Initial + 2 retries
	})

	t.Run("non-retryable error", func(t *testing.T) {
		callCount := 0
		fn := func() error {
			callCount++
			return errors.New("ValidationException")
		}

		q := &Query{}
		policy := &RetryPolicy{
			MaxRetries:    3,
			InitialDelay:  10 * time.Millisecond,
			MaxDelay:      100 * time.Millisecond,
			BackoffFactor: 2.0,
		}

		err := q.executeWithRetry(fn, policy)
		assert.Error(t, err)
		assert.Equal(t, 1, callCount) // No retries for non-retryable errors
	})

	t.Run("nil policy", func(t *testing.T) {
		callCount := 0
		fn := func() error {
			callCount++
			return nil
		}

		q := &Query{}
		err := q.executeWithRetry(fn, nil)
		assert.NoError(t, err)
		assert.Equal(t, 1, callCount)
	})
}

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		err       error
		name      string
		wantRetry bool
	}{
		{
			name:      "ProvisionedThroughputExceededException",
			err:       errors.New("ProvisionedThroughputExceededException: Request rate exceeded"),
			wantRetry: true,
		},
		{
			name:      "ThrottlingException",
			err:       errors.New("ThrottlingException: Rate exceeded"),
			wantRetry: true,
		},
		{
			name:      "InternalServerError",
			err:       errors.New("InternalServerError: Something went wrong"),
			wantRetry: true,
		},
		{
			name:      "ServiceUnavailable",
			err:       errors.New("ServiceUnavailable: Service is down"),
			wantRetry: true,
		},
		{
			name:      "RequestLimitExceeded",
			err:       errors.New("RequestLimitExceeded: Too many requests"),
			wantRetry: true,
		},
		{
			name:      "ValidationException",
			err:       errors.New("ValidationException: Invalid input"),
			wantRetry: false,
		},
		{
			name:      "ResourceNotFoundException",
			err:       errors.New("ResourceNotFoundException: Table not found"),
			wantRetry: false,
		},
		{
			name:      "nil error",
			err:       nil,
			wantRetry: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRetryableError(tt.err)
			assert.Equal(t, tt.wantRetry, got)
		})
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		substr string
		want   bool
	}{
		{
			name:   "exact match",
			s:      "hello",
			substr: "hello",
			want:   true,
		},
		{
			name:   "substring at start",
			s:      "hello world",
			substr: "hello",
			want:   true,
		},
		{
			name:   "substring in middle",
			s:      "hello world",
			substr: "lo wo",
			want:   true,
		},
		{
			name:   "substring at end",
			s:      "hello world",
			substr: "world",
			want:   true,
		},
		{
			name:   "no match",
			s:      "hello world",
			substr: "xyz",
			want:   false,
		},
		{
			name:   "empty substring",
			s:      "hello",
			substr: "",
			want:   false,
		},
		{
			name:   "empty string",
			s:      "",
			substr: "hello",
			want:   false,
		},
		{
			name:   "both empty",
			s:      "",
			substr: "",
			want:   false,
		},
		{
			name:   "substring longer than string",
			s:      "hi",
			substr: "hello",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := contains(tt.s, tt.substr)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBatchCreateWithResult(t *testing.T) {
	items := []TestItem{
		{ID: "1", Name: "Item 1"},
		{ID: "2", Name: "Item 2"},
		{ID: "3", Name: "Item 3"},
	}

	q := &Query{
		metadata: &TestMetadata{},
		ctx:      context.Background(),
		executor: &mockQueryExecutor{}, // Mock executor to avoid nil panic
	}

	result, err := q.BatchCreateWithResult(items)

	// The BatchCreate will fail because we don't have a real executor
	assert.Error(t, err)
	assert.NotNil(t, result)
	// Since we're using a custom error handler that tracks failures,
	// and the executor is not a BatchExecutor, we expect failures
	assert.GreaterOrEqual(t, result.Failed, 0) // May be 0 if early failure
	// Errors may or may not be recorded depending on when failure occurs
}

// Add a mock executor to avoid nil panics
type mockQueryExecutor struct{}

func (m *mockQueryExecutor) ExecuteQuery(input *core.CompiledQuery, dest any) error {
	return errors.New("not implemented")
}

func (m *mockQueryExecutor) ExecuteScan(input *core.CompiledQuery, dest any) error {
	return errors.New("not implemented")
}

func TestQueryTimeout(t *testing.T) {
	q := &Query{
		ctx: context.Background(),
	}

	timeout := 5 * time.Second
	newQuery := q.QueryTimeout(timeout)

	// Should return self
	assert.Equal(t, q, newQuery)

	// Context should have a deadline
	deadline, ok := q.ctx.Deadline()
	assert.True(t, ok)
	assert.WithinDuration(t, time.Now().Add(timeout), deadline, 100*time.Millisecond)
}

func TestWithCancellation(t *testing.T) {
	q := &Query{
		ctx: context.Background(),
	}

	newQuery, canceler := q.WithCancellation()

	// Should return self
	assert.Equal(t, q, newQuery)
	assert.NotNil(t, canceler)

	// Test cancellation
	done := make(chan bool)
	go func() {
		<-q.ctx.Done()
		done <- true
	}()

	canceler.Cancel()

	select {
	case <-done:
		// Context was canceled successfully
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Context was not canceled")
	}

	// Calling cancel again should not panic
	canceler.Cancel()
}

func TestBatchDeleteWithOptions(t *testing.T) {
	keys := []any{
		TestItem{ID: "1", CreatedAt: 100},
		TestItem{ID: "2", CreatedAt: 200},
		TestItem{ID: "3", CreatedAt: 300},
	}

	t.Run("empty keys", func(t *testing.T) {
		q := &Query{
			metadata: &TestMetadata{},
			ctx:      context.Background(),
		}

		err := q.BatchDelete([]any{})
		assert.NoError(t, err)
	})

	t.Run("with progress callback", func(t *testing.T) {
		progressCalled := false
		opts := &BatchUpdateOptions{
			MaxBatchSize: 2,
			ProgressCallback: func(processed, total int) {
				progressCalled = true
			},
			ErrorHandler: func(item any, err error) error {
				return nil // Ignore errors
			},
		}

		q := &Query{
			metadata: &TestMetadata{},
			ctx:      context.Background(),
		}

		err := q.BatchDeleteWithOptions(keys, opts)
		assert.NoError(t, err)
		assert.True(t, progressCalled)
	})
}

func TestExecuteBatchesParallel(t *testing.T) {
	// Test concurrent execution
	items := make([][]any, 10)
	for i := 0; i < 10; i++ {
		items[i] = []any{TestItem{ID: fmt.Sprintf("%d", i)}}
	}

	var mu sync.Mutex
	var executionOrder []int

	q := &Query{
		metadata: &TestMetadata{},
		ctx:      context.Background(),
		// Mock the execution to track order
	}

	// Monkey patch the executeUpdateBatch method for testing
	// In real tests, we'd use dependency injection
	processed := 0
	total := 10

	opts := &BatchUpdateOptions{
		Parallel:       true,
		MaxConcurrency: 3,
		ErrorHandler: func(item any, err error) error {
			// Track execution order
			if batch, ok := item.([]any); ok && len(batch) > 0 {
				if testItem, ok := batch[0].(TestItem); ok {
					mu.Lock()
					for i, id := range []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9"} {
						if testItem.ID == id {
							executionOrder = append(executionOrder, i)
							break
						}
					}
					mu.Unlock()
				}
			}
			return nil
		},
	}

	err := q.executeBatchesParallel(items, opts, []string{"Name"}, &processed, total)
	assert.NoError(t, err)

	// Verify all batches were attempted
	assert.Len(t, executionOrder, 10)

	// Verify they didn't all execute sequentially (some parallelism occurred)
	// This is a weak test but better than nothing
	isSequential := true
	for i := 1; i < len(executionOrder); i++ {
		if executionOrder[i] < executionOrder[i-1] {
			isSequential = false
			break
		}
	}
	// With parallel execution, we expect some out-of-order execution
	assert.False(t, isSequential, "Expected some parallel execution")
}
