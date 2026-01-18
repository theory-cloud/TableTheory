package query

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/core"
)

func TestMainExecutor_ExecuteBatchGet_NilInputAndEmptyKeys_COV6(t *testing.T) {
	executor := NewExecutor(&MockDynamoDBClient{}, context.Background())

	_, err := executor.ExecuteBatchGet(nil, nil)
	require.Error(t, err)

	items, err := executor.ExecuteBatchGet(&CompiledBatchGet{TableName: "tbl"}, nil)
	require.NoError(t, err)
	require.Nil(t, items)
}

func TestMainExecutor_ExecuteBatchGet_BreaksOnZeroRemainingUnprocessed_COV6(t *testing.T) {
	ctx := context.Background()
	callCount := 0
	mockClient := &MockDynamoDBClient{
		BatchGetItemFunc: func(context.Context, *dynamodb.BatchGetItemInput, ...func(*dynamodb.Options)) (*dynamodb.BatchGetItemOutput, error) {
			callCount++
			return &dynamodb.BatchGetItemOutput{
				Responses: map[string][]map[string]types.AttributeValue{
					"tbl": {
						{"id": &types.AttributeValueMemberS{Value: "1"}},
					},
				},
				UnprocessedKeys: map[string]types.KeysAndAttributes{
					"tbl": {Keys: nil},
				},
			}, nil
		},
	}

	executor := NewExecutor(mockClient, ctx)
	items, err := executor.ExecuteBatchGet(&CompiledBatchGet{
		TableName: "tbl",
		Keys: []map[string]types.AttributeValue{
			{"id": &types.AttributeValueMemberS{Value: "1"}},
		},
	}, nil)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, 1, callCount)
}

func TestCountUnprocessedKeys_EmptyInput_COV6(t *testing.T) {
	require.Equal(t, 0, countUnprocessedKeys(nil))
	require.Equal(t, 0, countUnprocessedKeys(map[string]types.KeysAndAttributes{}))
}

func TestMainExecutor_ExecuteBatchGet_WrapsClientErrors_COV6(t *testing.T) {
	mockClient := &MockDynamoDBClient{
		BatchGetItemFunc: func(context.Context, *dynamodb.BatchGetItemInput, ...func(*dynamodb.Options)) (*dynamodb.BatchGetItemOutput, error) {
			return nil, errors.New("boom")
		},
	}

	executor := NewExecutor(mockClient, context.Background())
	items, err := executor.ExecuteBatchGet(&CompiledBatchGet{
		TableName: "tbl",
		Keys: []map[string]types.AttributeValue{
			{"id": &types.AttributeValueMemberS{Value: "1"}},
		},
	}, nil)
	require.Error(t, err)
	require.Nil(t, items)
	assert.Contains(t, err.Error(), "failed to batch get items")
	assert.Contains(t, err.Error(), "boom")
}

func TestMainExecutor_ExecuteQueryWithPagination_ErrorBranches_COV6(t *testing.T) {
	ctx := context.Background()

	t.Run("wraps query errors", func(t *testing.T) {
		executor := NewExecutor(&MockDynamoDBClient{
			QueryFunc: func(context.Context, *dynamodb.QueryInput, ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
				return nil, errors.New("boom")
			},
		}, ctx)

		_, err := executor.ExecuteQueryWithPagination(&core.CompiledQuery{TableName: "tbl"}, &[]struct{}{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to execute query")
		assert.Contains(t, err.Error(), "boom")
	})

	t.Run("bubbles unmarshal errors", func(t *testing.T) {
		executor := NewExecutor(&MockDynamoDBClient{
			QueryFunc: func(context.Context, *dynamodb.QueryInput, ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
				return &dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{}}, nil
			},
		}, ctx)

		_, err := executor.ExecuteQueryWithPagination(&core.CompiledQuery{TableName: "tbl"}, struct{}{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "destination must be a pointer")
	})
}

func TestMainExecutor_ExecuteScanWithPagination_ErrorBranches_COV6(t *testing.T) {
	ctx := context.Background()

	t.Run("wraps scan errors", func(t *testing.T) {
		executor := NewExecutor(&MockDynamoDBClient{
			ScanFunc: func(context.Context, *dynamodb.ScanInput, ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
				return nil, errors.New("boom")
			},
		}, ctx)

		_, err := executor.ExecuteScanWithPagination(&core.CompiledQuery{TableName: "tbl"}, &[]struct{}{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to execute scan")
		assert.Contains(t, err.Error(), "boom")
	})

	t.Run("bubbles unmarshal errors", func(t *testing.T) {
		executor := NewExecutor(&MockDynamoDBClient{
			ScanFunc: func(context.Context, *dynamodb.ScanInput, ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
				return &dynamodb.ScanOutput{Items: []map[string]types.AttributeValue{}}, nil
			},
		}, ctx)

		_, err := executor.ExecuteScanWithPagination(&core.CompiledQuery{TableName: "tbl"}, struct{}{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "destination must be a pointer")
	})
}

func TestMainExecutor_ExecutePutItem_ValidationAndErrorWrapping_COV6(t *testing.T) {
	ctx := context.Background()

	t.Run("nil input returns error", func(t *testing.T) {
		executor := NewExecutor(&MockDynamoDBClient{}, ctx)
		err := executor.ExecutePutItem(nil, map[string]types.AttributeValue{"id": &types.AttributeValueMemberS{Value: "1"}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "compiled query cannot be nil")
	})

	t.Run("empty item returns error", func(t *testing.T) {
		executor := NewExecutor(&MockDynamoDBClient{}, ctx)
		err := executor.ExecutePutItem(&core.CompiledQuery{TableName: "tbl"}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "item cannot be empty")
	})

	t.Run("wraps non-conditional errors", func(t *testing.T) {
		executor := NewExecutor(&MockDynamoDBClient{
			PutItemFunc: func(context.Context, *dynamodb.PutItemInput, ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
				return nil, errors.New("boom")
			},
		}, ctx)

		err := executor.ExecutePutItem(&core.CompiledQuery{TableName: "tbl"}, map[string]types.AttributeValue{"id": &types.AttributeValueMemberS{Value: "1"}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to put item")
		assert.Contains(t, err.Error(), "boom")
	})
}
