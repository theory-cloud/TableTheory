package schema

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/model"
)

type cov6ManagerModel struct {
	ID string `theorydb:"pk"`
}

func (cov6ManagerModel) TableName() string { return "tbl" }

type cov6ManagerTwoGSIsModel struct {
	PK   string `theorydb:"pk"`
	GSI1 string `theorydb:"index:one,pk"`
	GSI2 string `theorydb:"index:two,pk"`
}

func (cov6ManagerTwoGSIsModel) TableName() string { return "tbl" }

func TestManager_ClientErrorsAreWrapped_COV6(t *testing.T) {
	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&cov6ManagerModel{}))

	mgr := NewManager(nil, registry)

	exists, err := mgr.TableExists("tbl")
	require.ErrorContains(t, err, "failed to get client for table exists check")
	require.False(t, exists)

	require.ErrorContains(t, mgr.DeleteTable("tbl"), "failed to get client for table deletion")
	require.ErrorContains(t, mgr.waitForTableActive("tbl"), "failed to get client for table waiter")

	_, err = mgr.DescribeTable(&cov6ManagerModel{})
	require.ErrorContains(t, err, "failed to get client for table description")

	require.ErrorContains(t, mgr.CreateTable(&cov6ManagerModel{}), "failed to get client for table creation")
	require.ErrorContains(t, mgr.UpdateTable(&cov6ManagerModel{}), "failed to get client for table description")

	require.ErrorContains(
		t,
		mgr.BatchUpdateTable(&cov6ManagerModel{}, []TableOption{WithBillingMode(types.BillingModePayPerRequest)}),
		"batch update failed at step 1",
	)
}

func TestManager_CreateTable_IgnoresResourceInUse_COV6(t *testing.T) {
	httpClient := newCapturingHTTPClient(nil)
	httpClient.SetResponseSequence("DynamoDB_20120810.CreateTable", []stubbedResponse{
		stubbedAWSError("ResourceInUseException", "exists"),
	})

	mgr := newTestManager(t, httpClient)
	require.NoError(t, mgr.registry.Register(&cov6ManagerModel{}))

	require.NoError(t, mgr.CreateTable(&cov6ManagerModel{}))

	reqs := httpClient.Requests()
	require.Equal(t, 1, countRequestsByTarget(reqs, "DynamoDB_20120810.CreateTable"))
}

func TestManager_CreateTable_WrapsCreateErrors_COV6(t *testing.T) {
	httpClient := newCapturingHTTPClient(nil)
	httpClient.SetResponseSequence("DynamoDB_20120810.CreateTable", []stubbedResponse{
		stubbedAWSError("ValidationException", "boom"),
	})

	mgr := newTestManager(t, httpClient)
	require.NoError(t, mgr.registry.Register(&cov6ManagerModel{}))

	err := mgr.CreateTable(&cov6ManagerModel{})
	require.ErrorContains(t, err, "failed to create table")
}

func TestManager_TableExists_NotFoundAndOtherErrors_COV6(t *testing.T) {
	httpClient := newCapturingHTTPClient(nil)
	httpClient.SetResponseSequence("DynamoDB_20120810.DescribeTable", []stubbedResponse{
		stubbedAWSError("ResourceNotFoundException", "not found"),
		stubbedAWSError("ValidationException", "boom"),
	})

	mgr := newTestManager(t, httpClient)

	exists, err := mgr.TableExists("tbl")
	require.NoError(t, err)
	require.False(t, exists)

	_, err = mgr.TableExists("tbl")
	require.Error(t, err)
}

func TestManager_DescribeTable_WrapsDescribeErrors_COV6(t *testing.T) {
	httpClient := newCapturingHTTPClient(nil)
	httpClient.SetResponseSequence("DynamoDB_20120810.DescribeTable", []stubbedResponse{
		stubbedAWSError("ValidationException", "boom"),
	})

	mgr := newTestManager(t, httpClient)
	require.NoError(t, mgr.registry.Register(&cov6ManagerModel{}))

	_, err := mgr.DescribeTable(&cov6ManagerModel{})
	require.ErrorContains(t, err, "failed to describe table")
}

func TestManager_UpdateTable_WrapsUpdateErrors_COV6(t *testing.T) {
	httpClient := newCapturingHTTPClient(map[string]string{
		"DynamoDB_20120810.DescribeTable": `{"Table":{"TableName":"tbl","TableStatus":"ACTIVE","BillingModeSummary":{"BillingMode":"PAY_PER_REQUEST"}}}`,
	})
	httpClient.SetResponseSequence("DynamoDB_20120810.UpdateTable", []stubbedResponse{
		stubbedAWSError("ValidationException", "boom"),
	})

	mgr := newTestManager(t, httpClient)
	require.NoError(t, mgr.registry.Register(&cov6ManagerModel{}))

	err := mgr.UpdateTable(&cov6ManagerModel{}, WithBillingMode(types.BillingModePayPerRequest))
	require.ErrorContains(t, err, "failed to update table")
}

func TestManager_DeleteTable_SuccessAndDeleteErrors_COV6(t *testing.T) {
	t.Run("success returns when waiter sees not found", func(t *testing.T) {
		httpClient := newCapturingHTTPClient(nil)
		httpClient.SetResponseSequence("DynamoDB_20120810.DeleteTable", []stubbedResponse{
			{body: `{}`},
		})
		httpClient.SetResponseSequence("DynamoDB_20120810.DescribeTable", []stubbedResponse{
			stubbedAWSError("ResourceNotFoundException", "not found"),
		})

		mgr := newTestManager(t, httpClient)
		require.NoError(t, mgr.DeleteTable("tbl"))
	})

	t.Run("wraps delete errors", func(t *testing.T) {
		httpClient := newCapturingHTTPClient(nil)
		httpClient.SetResponseSequence("DynamoDB_20120810.DeleteTable", []stubbedResponse{
			stubbedAWSError("ValidationException", "boom"),
		})

		mgr := newTestManager(t, httpClient)
		require.ErrorContains(t, mgr.DeleteTable("tbl"), "failed to delete table")
	})
}

func TestManager_MetadataErrorsAreWrapped_COV6(t *testing.T) {
	registry := model.NewRegistry()
	mgr := NewManager(nil, registry)

	require.ErrorContains(t, mgr.CreateTable(&cov6ManagerModel{}), "failed to get model metadata")

	_, err := mgr.DescribeTable(&cov6ManagerModel{})
	require.ErrorContains(t, err, "failed to get model metadata")

	require.ErrorContains(t, mgr.UpdateTable(&cov6ManagerModel{}), "failed to get model metadata")
}

func TestManager_UpdateTable_RejectsMultipleGSIChanges_COV6(t *testing.T) {
	httpClient := newCapturingHTTPClient(map[string]string{
		"DynamoDB_20120810.DescribeTable": `{"Table":{"TableName":"tbl","TableStatus":"ACTIVE","BillingModeSummary":{"BillingMode":"PAY_PER_REQUEST"}}}`,
	})

	mgr := newTestManager(t, httpClient)
	require.NoError(t, mgr.registry.Register(&cov6ManagerTwoGSIsModel{}))

	err := mgr.UpdateTable(&cov6ManagerTwoGSIsModel{})
	require.ErrorContains(t, err, "multiple GSI changes detected")
}
