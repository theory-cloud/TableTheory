package schema

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestManager_copyTable_RefusesExistingTarget_COV5(t *testing.T) {
	httpClient := newCapturingHTTPClient(map[string]string{
		"DynamoDB_20120810.DescribeTable": `{"Table":{"TableName":"backup","TableStatus":"ACTIVE","BillingModeSummary":{"BillingMode":"PAY_PER_REQUEST"}}}`,
	})

	mgr := newTestManager(t, httpClient)
	require.ErrorContains(t, mgr.copyTable(context.Background(), "source", "backup"), "refusing to delete")
	require.Equal(t, 0, countRequestsByTarget(httpClient.Requests(), "DynamoDB_20120810.DeleteTable"))
}

func TestManager_copyTable_RefusesSourceAsTarget_COV5(t *testing.T) {
	httpClient := newCapturingHTTPClient(nil)

	mgr := newTestManager(t, httpClient)
	require.ErrorContains(t, mgr.copyTable(context.Background(), "source", "source"), "must differ")
	require.Equal(t, 0, countRequestsByTarget(httpClient.Requests(), "DynamoDB_20120810.DeleteTable"))
}

func TestManager_createBackup_RefusesSourceAsBackupName_COV5(t *testing.T) {
	httpClient := newCapturingHTTPClient(nil)

	mgr := newTestManager(t, httpClient)
	require.ErrorContains(t, mgr.createBackup(context.Background(), "source", "source"), "must differ")
	require.Empty(t, httpClient.Requests())
}
