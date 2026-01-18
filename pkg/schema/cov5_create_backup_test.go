package schema

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestManager_createBackup_CallsCreateBackupWhenTableExists(t *testing.T) {
	httpClient := newCapturingHTTPClient(map[string]string{
		"DynamoDB_20120810.DescribeTable": `{"Table":{"TableName":"source","TableStatus":"ACTIVE","BillingModeSummary":{"BillingMode":"PAY_PER_REQUEST"}}}`,
		"DynamoDB_20120810.CreateBackup":  `{}`,
	})

	mgr := newTestManager(t, httpClient)

	require.NoError(t, mgr.createBackup(context.Background(), "source", "backup"))

	reqs := httpClient.Requests()
	require.Equal(t, 1, countRequestsByTarget(reqs, "DynamoDB_20120810.CreateBackup"))
}
