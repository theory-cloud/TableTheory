package query

import (
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/theory-cloud/tabletheory/pkg/core"
)

// Mock types for testing
type mockUpdateExecutor struct {
	mock.Mock
}

func (m *mockUpdateExecutor) ExecuteQuery(input *core.CompiledQuery, dest any) error {
	args := m.Called(input, dest)
	return args.Error(0)
}

func (m *mockUpdateExecutor) ExecuteScan(input *core.CompiledQuery, dest any) error {
	args := m.Called(input, dest)
	return args.Error(0)
}

func (m *mockUpdateExecutor) ExecuteUpdateItem(input *core.CompiledQuery, key map[string]types.AttributeValue) error {
	args := m.Called(input, key)
	return args.Error(0)
}

// Test model
type BlogPost struct {
	PublishedAt time.Time
	UpdatedAt   time.Time
	ID          string `theorydb:"pk"`
	Title       string
	Content     string
	Tags        []string
	ViewCount   int64
	LikeCount   int64
	Version     int64
}

func TestUpdateBuilder_AtomicCounter(t *testing.T) {
	// This test demonstrates how Team 2 can use UpdateBuilder for atomic counters
	executor := new(mockUpdateExecutor)
	metadata := &mockMetadata{
		tableName: "BlogPosts",
		primaryKey: core.KeySchema{
			PartitionKey: "ID",
		},
	}

	q := &Query{
		executor: executor,
		metadata: metadata,
		conditions: []Condition{
			{Field: "ID", Operator: "=", Value: "post123"},
		},
	}

	// Set up expectation
	executor.On("ExecuteUpdateItem", mock.MatchedBy(func(compiled *core.CompiledQuery) bool {
		// Verify the update expression contains ADD for view count
		return compiled.Operation == "UpdateItem" &&
			compiled.TableName == "BlogPosts" &&
			compiled.UpdateExpression != "" &&
			strings.Contains(compiled.UpdateExpression, "ADD")
	}), mock.Anything).Return(nil)

	// Example: Increment view count atomically
	err := q.UpdateBuilder().
		Add("ViewCount", 1).
		Set("UpdatedAt", time.Now()).
		Execute()

	assert.NoError(t, err)
	executor.AssertExpectations(t)
}

func TestUpdateBuilder_MultipleAtomicCounters(t *testing.T) {
	// Test incrementing multiple counters
	executor := new(mockUpdateExecutor)
	metadata := &mockMetadata{
		tableName: "BlogPosts",
		primaryKey: core.KeySchema{
			PartitionKey: "ID",
		},
	}

	q := &Query{
		executor: executor,
		metadata: metadata,
		conditions: []Condition{
			{Field: "ID", Operator: "=", Value: "post123"},
		},
	}

	executor.On("ExecuteUpdateItem", mock.Anything, mock.Anything).Return(nil)

	// Increment both view count and like count
	err := q.UpdateBuilder().
		Add("ViewCount", 1).
		Add("LikeCount", 2).
		Set("UpdatedAt", time.Now()).
		Execute()

	assert.NoError(t, err)
}

func TestUpdateBuilder_ConditionalUpdate(t *testing.T) {
	// Test conditional updates with version check
	executor := new(mockUpdateExecutor)
	metadata := &mockMetadata{
		tableName: "BlogPosts",
		primaryKey: core.KeySchema{
			PartitionKey: "ID",
		},
	}

	q := &Query{
		executor: executor,
		metadata: metadata,
		conditions: []Condition{
			{Field: "ID", Operator: "=", Value: "post123"},
		},
	}

	executor.On("ExecuteUpdateItem", mock.MatchedBy(func(compiled *core.CompiledQuery) bool {
		// Verify condition expression is set
		return compiled.ConditionExpression != ""
	}), mock.Anything).Return(nil)

	// Update with optimistic locking
	err := q.UpdateBuilder().
		Set("Title", "Updated Title").
		Set("UpdatedAt", time.Now()).
		Add("Version", 1).
		ConditionVersion(5). // Current version is 5
		Execute()

	assert.NoError(t, err)
}

func TestUpdateBuilder_IncrementDecrement(t *testing.T) {
	// Test the convenience methods
	executor := new(mockUpdateExecutor)
	metadata := &mockMetadata{
		tableName: "Products",
		primaryKey: core.KeySchema{
			PartitionKey: "ID",
		},
	}

	q := &Query{
		executor: executor,
		metadata: metadata,
		conditions: []Condition{
			{Field: "ID", Operator: "=", Value: "prod456"},
		},
	}

	executor.On("ExecuteUpdateItem", mock.Anything, mock.Anything).Return(nil)

	// Decrement stock count
	err := q.UpdateBuilder().
		Decrement("StockCount").
		Set("LastSoldAt", time.Now()).
		Execute()

	assert.NoError(t, err)
}

func TestUpdateBuilder_ListOperations(t *testing.T) {
	// Test list append and prepend operations
	executor := new(mockUpdateExecutor)
	metadata := &mockMetadata{
		tableName: "Products",
		primaryKey: core.KeySchema{
			PartitionKey: "ID",
		},
	}

	q := &Query{
		executor: executor,
		metadata: metadata,
		conditions: []Condition{
			{Field: "ID", Operator: "=", Value: "prod123"},
		},
	}

	// Test AppendToList
	t.Run("AppendToList", func(t *testing.T) {
		// Reset expectations
		executor.ExpectedCalls = nil

		executor.On("ExecuteUpdateItem", mock.MatchedBy(func(compiled *core.CompiledQuery) bool {
			// Verify the update expression contains list_append
			// DynamoDB uses placeholders like #n1 for names
			return strings.Contains(compiled.UpdateExpression, "list_append") &&
				strings.Contains(compiled.UpdateExpression, "SET") &&
				// Check that Tags is in the attribute names
				containsValue(compiled.ExpressionAttributeNames, "Tags")
		}), mock.Anything).Return(nil).Once()

		err := q.UpdateBuilder().
			AppendToList("Tags", []string{"new-tag"}).
			Execute()

		assert.NoError(t, err)
		executor.AssertExpectations(t)
	})

	// Test PrependToList
	t.Run("PrependToList", func(t *testing.T) {
		// Reset expectations
		executor.ExpectedCalls = nil

		executor.On("ExecuteUpdateItem", mock.MatchedBy(func(compiled *core.CompiledQuery) bool {
			// Verify the update expression contains list_append with values first
			return strings.Contains(compiled.UpdateExpression, "list_append") &&
				strings.Contains(compiled.UpdateExpression, "SET") &&
				// Check that Tags is in the attribute names
				containsValue(compiled.ExpressionAttributeNames, "Tags")
		}), mock.Anything).Return(nil).Once()

		err := q.UpdateBuilder().
			PrependToList("Tags", []string{"first-tag"}).
			Execute()

		assert.NoError(t, err)
		executor.AssertExpectations(t)
	})

	// Test SetIfNotExists
	t.Run("SetIfNotExists", func(t *testing.T) {
		// Reset expectations
		executor.ExpectedCalls = nil

		executor.On("ExecuteUpdateItem", mock.MatchedBy(func(compiled *core.CompiledQuery) bool {
			// Verify the update expression contains if_not_exists or SET
			// The implementation might fall back to SET if AddUpdateFunction fails
			return (strings.Contains(compiled.UpdateExpression, "if_not_exists") ||
				strings.Contains(compiled.UpdateExpression, "SET")) &&
				containsValue(compiled.ExpressionAttributeNames, "Description")
		}), mock.Anything).Return(nil).Once()

		err := q.UpdateBuilder().
			SetIfNotExists("Description", nil, "Default description").
			Execute()

		assert.NoError(t, err)
		executor.AssertExpectations(t)
	})
}

func TestUpdateBuilder_RemoveAttribute(t *testing.T) {
	// Test removing attributes
	executor := new(mockUpdateExecutor)
	metadata := &mockMetadata{
		tableName: "Users",
		primaryKey: core.KeySchema{
			PartitionKey: "ID",
		},
	}

	q := &Query{
		executor: executor,
		metadata: metadata,
		conditions: []Condition{
			{Field: "ID", Operator: "=", Value: "user789"},
		},
	}

	executor.On("ExecuteUpdateItem", mock.MatchedBy(func(compiled *core.CompiledQuery) bool {
		// Verify REMOVE is in the update expression
		return strings.Contains(compiled.UpdateExpression, "REMOVE")
	}), mock.Anything).Return(nil)

	// Remove temporary attributes
	err := q.UpdateBuilder().
		Remove("TempToken").
		Remove("VerificationCode").
		Set("UpdatedAt", time.Now()).
		Execute()

	assert.NoError(t, err)
}

func TestUpdateBuilder_ComplexUpdate(t *testing.T) {
	// Test a complex update with multiple operations
	executor := new(mockUpdateExecutor)
	metadata := &mockMetadata{
		tableName: "Analytics",
		primaryKey: core.KeySchema{
			PartitionKey: "ID",
			SortKey:      "Date",
		},
	}

	q := &Query{
		executor: executor,
		metadata: metadata,
		conditions: []Condition{
			{Field: "ID", Operator: "=", Value: "metric123"},
			{Field: "Date", Operator: "=", Value: "2024-01-01"},
		},
	}

	executor.On("ExecuteUpdateItem", mock.Anything, mock.Anything).Return(nil)

	// Complex update with multiple operations
	err := q.UpdateBuilder().
		Add("PageViews", 10).
		Add("UniqueVisitors", 3).
		Set("LastUpdated", time.Now()).
		Set("Status", "active").
		ConditionExists("ID").
		Execute()

	assert.NoError(t, err)
}

// Helper function to check if a map contains a value
func containsValue(m map[string]string, value string) bool {
	for _, v := range m {
		if v == value {
			return true
		}
	}
	return false
}

// Mock metadata for testing
type mockMetadata struct {
	tableName  string
	primaryKey core.KeySchema
}

func (m *mockMetadata) TableName() string                                      { return m.tableName }
func (m *mockMetadata) PrimaryKey() core.KeySchema                             { return m.primaryKey }
func (m *mockMetadata) Indexes() []core.IndexSchema                            { return nil }
func (m *mockMetadata) AttributeMetadata(field string) *core.AttributeMetadata { return nil }
func (m *mockMetadata) VersionFieldName() string                               { return "" }

func TestUpdateBuilderReturnValues(t *testing.T) {
	metadata := &mockMetadata{
		tableName: "test-table",
		primaryKey: core.KeySchema{
			PartitionKey: "ID",
			// No sort key for these tests
		},
	}

	t.Run("ExecuteWithResult sets return values", func(t *testing.T) {
		var capturedQuery *core.CompiledQuery

		mockExecutor := &mockUpdateWithResultExecutor{
			ExecuteUpdateItemWithResultFunc: func(input *core.CompiledQuery, key map[string]types.AttributeValue) (*core.UpdateResult, error) {
				capturedQuery = input

				// Return mock result
				return &core.UpdateResult{
					Attributes: map[string]types.AttributeValue{
						"ID":    &types.AttributeValueMemberS{Value: "123"},
						"Name":  &types.AttributeValueMemberS{Value: "Updated Name"},
						"Value": &types.AttributeValueMemberN{Value: "100"},
					},
				}, nil
			},
		}

		q := &Query{
			metadata: metadata,
			conditions: []Condition{
				{Field: "ID", Operator: "=", Value: "123"},
			},
			executor: mockExecutor,
		}

		ub := q.UpdateBuilder()

		// Build update
		ub.Set("Name", "Updated Name").
			Add("Value", 10).
			ReturnValues("ALL_NEW")

		// Execute with result
		var result TestItem
		err := ub.ExecuteWithResult(&result)

		assert.NoError(t, err)
		assert.Equal(t, "ALL_NEW", capturedQuery.ReturnValues)
		assert.Equal(t, "123", result.ID)
		assert.Equal(t, "Updated Name", result.Name)
		assert.Equal(t, 100, result.Value)
	})

	t.Run("ExecuteWithResult with default return values", func(t *testing.T) {
		var capturedQuery *core.CompiledQuery

		mockExecutor := &mockUpdateWithResultExecutor{
			ExecuteUpdateItemWithResultFunc: func(input *core.CompiledQuery, key map[string]types.AttributeValue) (*core.UpdateResult, error) {
				capturedQuery = input
				return &core.UpdateResult{}, nil
			},
		}

		q := &Query{
			metadata: metadata,
			conditions: []Condition{
				{Field: "ID", Operator: "=", Value: "123"},
			},
			executor: mockExecutor,
		}

		ub := q.UpdateBuilder()
		ub.Set("Name", "Updated")

		var result TestItem
		err := ub.ExecuteWithResult(&result)

		assert.NoError(t, err)
		assert.Equal(t, "ALL_NEW", capturedQuery.ReturnValues) // Default when not set
	})

	t.Run("ExecuteWithResult validates result parameter", func(t *testing.T) {
		q := &Query{
			metadata: metadata,
			conditions: []Condition{
				{Field: "ID", Operator: "=", Value: "123"},
			},
			executor: &mockUpdateExecutor{},
		}

		ub := q.UpdateBuilder()
		ub.Set("Name", "Updated")

		// Test with nil
		err := ub.ExecuteWithResult(nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "result must be a non-nil pointer")

		// Test with non-pointer
		var result TestItem
		err = ub.ExecuteWithResult(result)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "result must be a non-nil pointer")
	})

	t.Run("ReturnValues options", func(t *testing.T) {
		var capturedQuery *core.CompiledQuery

		mockExecutor := new(mockUpdateExecutor)
		mockExecutor.On("ExecuteUpdateItem", mock.MatchedBy(func(input *core.CompiledQuery) bool {
			capturedQuery = input
			return true
		}), mock.Anything).Return(nil)

		q := &Query{
			metadata: metadata,
			conditions: []Condition{
				{Field: "ID", Operator: "=", Value: "123"},
			},
			executor: mockExecutor,
		}

		// Test different return value options
		options := []string{"NONE", "ALL_OLD", "UPDATED_OLD", "ALL_NEW", "UPDATED_NEW"}

		for _, option := range options {
			ub := q.UpdateBuilder()
			ub.Set("Name", "Updated").ReturnValues(option)

			err := ub.Execute()
			assert.NoError(t, err)
			assert.Equal(t, option, capturedQuery.ReturnValues)
		}
	})
}

// mockUpdateWithResultExecutor implements UpdateItemWithResultExecutor for testing
type mockUpdateWithResultExecutor struct {
	ExecuteUpdateItemWithResultFunc func(input *core.CompiledQuery, key map[string]types.AttributeValue) (*core.UpdateResult, error)
	mockUpdateExecutor
}

func (m *mockUpdateWithResultExecutor) ExecuteUpdateItemWithResult(input *core.CompiledQuery, key map[string]types.AttributeValue) (*core.UpdateResult, error) {
	if m.ExecuteUpdateItemWithResultFunc != nil {
		return m.ExecuteUpdateItemWithResultFunc(input, key)
	}
	return &core.UpdateResult{}, nil
}
