package query

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/core"
	customerrors "github.com/theory-cloud/tabletheory/pkg/errors"
)

// MockDynamoDBClient is a mock implementation of DynamoDBAPI
type (
	queryFn          func(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	scanFn           func(ctx context.Context, params *dynamodb.ScanInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error)
	getItemFn        func(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	putItemFn        func(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	updateItemFn     func(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
	deleteItemFn     func(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
	batchGetItemFn   func(ctx context.Context, params *dynamodb.BatchGetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchGetItemOutput, error)
	batchWriteItemFn func(ctx context.Context, params *dynamodb.BatchWriteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchWriteItemOutput, error)
)

type MockDynamoDBClient struct {
	QueryFunc          queryFn
	ScanFunc           scanFn
	GetItemFunc        getItemFn
	PutItemFunc        putItemFn
	UpdateItemFunc     updateItemFn
	DeleteItemFunc     deleteItemFn
	BatchGetItemFunc   batchGetItemFn
	BatchWriteItemFunc batchWriteItemFn
}

func (m *MockDynamoDBClient) Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	if m.QueryFunc != nil {
		return m.QueryFunc(ctx, params, optFns...)
	}
	return &dynamodb.QueryOutput{}, nil
}

func (m *MockDynamoDBClient) Scan(ctx context.Context, params *dynamodb.ScanInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
	if m.ScanFunc != nil {
		return m.ScanFunc(ctx, params, optFns...)
	}
	return &dynamodb.ScanOutput{}, nil
}

func (m *MockDynamoDBClient) GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	if m.GetItemFunc != nil {
		return m.GetItemFunc(ctx, params, optFns...)
	}
	return &dynamodb.GetItemOutput{}, nil
}

func (m *MockDynamoDBClient) PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	if m.PutItemFunc != nil {
		return m.PutItemFunc(ctx, params, optFns...)
	}
	return &dynamodb.PutItemOutput{}, nil
}

func (m *MockDynamoDBClient) UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	if m.UpdateItemFunc != nil {
		return m.UpdateItemFunc(ctx, params, optFns...)
	}
	return &dynamodb.UpdateItemOutput{}, nil
}

func (m *MockDynamoDBClient) DeleteItem(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	if m.DeleteItemFunc != nil {
		return m.DeleteItemFunc(ctx, params, optFns...)
	}
	return &dynamodb.DeleteItemOutput{}, nil
}

func (m *MockDynamoDBClient) BatchGetItem(ctx context.Context, params *dynamodb.BatchGetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchGetItemOutput, error) {
	if m.BatchGetItemFunc != nil {
		return m.BatchGetItemFunc(ctx, params, optFns...)
	}
	return &dynamodb.BatchGetItemOutput{}, nil
}

func (m *MockDynamoDBClient) BatchWriteItem(ctx context.Context, params *dynamodb.BatchWriteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchWriteItemOutput, error) {
	if m.BatchWriteItemFunc != nil {
		return m.BatchWriteItemFunc(ctx, params, optFns...)
	}
	return &dynamodb.BatchWriteItemOutput{}, nil
}

func TestMainExecutorImplementsInterfaces(t *testing.T) {
	// This test ensures that MainExecutor implements all required interfaces
	ctx := context.Background()
	mockClient := &MockDynamoDBClient{}
	executor := NewExecutor(mockClient, ctx)

	// Test interface implementations
	var _ QueryExecutor = executor
	var _ GetItemExecutor = executor
	var _ PutItemExecutor = executor
	var _ UpdateItemExecutor = executor
	var _ DeleteItemExecutor = executor
	var _ PaginatedQueryExecutor = executor
	var _ BatchExecutor = executor
}

func TestExecutePutItem(t *testing.T) {
	ctx := context.Background()

	t.Run("successful put", func(t *testing.T) {
		var capturedInput *dynamodb.PutItemInput
		mockClient := &MockDynamoDBClient{
			PutItemFunc: func(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
				capturedInput = params
				return &dynamodb.PutItemOutput{}, nil
			},
		}

		executor := NewExecutor(mockClient, ctx)

		input := &core.CompiledQuery{
			TableName:           "test-table",
			ConditionExpression: "attribute_not_exists(id)",
			ExpressionAttributeNames: map[string]string{
				"#name": "name",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":val": &types.AttributeValueMemberS{Value: "test"},
			},
		}

		item := map[string]types.AttributeValue{
			"id":   &types.AttributeValueMemberS{Value: "123"},
			"name": &types.AttributeValueMemberS{Value: "Test Item"},
		}

		err := executor.ExecutePutItem(input, item)
		require.NoError(t, err)

		// Verify the captured input
		assert.Equal(t, "test-table", *capturedInput.TableName)
		assert.Equal(t, "attribute_not_exists(id)", *capturedInput.ConditionExpression)
		assert.Equal(t, item, capturedInput.Item)
	})

	t.Run("conditional failure returns sentinel error", func(t *testing.T) {
		mockClient := &MockDynamoDBClient{
			PutItemFunc: func(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
				return nil, &types.ConditionalCheckFailedException{
					Message: aws.String("condition failed"),
				}
			},
		}

		executor := NewExecutor(mockClient, ctx)
		input := &core.CompiledQuery{TableName: "test-table"}
		item := map[string]types.AttributeValue{"id": &types.AttributeValueMemberS{Value: "123"}}

		err := executor.ExecutePutItem(input, item)
		require.Error(t, err)
		assert.ErrorIs(t, err, customerrors.ErrConditionFailed)
	})
}

func TestExecuteUpdateItem(t *testing.T) {
	ctx := context.Background()

	t.Run("delegates to UpdateExecutor", func(t *testing.T) {
		var capturedInput *dynamodb.UpdateItemInput
		mockClient := &MockDynamoDBClient{
			UpdateItemFunc: func(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
				capturedInput = params
				return &dynamodb.UpdateItemOutput{}, nil
			},
		}

		executor := NewExecutor(mockClient, ctx)

		input := &core.CompiledQuery{
			TableName:        "test-table",
			UpdateExpression: "SET #name = :name",
			ExpressionAttributeNames: map[string]string{
				"#name": "name",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":name": &types.AttributeValueMemberS{Value: "Updated Name"},
			},
		}

		key := map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: "123"},
		}

		err := executor.ExecuteUpdateItem(input, key)
		require.NoError(t, err)

		// Verify the update was called
		assert.NotNil(t, capturedInput)
		assert.Equal(t, "test-table", *capturedInput.TableName)
		assert.Equal(t, key, capturedInput.Key)
	})
}

func TestExecuteDeleteItem(t *testing.T) {
	ctx := context.Background()

	t.Run("successful delete", func(t *testing.T) {
		var capturedInput *dynamodb.DeleteItemInput
		mockClient := &MockDynamoDBClient{
			DeleteItemFunc: func(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
				capturedInput = params
				return &dynamodb.DeleteItemOutput{}, nil
			},
		}

		executor := NewExecutor(mockClient, ctx)

		input := &core.CompiledQuery{
			TableName:           "test-table",
			ConditionExpression: "attribute_exists(id)",
		}

		key := map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: "123"},
		}

		err := executor.ExecuteDeleteItem(input, key)
		require.NoError(t, err)

		// Verify the captured input
		assert.Equal(t, "test-table", *capturedInput.TableName)
		assert.Equal(t, "attribute_exists(id)", *capturedInput.ConditionExpression)
		assert.Equal(t, key, capturedInput.Key)
	})

	t.Run("conditional delete failure returns sentinel error", func(t *testing.T) {
		mockClient := &MockDynamoDBClient{
			DeleteItemFunc: func(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
				return nil, &types.ConditionalCheckFailedException{Message: aws.String("cond fail")}
			},
		}

		executor := NewExecutor(mockClient, ctx)
		input := &core.CompiledQuery{TableName: "test-table"}
		key := map[string]types.AttributeValue{"id": &types.AttributeValueMemberS{Value: "123"}}

		err := executor.ExecuteDeleteItem(input, key)
		require.Error(t, err)
		assert.ErrorIs(t, err, customerrors.ErrConditionFailed)
	})
}

func TestExecuteBatchGetRetriesUnprocessedKeys(t *testing.T) {
	ctx := context.Background()
	callCount := 0

	mockClient := &MockDynamoDBClient{
		BatchGetItemFunc: func(ctx context.Context, params *dynamodb.BatchGetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchGetItemOutput, error) {
			callCount++
			if callCount == 1 {
				return &dynamodb.BatchGetItemOutput{
					Responses: map[string][]map[string]types.AttributeValue{
						"tbl": {
							{"id": &types.AttributeValueMemberS{Value: "1"}},
						},
					},
					UnprocessedKeys: map[string]types.KeysAndAttributes{
						"tbl": {
							Keys: []map[string]types.AttributeValue{
								{"id": &types.AttributeValueMemberS{Value: "2"}},
							},
						},
					},
				}, nil
			}
			return &dynamodb.BatchGetItemOutput{
				Responses: map[string][]map[string]types.AttributeValue{
					"tbl": {
						{"id": &types.AttributeValueMemberS{Value: "2"}},
					},
				},
			}, nil
		},
	}

	executor := NewExecutor(mockClient, ctx)
	input := &CompiledBatchGet{
		TableName: "tbl",
		Keys: []map[string]types.AttributeValue{
			{"id": &types.AttributeValueMemberS{Value: "1"}},
			{"id": &types.AttributeValueMemberS{Value: "2"}},
		},
	}

	items, err := executor.ExecuteBatchGet(input, core.DefaultBatchGetOptions())
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, 2, callCount)
}

func TestExecuteBatchGetExhaustsRetries(t *testing.T) {
	ctx := context.Background()
	mockClient := &MockDynamoDBClient{
		BatchGetItemFunc: func(ctx context.Context, params *dynamodb.BatchGetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchGetItemOutput, error) {
			return &dynamodb.BatchGetItemOutput{
				Responses: map[string][]map[string]types.AttributeValue{
					"tbl": {
						{"id": &types.AttributeValueMemberS{Value: "1"}},
					},
				},
				UnprocessedKeys: map[string]types.KeysAndAttributes{
					"tbl": {
						Keys: []map[string]types.AttributeValue{
							{"id": &types.AttributeValueMemberS{Value: "2"}},
						},
					},
				},
			}, nil
		},
	}

	executor := NewExecutor(mockClient, ctx)
	input := &CompiledBatchGet{
		TableName: "tbl",
		Keys: []map[string]types.AttributeValue{
			{"id": &types.AttributeValueMemberS{Value: "1"}},
			{"id": &types.AttributeValueMemberS{Value: "2"}},
		},
	}

	opts := core.DefaultBatchGetOptions()
	opts.RetryPolicy.MaxRetries = 0

	items, err := executor.ExecuteBatchGet(input, opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exhausted retries")
	require.Len(t, items, 1)
}

func TestExecuteBatchGetHonorsNilRetryPolicy(t *testing.T) {
	ctx := context.Background()
	callCount := 0
	mockClient := &MockDynamoDBClient{
		BatchGetItemFunc: func(ctx context.Context, params *dynamodb.BatchGetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchGetItemOutput, error) {
			callCount++
			return &dynamodb.BatchGetItemOutput{
				Responses: map[string][]map[string]types.AttributeValue{
					"tbl": {
						{"id": &types.AttributeValueMemberS{Value: "1"}},
					},
				},
				UnprocessedKeys: map[string]types.KeysAndAttributes{
					"tbl": {
						Keys: []map[string]types.AttributeValue{
							{"id": &types.AttributeValueMemberS{Value: "2"}},
						},
					},
				},
			}, nil
		},
	}

	executor := NewExecutor(mockClient, ctx)
	input := &CompiledBatchGet{
		TableName: "tbl",
		Keys: []map[string]types.AttributeValue{
			{"id": &types.AttributeValueMemberS{Value: "1"}},
			{"id": &types.AttributeValueMemberS{Value: "2"}},
		},
	}

	opts := &core.BatchGetOptions{
		RetryPolicy: nil,
	}

	items, err := executor.ExecuteBatchGet(input, opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exhausted retries")
	require.Len(t, items, 1)
	assert.Equal(t, 1, callCount, "should not retry when retry policy is nil")
}
