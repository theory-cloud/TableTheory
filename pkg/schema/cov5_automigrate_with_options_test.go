package schema

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type cov5AutoMigrateSource struct {
	ID string `theorydb:"pk,attr:id"`
}

func (cov5AutoMigrateSource) TableName() string { return "source" }

type cov5AutoMigrateTarget struct {
	ID string `theorydb:"pk,attr:id"`
}

func (cov5AutoMigrateTarget) TableName() string { return "target" }

func TestManager_AutoMigrateWithOptions_BackupTargetAndCopyData(t *testing.T) {
	httpClient := newCapturingHTTPClient(map[string]string{
		"DynamoDB_20120810.CreateBackup": `{}`,
	})
	httpClient.SetResponseSequence("DynamoDB_20120810.DescribeTable", []stubbedResponse{
		{body: `{"Table":{"TableName":"source","TableStatus":"ACTIVE","BillingModeSummary":{"BillingMode":"PAY_PER_REQUEST"}}}`},
		{body: `{"Table":{"TableName":"target","TableStatus":"ACTIVE","BillingModeSummary":{"BillingMode":"PAY_PER_REQUEST"}}}`},
	})
	httpClient.SetResponseSequence("DynamoDB_20120810.Scan", []stubbedResponse{
		{body: `{"Items":[{"id":{"S":"1"}}],"Count":1,"ScannedCount":1}`},
	})

	mgr := newTestManager(t, httpClient)

	source := &cov5AutoMigrateSource{ID: "1"}
	target := &cov5AutoMigrateTarget{}

	require.NoError(t, mgr.AutoMigrateWithOptions(
		source,
		WithContext(context.Background()),
		WithBackupTable("backup"),
		WithTargetModel(target),
		WithDataCopy(true),
		WithBatchSize(1),
	))

	reqs := httpClient.Requests()
	require.Equal(t, 1, countRequestsByTarget(reqs, "DynamoDB_20120810.CreateBackup"))
	require.GreaterOrEqual(t, countRequestsByTarget(reqs, "DynamoDB_20120810.Scan"), 1)
}
