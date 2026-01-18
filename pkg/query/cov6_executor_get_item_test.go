package query

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/core"
	customerrors "github.com/theory-cloud/tabletheory/pkg/errors"
)

type stubDynamoDBAPI struct {
	getItem func(context.Context, *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error)
}

func (s *stubDynamoDBAPI) Query(context.Context, *dynamodb.QueryInput, ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	return nil, errors.New("unexpected call")
}

func (s *stubDynamoDBAPI) Scan(context.Context, *dynamodb.ScanInput, ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
	return nil, errors.New("unexpected call")
}

func (s *stubDynamoDBAPI) GetItem(ctx context.Context, params *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	if s.getItem == nil {
		return nil, errors.New("GetItem not configured")
	}
	return s.getItem(ctx, params)
}

func (s *stubDynamoDBAPI) PutItem(context.Context, *dynamodb.PutItemInput, ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	return nil, errors.New("unexpected call")
}

func (s *stubDynamoDBAPI) UpdateItem(context.Context, *dynamodb.UpdateItemInput, ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	return nil, errors.New("unexpected call")
}

func (s *stubDynamoDBAPI) DeleteItem(context.Context, *dynamodb.DeleteItemInput, ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	return nil, errors.New("unexpected call")
}

func (s *stubDynamoDBAPI) BatchGetItem(context.Context, *dynamodb.BatchGetItemInput, ...func(*dynamodb.Options)) (*dynamodb.BatchGetItemOutput, error) {
	return nil, errors.New("unexpected call")
}

func (s *stubDynamoDBAPI) BatchWriteItem(context.Context, *dynamodb.BatchWriteItemInput, ...func(*dynamodb.Options)) (*dynamodb.BatchWriteItemOutput, error) {
	return nil, errors.New("unexpected call")
}

func TestMainExecutor_SetContext_DefaultsToBackground_COV6(t *testing.T) {
	executor := NewExecutor(&stubDynamoDBAPI{}, nil)
	require.Nil(t, executor.ctx)

	executor.SetContext(nil) //nolint:staticcheck // cover nil context handling
	require.Equal(t, context.Background(), executor.ctx)
}

func TestMainExecutor_ExecuteGetItem_Branches_COV6(t *testing.T) {
	consistent := true
	compiled := &core.CompiledQuery{
		TableName:            "TestTable",
		ProjectionExpression: "#id",
		ExpressionAttributeNames: map[string]string{
			"#id": "id",
		},
		ConsistentRead: &consistent,
	}

	key := map[string]types.AttributeValue{
		"id": &types.AttributeValueMemberS{Value: "1"},
	}

	t.Run("Valid_MapDestination", func(t *testing.T) {
		client := &stubDynamoDBAPI{
			getItem: func(ctx context.Context, input *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
				require.Equal(t, context.Background(), ctx)
				require.NotNil(t, input.TableName)
				require.Equal(t, "TestTable", *input.TableName)
				require.Equal(t, compiled.ExpressionAttributeNames, input.ExpressionAttributeNames)
				require.NotNil(t, input.ProjectionExpression)
				require.Equal(t, "#id", *input.ProjectionExpression)
				require.Equal(t, compiled.ConsistentRead, input.ConsistentRead)
				require.Equal(t, key, input.Key)

				return &dynamodb.GetItemOutput{
					Item: map[string]types.AttributeValue{
						"id": &types.AttributeValueMemberS{Value: "1"},
					},
				}, nil
			},
		}

		executor := NewExecutor(client, context.Background())
		executor.SetContext(nil) //nolint:staticcheck // cover nil context handling

		var dest map[string]types.AttributeValue
		err := executor.ExecuteGetItem(compiled, key, &dest)
		require.NoError(t, err)
		require.Equal(t, key["id"], dest["id"])
	})

	t.Run("Valid_StructDestination", func(t *testing.T) {
		type item struct {
			ID string `theorydb:"id"`
		}

		client := &stubDynamoDBAPI{
			getItem: func(ctx context.Context, input *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
				return &dynamodb.GetItemOutput{
					Item: map[string]types.AttributeValue{
						"id": &types.AttributeValueMemberS{Value: "1"},
					},
				}, nil
			},
		}

		executor := NewExecutor(client, context.Background())
		var dest item
		err := executor.ExecuteGetItem(compiled, key, &dest)
		require.NoError(t, err)
		require.Equal(t, "1", dest.ID)
	})

	t.Run("NotFound", func(t *testing.T) {
		client := &stubDynamoDBAPI{
			getItem: func(ctx context.Context, input *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
				return &dynamodb.GetItemOutput{Item: nil}, nil
			},
		}

		executor := NewExecutor(client, context.Background())
		err := executor.ExecuteGetItem(compiled, key, &map[string]types.AttributeValue{})
		require.ErrorIs(t, err, customerrors.ErrItemNotFound)
	})

	t.Run("NilInput", func(t *testing.T) {
		executor := NewExecutor(&stubDynamoDBAPI{}, context.Background())
		err := executor.ExecuteGetItem(nil, key, &map[string]types.AttributeValue{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "compiled query cannot be nil")
	})

	t.Run("EmptyKey", func(t *testing.T) {
		executor := NewExecutor(&stubDynamoDBAPI{}, context.Background())
		err := executor.ExecuteGetItem(compiled, nil, &map[string]types.AttributeValue{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "key cannot be empty")
	})

	t.Run("ClientError", func(t *testing.T) {
		sentinel := errors.New("boom")
		client := &stubDynamoDBAPI{
			getItem: func(ctx context.Context, input *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
				return nil, sentinel
			},
		}

		executor := NewExecutor(client, context.Background())
		err := executor.ExecuteGetItem(compiled, key, &map[string]types.AttributeValue{})
		require.Error(t, err)
		require.True(t, errors.Is(err, sentinel), fmt.Sprintf("expected %v to wrap %v", err, sentinel))
	})
}
