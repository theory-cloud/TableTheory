package core

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockDynamoDBClient is a mock implementation of the DynamoDB client
type MockDynamoDBClient struct {
	UpdateItemFunc func(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
}

func (m *MockDynamoDBClient) UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	if m.UpdateItemFunc != nil {
		return m.UpdateItemFunc(ctx, params, optFns...)
	}
	return &dynamodb.UpdateItemOutput{}, nil
}

func TestExecuteUpdateItem(t *testing.T) {
	ctx := context.Background()

	t.Run("successful update", func(t *testing.T) {
		var capturedInput *dynamodb.UpdateItemInput
		mockClient := &MockDynamoDBClient{
			UpdateItemFunc: func(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
				capturedInput = params
				return &dynamodb.UpdateItemOutput{}, nil
			},
		}

		executor := NewUpdateExecutor(mockClient, ctx)

		input := &CompiledQuery{
			TableName:           "test-table",
			UpdateExpression:    "SET #name = :name, #age = :age",
			ConditionExpression: "attribute_exists(id)",
			ExpressionAttributeNames: map[string]string{
				"#name": "name",
				"#age":  "age",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":name": &types.AttributeValueMemberS{Value: "John Doe"},
				":age":  &types.AttributeValueMemberN{Value: "30"},
			},
			ReturnValues: "ALL_NEW",
		}

		key := map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: "123"},
		}

		err := executor.ExecuteUpdateItem(input, key)
		require.NoError(t, err)

		// Verify the captured input
		assert.Equal(t, "test-table", aws.ToString(capturedInput.TableName))
		assert.Equal(t, "SET #name = :name, #age = :age", aws.ToString(capturedInput.UpdateExpression))
		assert.Equal(t, "attribute_exists(id)", aws.ToString(capturedInput.ConditionExpression))
		assert.Equal(t, input.ExpressionAttributeNames, capturedInput.ExpressionAttributeNames)
		assert.Equal(t, input.ExpressionAttributeValues, capturedInput.ExpressionAttributeValues)
		assert.Equal(t, types.ReturnValueAllNew, capturedInput.ReturnValues)
		assert.Equal(t, key, capturedInput.Key)
	})

	t.Run("nil input error", func(t *testing.T) {
		mockClient := &MockDynamoDBClient{}
		executor := NewUpdateExecutor(mockClient, ctx)

		err := executor.ExecuteUpdateItem(nil, map[string]types.AttributeValue{"id": &types.AttributeValueMemberS{Value: "123"}})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "compiled query cannot be nil")
	})

	t.Run("empty key error", func(t *testing.T) {
		mockClient := &MockDynamoDBClient{}
		executor := NewUpdateExecutor(mockClient, ctx)

		input := &CompiledQuery{
			TableName: "test-table",
		}

		err := executor.ExecuteUpdateItem(input, map[string]types.AttributeValue{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "key cannot be empty")
	})

	t.Run("DynamoDB error", func(t *testing.T) {
		mockClient := &MockDynamoDBClient{
			UpdateItemFunc: func(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
				return nil, errors.New("DynamoDB error")
			},
		}

		executor := NewUpdateExecutor(mockClient, ctx)

		input := &CompiledQuery{
			TableName: "test-table",
		}

		key := map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: "123"},
		}

		err := executor.ExecuteUpdateItem(input, key)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update item")
	})

	t.Run("default return values", func(t *testing.T) {
		var capturedInput *dynamodb.UpdateItemInput
		mockClient := &MockDynamoDBClient{
			UpdateItemFunc: func(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
				capturedInput = params
				return &dynamodb.UpdateItemOutput{}, nil
			},
		}

		executor := NewUpdateExecutor(mockClient, ctx)

		input := &CompiledQuery{
			TableName: "test-table",
			// ReturnValues not set, should default to NONE
		}

		key := map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: "123"},
		}

		err := executor.ExecuteUpdateItem(input, key)
		require.NoError(t, err)

		// Verify default return value
		assert.Equal(t, types.ReturnValueNone, capturedInput.ReturnValues)
	})
}

func TestExecuteUpdateItemWithResult(t *testing.T) {
	ctx := context.Background()

	t.Run("successful update with result", func(t *testing.T) {
		mockClient := &MockDynamoDBClient{
			UpdateItemFunc: func(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
				return &dynamodb.UpdateItemOutput{
					Attributes: map[string]types.AttributeValue{
						"id":   &types.AttributeValueMemberS{Value: "123"},
						"name": &types.AttributeValueMemberS{Value: "Updated Name"},
						"age":  &types.AttributeValueMemberN{Value: "31"},
					},
					ConsumedCapacity: &types.ConsumedCapacity{
						TableName:          aws.String("test-table"),
						CapacityUnits:      aws.Float64(2.5),
						ReadCapacityUnits:  aws.Float64(1.0),
						WriteCapacityUnits: aws.Float64(1.5),
					},
				}, nil
			},
		}

		executor := NewUpdateExecutor(mockClient, ctx)

		input := &CompiledQuery{
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

		result, err := executor.ExecuteUpdateItemWithResult(input, key)
		require.NoError(t, err)
		require.NotNil(t, result)

		// Verify attributes
		assert.Equal(t, 3, len(result.Attributes))
		assert.Equal(t, &types.AttributeValueMemberS{Value: "123"}, result.Attributes["id"])
		assert.Equal(t, &types.AttributeValueMemberS{Value: "Updated Name"}, result.Attributes["name"])

		// Verify consumed capacity
		require.NotNil(t, result.ConsumedCapacity)
		assert.Equal(t, "test-table", result.ConsumedCapacity.TableName)
		assert.Equal(t, 2.5, result.ConsumedCapacity.CapacityUnits)
		assert.Equal(t, 1.0, result.ConsumedCapacity.ReadCapacityUnits)
		assert.Equal(t, 1.5, result.ConsumedCapacity.WriteCapacityUnits)
	})

	t.Run("update with no consumed capacity", func(t *testing.T) {
		mockClient := &MockDynamoDBClient{
			UpdateItemFunc: func(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
				return &dynamodb.UpdateItemOutput{
					Attributes: map[string]types.AttributeValue{
						"id": &types.AttributeValueMemberS{Value: "123"},
					},
					// ConsumedCapacity is nil
				}, nil
			},
		}

		executor := NewUpdateExecutor(mockClient, ctx)

		input := &CompiledQuery{
			TableName: "test-table",
		}

		key := map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: "123"},
		}

		result, err := executor.ExecuteUpdateItemWithResult(input, key)
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, 1, len(result.Attributes))
		assert.Nil(t, result.ConsumedCapacity)
	})
}
