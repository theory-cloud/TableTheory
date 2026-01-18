package query

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/core"
)

type cov5DynamoDBClient struct {
	queryInput  *dynamodb.QueryInput
	queryOutput *dynamodb.QueryOutput
	queryErr    error

	scanInput  *dynamodb.ScanInput
	scanOutput *dynamodb.ScanOutput
	scanErr    error

	updateInput  *dynamodb.UpdateItemInput
	updateOutput *dynamodb.UpdateItemOutput
	updateErr    error
}

func (c *cov5DynamoDBClient) Query(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	c.queryInput = params
	return c.queryOutput, c.queryErr
}

func (c *cov5DynamoDBClient) Scan(_ context.Context, params *dynamodb.ScanInput, _ ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
	c.scanInput = params
	return c.scanOutput, c.scanErr
}

func (c *cov5DynamoDBClient) GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	return nil, errors.New("not implemented")
}

func (c *cov5DynamoDBClient) PutItem(context.Context, *dynamodb.PutItemInput, ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	return nil, errors.New("not implemented")
}

func (c *cov5DynamoDBClient) UpdateItem(_ context.Context, params *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	c.updateInput = params
	return c.updateOutput, c.updateErr
}

func (c *cov5DynamoDBClient) DeleteItem(context.Context, *dynamodb.DeleteItemInput, ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	return nil, errors.New("not implemented")
}

func (c *cov5DynamoDBClient) BatchGetItem(context.Context, *dynamodb.BatchGetItemInput, ...func(*dynamodb.Options)) (*dynamodb.BatchGetItemOutput, error) {
	return nil, errors.New("not implemented")
}

func (c *cov5DynamoDBClient) BatchWriteItem(context.Context, *dynamodb.BatchWriteItemInput, ...func(*dynamodb.Options)) (*dynamodb.BatchWriteItemOutput, error) {
	return nil, errors.New("not implemented")
}

func TestMainExecutor_ExecuteQueryWithPagination_COV5(t *testing.T) {
	exec := NewExecutor(&cov5DynamoDBClient{}, context.Background())

	var dest []struct {
		ID string `dynamodb:"id"`
	}
	_, err := exec.ExecuteQueryWithPagination(nil, &dest)
	require.Error(t, err)

	limit := int32(2)
	scanForward := false
	consistent := true
	startKey := map[string]types.AttributeValue{
		"id": &types.AttributeValueMemberS{Value: "start"},
	}

	client := &cov5DynamoDBClient{
		queryOutput: &dynamodb.QueryOutput{
			Items: []map[string]types.AttributeValue{
				{"id": &types.AttributeValueMemberS{Value: "u1"}},
				{"id": &types.AttributeValueMemberS{Value: "u2"}},
			},
			ScannedCount:     2,
			LastEvaluatedKey: startKey,
		},
	}
	exec = NewExecutor(client, context.Background())

	input := &core.CompiledQuery{
		TableName:              "tbl",
		IndexName:              "byStatus",
		KeyConditionExpression: "#id = :id",
		FilterExpression:       "#st = :st",
		ProjectionExpression:   "#id",
		ExpressionAttributeNames: map[string]string{
			"#id": "id",
			"#st": "status",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":id": &types.AttributeValueMemberS{Value: "u1"},
			":st": &types.AttributeValueMemberS{Value: "ok"},
		},
		Limit:             &limit,
		ExclusiveStartKey: startKey,
		ScanIndexForward:  &scanForward,
		ConsistentRead:    &consistent,
	}

	var out []struct {
		ID string `dynamodb:"id"`
	}
	result, err := exec.ExecuteQueryWithPagination(input, &out)
	require.NoError(t, err)
	require.Len(t, out, 2)
	require.Equal(t, "u1", out[0].ID)
	require.Equal(t, "u2", out[1].ID)
	require.Equal(t, int64(2), result.Count)
	require.Equal(t, int64(2), result.ScannedCount)

	require.NotNil(t, client.queryInput)
	require.NotNil(t, client.queryInput.IndexName)
	require.Equal(t, "byStatus", *client.queryInput.IndexName)
	require.NotNil(t, client.queryInput.Limit)
	require.Equal(t, int32(2), *client.queryInput.Limit)
	require.NotNil(t, client.queryInput.ScanIndexForward)
	require.Equal(t, false, *client.queryInput.ScanIndexForward)
	require.NotNil(t, client.queryInput.ConsistentRead)
	require.Equal(t, true, *client.queryInput.ConsistentRead)
}

func TestMainExecutor_ExecuteScanWithPagination_COV5(t *testing.T) {
	exec := NewExecutor(&cov5DynamoDBClient{}, context.Background())

	var dest []struct {
		ID string `dynamodb:"id"`
	}
	_, err := exec.ExecuteScanWithPagination(nil, &dest)
	require.Error(t, err)

	limit := int32(1)
	segment := int32(0)
	totalSegments := int32(2)
	consistent := true
	startKey := map[string]types.AttributeValue{
		"id": &types.AttributeValueMemberS{Value: "start"},
	}

	client := &cov5DynamoDBClient{
		scanOutput: &dynamodb.ScanOutput{
			Items: []map[string]types.AttributeValue{
				{"id": &types.AttributeValueMemberS{Value: "u1"}},
			},
			ScannedCount:     1,
			LastEvaluatedKey: startKey,
		},
	}
	exec = NewExecutor(client, context.Background())

	input := &core.CompiledQuery{
		TableName:            "tbl",
		IndexName:            "byStatus",
		FilterExpression:     "#st = :st",
		ProjectionExpression: "#id",
		ExpressionAttributeNames: map[string]string{
			"#id": "id",
			"#st": "status",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":st": &types.AttributeValueMemberS{Value: "ok"},
		},
		Limit:             &limit,
		ExclusiveStartKey: startKey,
		Segment:           &segment,
		TotalSegments:     &totalSegments,
		ConsistentRead:    &consistent,
	}

	var out []struct {
		ID string `dynamodb:"id"`
	}
	result, err := exec.ExecuteScanWithPagination(input, &out)
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Equal(t, "u1", out[0].ID)
	require.Equal(t, int64(1), result.Count)
	require.Equal(t, int64(1), result.ScannedCount)

	require.NotNil(t, client.scanInput)
	require.NotNil(t, client.scanInput.IndexName)
	require.Equal(t, "byStatus", *client.scanInput.IndexName)
	require.NotNil(t, client.scanInput.Segment)
	require.Equal(t, int32(0), *client.scanInput.Segment)
	require.NotNil(t, client.scanInput.TotalSegments)
	require.Equal(t, int32(2), *client.scanInput.TotalSegments)
	require.NotNil(t, client.scanInput.ConsistentRead)
	require.Equal(t, true, *client.scanInput.ConsistentRead)
}

func TestMainExecutor_ExecuteUpdateItemWithResult_COV5(t *testing.T) {
	exec := NewExecutor(&cov5DynamoDBClient{}, context.Background())
	_, err := exec.ExecuteUpdateItemWithResult(nil, map[string]types.AttributeValue{"id": &types.AttributeValueMemberS{Value: "u1"}})
	require.Error(t, err)

	client := &cov5DynamoDBClient{
		updateOutput: &dynamodb.UpdateItemOutput{
			Attributes: map[string]types.AttributeValue{
				"id": &types.AttributeValueMemberS{Value: "u1"},
			},
		},
	}
	exec = NewExecutor(client, context.Background())

	input := &core.CompiledQuery{
		TableName:        "tbl",
		UpdateExpression: "SET #id = :id",
		ExpressionAttributeNames: map[string]string{
			"#id": "id",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":id": &types.AttributeValueMemberS{Value: "u1"},
		},
	}

	result, err := exec.ExecuteUpdateItemWithResult(input, map[string]types.AttributeValue{"id": &types.AttributeValueMemberS{Value: "u1"}})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Contains(t, result.Attributes, "id")
	require.NotNil(t, client.updateInput)
	require.NotNil(t, client.updateInput.ReturnValues)
	require.Equal(t, types.ReturnValueAllNew, client.updateInput.ReturnValues)
}
