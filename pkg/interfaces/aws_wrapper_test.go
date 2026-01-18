package interfaces

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"
)

type failingRoundTripper struct {
	err error
}

func (r failingRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, r.err
}

func newFailingDynamoDBClient(t *testing.T) *dynamodb.Client {
	t.Helper()

	cfg := aws.Config{
		Region:      "us-east-1",
		Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider("AKID", "SECRET", "")),
		HTTPClient: &http.Client{
			Transport: failingRoundTripper{err: errors.New("boom")},
		},
		Retryer: func() aws.Retryer {
			return aws.NopRetryer{}
		},
	}

	return dynamodb.NewFromConfig(cfg)
}

func TestDynamoDBClientWrapper_ForwardsCalls(t *testing.T) {
	client := newFailingDynamoDBClient(t)
	wrapper := NewDynamoDBClientWrapper(client)

	ctx := context.Background()

	_, err := wrapper.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName:   aws.String("T"),
		BillingMode: types.BillingModePayPerRequest,
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("pk"), KeyType: types.KeyTypeHash},
		},
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("pk"), AttributeType: types.ScalarAttributeTypeS},
		},
	})
	require.Error(t, err)

	_, err = wrapper.DescribeTable(ctx, &dynamodb.DescribeTableInput{TableName: aws.String("T")})
	require.Error(t, err)

	_, err = wrapper.DeleteTable(ctx, &dynamodb.DeleteTableInput{TableName: aws.String("T")})
	require.Error(t, err)

	_, err = wrapper.UpdateTimeToLive(ctx, &dynamodb.UpdateTimeToLiveInput{
		TableName: aws.String("T"),
		TimeToLiveSpecification: &types.TimeToLiveSpecification{
			AttributeName: aws.String("ttl"),
			Enabled:       aws.Bool(true),
		},
	})
	require.Error(t, err)

	_, err = wrapper.Scan(ctx, &dynamodb.ScanInput{TableName: aws.String("T")})
	require.Error(t, err)

	_, err = wrapper.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String("T"),
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: "1"},
		},
	})
	require.Error(t, err)

	_, err = wrapper.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String("T"),
		Item: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: "1"},
		},
	})
	require.Error(t, err)

	_, err = wrapper.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String("T"),
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: "1"},
		},
	})
	require.Error(t, err)

	_, err = wrapper.Query(ctx, &dynamodb.QueryInput{
		TableName: aws.String("T"),
	})
	require.Error(t, err)

	_, err = wrapper.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String("T"),
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: "1"},
		},
		UpdateExpression: aws.String("SET #a = :v"),
		ExpressionAttributeNames: map[string]string{
			"#a": "a",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":v": &types.AttributeValueMemberS{Value: "v"},
		},
	})
	require.Error(t, err)

	_, err = wrapper.BatchGetItem(ctx, &dynamodb.BatchGetItemInput{
		RequestItems: map[string]types.KeysAndAttributes{
			"T": {
				Keys: []map[string]types.AttributeValue{
					{"pk": &types.AttributeValueMemberS{Value: "1"}},
				},
			},
		},
	})
	require.Error(t, err)

	_, err = wrapper.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{
		RequestItems: map[string][]types.WriteRequest{
			"T": {
				{
					PutRequest: &types.PutRequest{
						Item: map[string]types.AttributeValue{
							"pk": &types.AttributeValueMemberS{Value: "1"},
						},
					},
				},
			},
		},
	})
	require.Error(t, err)
}

func TestTableWaiterWrapper_Constructors(t *testing.T) {
	client := newFailingDynamoDBClient(t)

	existsWaiter := NewTableExistsWaiterWrapper(client)
	require.NotNil(t, existsWaiter)

	notExistsWaiter := NewTableNotExistsWaiterWrapper(client)
	require.NotNil(t, notExistsWaiter)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := existsWaiter.Wait(ctx, &dynamodb.DescribeTableInput{TableName: aws.String("T")}, 1*time.Second)
	require.Error(t, err)

	err = notExistsWaiter.Wait(ctx, &dynamodb.DescribeTableInput{TableName: aws.String("T")}, 1*time.Second)
	require.Error(t, err)
}
