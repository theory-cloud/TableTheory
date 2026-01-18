package schema

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestManager_copyTable_ErrorsWhenDeletingExistingBackupFails_COV5(t *testing.T) {
	httpClient := newCapturingHTTPClient(map[string]string{
		"DynamoDB_20120810.DescribeTable": `{"Table":{"TableName":"backup","TableStatus":"ACTIVE","BillingModeSummary":{"BillingMode":"PAY_PER_REQUEST"}}}`,
	})
	httpClient.SetResponseSequence("DynamoDB_20120810.DeleteTable", []stubbedResponse{
		{status: 400, body: `{"__type":"ValidationException","message":"boom"}`},
	})

	mgr := newTestManager(t, httpClient)
	require.Error(t, mgr.copyTable(context.Background(), "source", "backup"))
	require.Equal(t, 1, countRequestsByTarget(httpClient.Requests(), "DynamoDB_20120810.DeleteTable"))
}
