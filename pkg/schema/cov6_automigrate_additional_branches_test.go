package schema

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/model"
)

func stubbedAWSError(errorType, message string) stubbedResponse {
	return stubbedResponse{
		status: http.StatusBadRequest,
		headers: map[string]string{
			"X-Amzn-ErrorType": errorType,
		},
		body: fmt.Sprintf(`{"__type":"com.amazonaws.dynamodb.v20120810#%s","message":"%s"}`, errorType, message),
	}
}

type cov6EnsureTargetModel struct {
	ID string `theorydb:"pk"`
}

func (cov6EnsureTargetModel) TableName() string { return "tbl" }

func TestManager_ensureTargetTable_ExistsSkipsCreate_COV6(t *testing.T) {
	httpClient := newCapturingHTTPClient(map[string]string{
		"DynamoDB_20120810.DescribeTable": `{"Table":{"TableName":"tbl","TableStatus":"ACTIVE","BillingModeSummary":{"BillingMode":"PAY_PER_REQUEST"}}}`,
	})

	mgr := newTestManager(t, httpClient)
	model := &cov6EnsureTargetModel{ID: "1"}
	require.NoError(t, mgr.registry.Register(model))

	require.NoError(t, mgr.ensureTargetTable(model, "tbl"))

	reqs := httpClient.Requests()
	require.Equal(t, 1, countRequestsByTarget(reqs, "DynamoDB_20120810.DescribeTable"))
	require.Equal(t, 0, countRequestsByTarget(reqs, "DynamoDB_20120810.CreateTable"))
}

func TestManager_ensureTargetTable_CreatesWhenMissing_COV6(t *testing.T) {
	httpClient := newCapturingHTTPClient(nil)
	httpClient.SetResponseSequence("DynamoDB_20120810.DescribeTable", []stubbedResponse{
		stubbedAWSError("ResourceNotFoundException", "not found"),
		{body: `{"Table":{"TableName":"tbl","TableStatus":"ACTIVE","BillingModeSummary":{"BillingMode":"PAY_PER_REQUEST"}}}`},
	})

	mgr := newTestManager(t, httpClient)
	model := &cov6EnsureTargetModel{ID: "1"}
	require.NoError(t, mgr.registry.Register(model))

	require.NoError(t, mgr.ensureTargetTable(model, "tbl"))

	reqs := httpClient.Requests()
	require.Equal(t, 2, countRequestsByTarget(reqs, "DynamoDB_20120810.DescribeTable"))
	require.Equal(t, 1, countRequestsByTarget(reqs, "DynamoDB_20120810.CreateTable"))
}

func TestManager_ensureTargetTable_WrapsTableExistsError_COV6(t *testing.T) {
	httpClient := newCapturingHTTPClient(nil)
	httpClient.SetResponseSequence("DynamoDB_20120810.DescribeTable", []stubbedResponse{
		stubbedAWSError("ValidationException", "boom"),
	})

	mgr := newTestManager(t, httpClient)
	model := &cov6EnsureTargetModel{ID: "1"}
	require.NoError(t, mgr.registry.Register(model))

	err := mgr.ensureTargetTable(model, "tbl")
	require.ErrorContains(t, err, "failed to check target table existence")
}

func TestManager_ensureTargetTable_WrapsCreateTableError_COV6(t *testing.T) {
	httpClient := newCapturingHTTPClient(nil)
	httpClient.SetResponseSequence("DynamoDB_20120810.DescribeTable", []stubbedResponse{
		stubbedAWSError("ResourceNotFoundException", "not found"),
	})
	httpClient.SetResponseSequence("DynamoDB_20120810.CreateTable", []stubbedResponse{
		stubbedAWSError("ValidationException", "create failed"),
	})

	mgr := newTestManager(t, httpClient)
	model := &cov6EnsureTargetModel{ID: "1"}
	require.NoError(t, mgr.registry.Register(model))

	err := mgr.ensureTargetTable(model, "tbl")
	require.ErrorContains(t, err, "failed to create target table")
}

func TestManager_copyDataIfRequested_TransformValidationAndEarlyReturns_COV6(t *testing.T) {
	mgr := &Manager{registry: model.NewRegistry()}

	sourceMeta := &model.Metadata{TableName: "source"}
	targetMeta := &model.Metadata{TableName: "target"}

	opts := &AutoMigrateOptions{DataCopy: false, Context: context.Background(), BatchSize: 25}
	require.NoError(t, mgr.copyDataIfRequested(opts, sourceMeta, targetMeta))

	opts = &AutoMigrateOptions{DataCopy: true, Context: context.Background(), BatchSize: 25}
	require.NoError(t, mgr.copyDataIfRequested(opts, sourceMeta, sourceMeta))

	opts = &AutoMigrateOptions{DataCopy: true, Context: context.Background(), BatchSize: 25, Transform: "not-a-function"}
	err := mgr.copyDataIfRequested(opts, sourceMeta, targetMeta)
	require.ErrorContains(t, err, "invalid transform function")
}

func TestManager_createBackup_FallsBackToCopyTable_COV6(t *testing.T) {
	httpClient := newCapturingHTTPClient(nil)
	httpClient.SetResponseSequence("DynamoDB_20120810.DescribeTable", []stubbedResponse{
		{body: `{"Table":{"TableName":"source","TableStatus":"ACTIVE","BillingModeSummary":{"BillingMode":"PAY_PER_REQUEST"}}}`},
		stubbedAWSError("ResourceNotFoundException", "not found"),
		{body: `{"Table":{"TableName":"source","TableStatus":"ACTIVE","BillingModeSummary":{"BillingMode":"PAY_PER_REQUEST"},"KeySchema":[{"AttributeName":"id","KeyType":"HASH"}],"AttributeDefinitions":[{"AttributeName":"id","AttributeType":"S"}]}}`},
		{body: `{"Table":{"TableName":"backup","TableStatus":"ACTIVE","BillingModeSummary":{"BillingMode":"PAY_PER_REQUEST"}}}`},
	})
	httpClient.SetResponseSequence("DynamoDB_20120810.CreateBackup", []stubbedResponse{
		{err: fmt.Errorf("backup failed")},
	})

	mgr := newTestManager(t, httpClient)

	require.NoError(t, mgr.createBackup(context.Background(), "source", "backup"))

	reqs := httpClient.Requests()
	require.Equal(t, 1, countRequestsByTarget(reqs, "DynamoDB_20120810.CreateBackup"))
	require.Equal(t, 1, countRequestsByTarget(reqs, "DynamoDB_20120810.CreateTable"))
	require.Equal(t, 1, countRequestsByTarget(reqs, "DynamoDB_20120810.Scan"))
}

func TestManager_createBackup_ErrorsWhenSourceMissing_COV6(t *testing.T) {
	httpClient := newCapturingHTTPClient(nil)
	httpClient.SetResponseSequence("DynamoDB_20120810.DescribeTable", []stubbedResponse{
		stubbedAWSError("ResourceNotFoundException", "not found"),
	})

	mgr := newTestManager(t, httpClient)

	err := mgr.createBackup(context.Background(), "source", "backup")
	require.ErrorContains(t, err, "does not exist")
}

func TestSleepWithBackoff_RespectsContextCancellation_COV6(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := sleepWithBackoff(ctx, 2)
	require.Error(t, err)
}

func TestBatchWriteWithRetries_ValidationAndErrorPaths_COV6(t *testing.T) {
	t.Run("maxRetries <= 0 returns input", func(t *testing.T) {
		reqs := []types.WriteRequest{
			{PutRequest: &types.PutRequest{Item: map[string]types.AttributeValue{
				"id": &types.AttributeValueMemberS{Value: "1"},
			}}},
		}

		got, err := batchWriteWithRetries(context.Background(), nil, "tbl", reqs, 0)
		require.NoError(t, err)
		require.Equal(t, reqs, got)
	})

	t.Run("wraps BatchWriteItem failures", func(t *testing.T) {
		httpClient := newCapturingHTTPClient(nil)
		httpClient.SetResponseSequence("DynamoDB_20120810.BatchWriteItem", []stubbedResponse{
			stubbedAWSError("ValidationException", "boom"),
		})

		mgr := newTestManager(t, httpClient)
		client, err := mgr.session.Client()
		require.NoError(t, err)

		reqs := []types.WriteRequest{
			{PutRequest: &types.PutRequest{Item: map[string]types.AttributeValue{
				"id": &types.AttributeValueMemberS{Value: "1"},
			}}},
		}

		_, err = batchWriteWithRetries(context.Background(), client, "tbl", reqs, 1)
		require.ErrorContains(t, err, "failed to write batch")
	})
}

func TestPutWriteRequestsIndividually_SkipsNilAndWrapsErrors_COV6(t *testing.T) {
	httpClient := newCapturingHTTPClient(nil)
	httpClient.SetResponseSequence("DynamoDB_20120810.PutItem", []stubbedResponse{
		stubbedAWSError("ValidationException", "boom"),
	})

	mgr := newTestManager(t, httpClient)
	client, err := mgr.session.Client()
	require.NoError(t, err)

	err = putWriteRequestsIndividually(context.Background(), client, "tbl", []types.WriteRequest{
		{},
		{PutRequest: &types.PutRequest{Item: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: "1"},
		}}},
	})
	require.ErrorContains(t, err, "failed to put individual item after batch failures")
}
