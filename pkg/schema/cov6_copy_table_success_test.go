package schema

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestManager_copyTable_CreatesTargetWithIndexesAndThroughput_COV6(t *testing.T) {
	httpClient := newCapturingHTTPClient(map[string]string{
		"DynamoDB_20120810.CreateTable": `{}`,
	})

	httpClient.SetResponseSequence("DynamoDB_20120810.DescribeTable", []stubbedResponse{
		{
			status:  400,
			headers: map[string]string{"X-Amzn-ErrorType": "ResourceNotFoundException"},
			body:    `{"__type":"com.amazonaws.dynamodb.v20120810#ResourceNotFoundException","message":"not found"}`,
		},
		{
			body: `{
  "Table": {
    "TableName": "source",
    "TableStatus": "ACTIVE",
    "KeySchema": [{"AttributeName": "pk", "KeyType": "HASH"}],
    "AttributeDefinitions": [
      {"AttributeName": "pk", "AttributeType": "S"},
      {"AttributeName": "sk", "AttributeType": "S"},
      {"AttributeName": "gpk", "AttributeType": "S"}
    ],
    "BillingModeSummary": {"BillingMode": "PROVISIONED"},
    "ProvisionedThroughput": {"ReadCapacityUnits": 5, "WriteCapacityUnits": 5},
    "GlobalSecondaryIndexes": [{
      "IndexName": "gsi1",
      "KeySchema": [{"AttributeName": "gpk", "KeyType": "HASH"}],
      "Projection": {"ProjectionType": "ALL"},
      "ProvisionedThroughput": {"ReadCapacityUnits": 5, "WriteCapacityUnits": 5}
    }],
    "LocalSecondaryIndexes": [{
      "IndexName": "lsi1",
      "KeySchema": [{"AttributeName": "pk", "KeyType": "HASH"}, {"AttributeName": "sk", "KeyType": "RANGE"}],
      "Projection": {"ProjectionType": "ALL"}
    }]
  }
}`,
		},
		{
			body: `{"Table":{"TableName":"target","TableStatus":"ACTIVE","BillingModeSummary":{"BillingMode":"PROVISIONED"}}}`,
		},
	})

	mgr := newTestManager(t, httpClient)
	require.NoError(t, mgr.copyTable(context.Background(), "source", "target"))

	reqs := httpClient.Requests()
	require.Equal(t, 1, countRequestsByTarget(reqs, "DynamoDB_20120810.CreateTable"))

	var createPayload map[string]any
	for _, req := range reqs {
		if req.Target == "DynamoDB_20120810.CreateTable" {
			createPayload = req.Payload
			break
		}
	}
	require.NotNil(t, createPayload)
	require.Contains(t, createPayload, "GlobalSecondaryIndexes")
	require.Contains(t, createPayload, "LocalSecondaryIndexes")
	require.Contains(t, createPayload, "ProvisionedThroughput")
}
