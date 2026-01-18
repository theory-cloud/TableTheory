package schema

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/model"
)

func TestManager_copyData_ScansAndProcessesItems(t *testing.T) {
	httpClient := newCapturingHTTPClient(nil)
	httpClient.SetResponseSequence("DynamoDB_20120810.Scan", []stubbedResponse{
		{body: `{"Items":[{"pk":{"S":"1"}}],"Count":1,"ScannedCount":1,"LastEvaluatedKey":{"pk":{"S":"1"}}}`},
		{body: `{"Items":[{"pk":{"S":"2"}}],"Count":1,"ScannedCount":1}`},
	})

	mgr := newTestManager(t, httpClient)

	opts := &AutoMigrateOptions{
		Context:   context.Background(),
		BatchSize: 1,
	}

	sourceMeta := &model.Metadata{TableName: "source"}
	targetMeta := &model.Metadata{TableName: "target"}

	require.NoError(t, mgr.copyData(opts, sourceMeta, targetMeta, nil))

	reqs := httpClient.Requests()
	require.Equal(t, 2, countRequestsByTarget(reqs, "DynamoDB_20120810.Scan"))
	require.GreaterOrEqual(t, countRequestsByTarget(reqs, "DynamoDB_20120810.BatchWriteItem"), 1)
}
