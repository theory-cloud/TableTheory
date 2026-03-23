package schema

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/model"
)

type ttlSchemaModel struct {
	ID        string `theorydb:"pk"`
	ExpiresAt int64  `theorydb:"ttl"`
}

func (ttlSchemaModel) TableName() string { return "ttl_records" }

type noTTLModel struct {
	ID string `theorydb:"pk"`
}

func (noTTLModel) TableName() string { return "no_ttl_records" }

func TestManager_CreateTable_EnablesTTLFromModel_COV6(t *testing.T) {
	httpClient := newCapturingHTTPClient(nil)
	httpClient.SetResponseSequence("DynamoDB_20120810.CreateTable", []stubbedResponse{
		{body: `{}`},
	})
	httpClient.SetResponseSequence("DynamoDB_20120810.DescribeTable", []stubbedResponse{
		{body: `{"Table":{"TableName":"ttl_records","TableStatus":"ACTIVE","BillingModeSummary":{"BillingMode":"PAY_PER_REQUEST"}}}`},
	})
	httpClient.SetResponseSequence("DynamoDB_20120810.UpdateTimeToLive", []stubbedResponse{
		{body: `{"TimeToLiveSpecification":{"AttributeName":"expiresAt","Enabled":true}}`},
	})

	mgr := newTestManager(t, httpClient)
	require.NoError(t, mgr.registry.Register(&ttlSchemaModel{}))

	require.NoError(t, mgr.CreateTable(&ttlSchemaModel{}))

	reqs := httpClient.Requests()
	require.Equal(t, 1, countRequestsByTarget(reqs, "DynamoDB_20120810.CreateTable"))
	require.Equal(t, 1, countRequestsByTarget(reqs, "DynamoDB_20120810.UpdateTimeToLive"))
	require.Equal(t, map[string]any{
		"AttributeName": "expiresAt",
		"Enabled":       true,
	}, reqs[len(reqs)-1].Payload["TimeToLiveSpecification"])
}

func TestManager_CreateTable_ExistingTableStillSyncsTTL_COV6(t *testing.T) {
	httpClient := newCapturingHTTPClient(nil)
	httpClient.SetResponseSequence("DynamoDB_20120810.CreateTable", []stubbedResponse{
		stubbedAWSError("ResourceInUseException", "exists"),
	})
	httpClient.SetResponseSequence("DynamoDB_20120810.UpdateTimeToLive", []stubbedResponse{
		{body: `{"TimeToLiveSpecification":{"AttributeName":"expiresAt","Enabled":true}}`},
	})

	mgr := newTestManager(t, httpClient)
	require.NoError(t, mgr.registry.Register(&ttlSchemaModel{}))

	require.NoError(t, mgr.CreateTable(&ttlSchemaModel{}))

	reqs := httpClient.Requests()
	require.Equal(t, 1, countRequestsByTarget(reqs, "DynamoDB_20120810.CreateTable"))
	require.Equal(t, 1, countRequestsByTarget(reqs, "DynamoDB_20120810.UpdateTimeToLive"))
}

func TestManager_UpdateTable_SyncsTTLWithoutOtherTableChanges_COV6(t *testing.T) {
	httpClient := newCapturingHTTPClient(map[string]string{
		"DynamoDB_20120810.DescribeTable": `{"Table":{"TableName":"ttl_records","TableStatus":"ACTIVE","BillingModeSummary":{"BillingMode":"PAY_PER_REQUEST"}}}`,
	})
	httpClient.SetResponseSequence("DynamoDB_20120810.UpdateTimeToLive", []stubbedResponse{
		{body: `{"TimeToLiveSpecification":{"AttributeName":"expiresAt","Enabled":true}}`},
	})

	mgr := newTestManager(t, httpClient)
	require.NoError(t, mgr.registry.Register(&ttlSchemaModel{}))

	require.NoError(t, mgr.UpdateTable(&ttlSchemaModel{}))

	reqs := httpClient.Requests()
	require.Equal(t, 0, countRequestsByTarget(reqs, "DynamoDB_20120810.UpdateTable"))
	require.Equal(t, 1, countRequestsByTarget(reqs, "DynamoDB_20120810.UpdateTimeToLive"))
}

func TestManager_EnableTTL_ValidatesTTLField_COV6(t *testing.T) {
	mgr := &Manager{registry: modelRegistryWith(t, &noTTLModel{})}

	err := mgr.EnableTTL(&noTTLModel{})
	require.ErrorContains(t, err, "does not define a ttl field")
}

func modelRegistryWith(t *testing.T, values ...any) *model.Registry {
	t.Helper()

	registry := model.NewRegistry()
	for _, value := range values {
		require.NoError(t, registry.Register(value))
	}
	return registry
}
