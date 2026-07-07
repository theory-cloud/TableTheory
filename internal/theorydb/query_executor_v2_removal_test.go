package theorydb

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v2/pkg/core"
	customerrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	"github.com/theory-cloud/tabletheory/v2/pkg/session"
)

func TestQueryExecutor_ProductionReadStackAfterMainExecutorRemoval_V2(t *testing.T) {
	httpClient := newCapturingHTTPClient(map[string]string{
		"DynamoDB_20120810.Query":   `{"Items":[{"id":{"S":"u1"}}],"Count":1,"ScannedCount":2,"LastEvaluatedKey":{"id":{"S":"u1"}}}`,
		"DynamoDB_20120810.GetItem": `{"Item":{"id":{"S":"u1"}}}`,
	})

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	executor := &queryExecutor{db: mustDB(t, dbAny), ctx: context.Background()}

	limit := int32(1)
	consistentRead := true
	queryInput := &core.CompiledQuery{
		TableName:                 "TestTable",
		IndexName:                 "by-id",
		KeyConditionExpression:    "#id = :id",
		ExpressionAttributeNames:  map[string]string{"#id": "id"},
		ExpressionAttributeValues: map[string]types.AttributeValue{":id": &types.AttributeValueMemberS{Value: "u1"}},
		Limit:                     &limit,
		ConsistentRead:            &consistentRead,
	}

	var queried []map[string]types.AttributeValue
	queryResult, err := executor.ExecuteQueryWithPagination(queryInput, &queried)
	require.NoError(t, err)
	require.Len(t, queried, 1)
	require.Equal(t, int64(1), queryResult.Count)
	require.Equal(t, int64(2), queryResult.ScannedCount)
	require.Contains(t, queryResult.LastEvaluatedKey, "id")

	var got map[string]types.AttributeValue
	err = executor.ExecuteGetItem(&core.CompiledQuery{
		TableName:            "TestTable",
		ProjectionExpression: "id",
		ConsistentRead:       &consistentRead,
	}, map[string]types.AttributeValue{
		"id": &types.AttributeValueMemberS{Value: "u1"},
	}, &got)
	require.NoError(t, err)
	require.Contains(t, got, "id")

	queryReq := findRequestByTarget(httpClient.Requests(), "DynamoDB_20120810.Query")
	require.NotNil(t, queryReq)
	require.Equal(t, "TestTable", queryReq.Payload["TableName"])
	require.Equal(t, "by-id", queryReq.Payload["IndexName"])
	require.Equal(t, float64(1), queryReq.Payload["Limit"])
	require.Equal(t, true, queryReq.Payload["ConsistentRead"])

	getReq := findRequestByTarget(httpClient.Requests(), "DynamoDB_20120810.GetItem")
	require.NotNil(t, getReq)
	require.Equal(t, "TestTable", getReq.Payload["TableName"])
	require.Equal(t, "id", getReq.Payload["ProjectionExpression"])
	require.Equal(t, true, getReq.Payload["ConsistentRead"])
}

func TestQueryExecutor_ProductionGetItemNotFoundAfterMainExecutorRemoval_V2(t *testing.T) {
	httpClient := newCapturingHTTPClient(map[string]string{
		"DynamoDB_20120810.GetItem": `{"Item":null}`,
	})

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	executor := &queryExecutor{db: mustDB(t, dbAny), ctx: context.Background()}

	var got map[string]types.AttributeValue
	err = executor.ExecuteGetItem(&core.CompiledQuery{TableName: "TestTable"}, map[string]types.AttributeValue{
		"id": &types.AttributeValueMemberS{Value: "missing"},
	}, &got)
	require.ErrorIs(t, err, customerrors.ErrItemNotFound)
}
