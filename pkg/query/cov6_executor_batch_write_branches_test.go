package query

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAttributeValueToInterface_CoversBinaryAndSetCases_COV6(t *testing.T) {
	out, err := attributeValueToInterface(&types.AttributeValueMemberSS{Value: []string{"a", "b"}})
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b"}, out)

	out, err = attributeValueToInterface(&types.AttributeValueMemberBS{Value: [][]byte{[]byte("x")}})
	require.NoError(t, err)
	require.Equal(t, [][]byte{[]byte("x")}, out)

	out, err = attributeValueToInterface(&types.AttributeValueMemberB{Value: []byte("raw")})
	require.NoError(t, err)
	require.Equal(t, []byte("raw"), out)
}

func TestUnmarshalAttributeValue_NullClearsScalarValue_COV6(t *testing.T) {
	v := 42
	dest := reflect.ValueOf(&v).Elem()

	err := unmarshalAttributeValue(&types.AttributeValueMemberNULL{Value: true}, dest)
	require.NoError(t, err)
	require.Zero(t, v)
}

func TestUnmarshalAnyAttributeValue_PropagatesUnsupportedType_COV6(t *testing.T) {
	type unsupportedAV struct{ types.AttributeValue }

	var out any
	dest := reflect.ValueOf(&out).Elem()

	err := unmarshalAttributeValue(&unsupportedAV{}, dest)
	require.Error(t, err)
}

func TestMainExecutor_ExecuteBatchWrite_Branches_COV6(t *testing.T) {
	ctx := context.Background()

	t.Run("nil input returns error", func(t *testing.T) {
		executor := NewExecutor(&MockDynamoDBClient{}, ctx)
		require.Error(t, executor.ExecuteBatchWrite(nil))
	})

	t.Run("empty items is a no-op", func(t *testing.T) {
		executor := NewExecutor(&MockDynamoDBClient{}, ctx)
		require.NoError(t, executor.ExecuteBatchWrite(&CompiledBatchWrite{TableName: "tbl"}))
	})

	t.Run("wraps client errors", func(t *testing.T) {
		mockClient := &MockDynamoDBClient{
			BatchWriteItemFunc: func(context.Context, *dynamodb.BatchWriteItemInput, ...func(*dynamodb.Options)) (*dynamodb.BatchWriteItemOutput, error) {
				return nil, errors.New("boom")
			},
		}

		executor := NewExecutor(mockClient, ctx)
		err := executor.ExecuteBatchWrite(&CompiledBatchWrite{
			TableName: "tbl",
			Items: []map[string]types.AttributeValue{
				{"id": &types.AttributeValueMemberS{Value: "1"}},
			},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to batch write items")
		assert.Contains(t, err.Error(), "boom")
	})

	t.Run("retries unprocessed items without delay", func(t *testing.T) {
		callCount := 0
		mockClient := &MockDynamoDBClient{
			BatchWriteItemFunc: func(_ context.Context, params *dynamodb.BatchWriteItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.BatchWriteItemOutput, error) {
				callCount++
				if callCount == 1 {
					require.Contains(t, params.RequestItems, "tbl")
					unprocessed := map[string][]types.WriteRequest{
						"tbl": params.RequestItems["tbl"][:1],
					}
					return &dynamodb.BatchWriteItemOutput{UnprocessedItems: unprocessed}, nil
				}
				return &dynamodb.BatchWriteItemOutput{}, nil
			},
		}

		executor := NewExecutor(mockClient, ctx)
		require.NoError(t, executor.ExecuteBatchWrite(&CompiledBatchWrite{
			TableName: "tbl",
			Items: []map[string]types.AttributeValue{
				{"id": &types.AttributeValueMemberS{Value: "1"}},
				{"id": &types.AttributeValueMemberS{Value: "2"}},
			},
		}))
		require.Equal(t, 2, callCount)
	})
}
