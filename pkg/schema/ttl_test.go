package schema

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v6/pkg/model"
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

func TestManager_EnableTTL_Success_COV6(t *testing.T) {
	httpClient := newCapturingHTTPClient(nil)
	httpClient.SetResponseSequence("DynamoDB_20120810.UpdateTimeToLive", []stubbedResponse{
		{body: `{"TimeToLiveSpecification":{"AttributeName":"expiresAt","Enabled":true}}`},
	})

	mgr := newTestManager(t, httpClient)
	require.NoError(t, mgr.registry.Register(&ttlSchemaModel{}))

	require.NoError(t, mgr.EnableTTL(&ttlSchemaModel{}))

	reqs := httpClient.Requests()
	require.Equal(t, 1, countRequestsByTarget(reqs, "DynamoDB_20120810.UpdateTimeToLive"))
	require.Equal(t, map[string]any{
		"AttributeName": "expiresAt",
		"Enabled":       true,
	}, reqs[len(reqs)-1].Payload["TimeToLiveSpecification"])
}

func TestManager_EnableTTL_WrapsMetadataAndClientErrors_COV6(t *testing.T) {
	t.Run("metadata lookup errors are wrapped", func(t *testing.T) {
		mgr := &Manager{registry: model.NewRegistry()}

		err := mgr.EnableTTL(&ttlSchemaModel{})
		require.ErrorContains(t, err, "failed to get model metadata")
	})

	t.Run("client errors are wrapped", func(t *testing.T) {
		mgr := &Manager{registry: modelRegistryWith(t, &ttlSchemaModel{})}

		err := mgr.EnableTTL(&ttlSchemaModel{})
		require.ErrorContains(t, err, "failed to get client for ttl update")
	})
}

func TestManager_SyncModelTTL_WrapsUpdateErrors_COV6(t *testing.T) {
	httpClient := newCapturingHTTPClient(nil)
	httpClient.SetResponseSequence("DynamoDB_20120810.UpdateTimeToLive", []stubbedResponse{
		stubbedAWSError("ValidationException", "boom"),
	})

	mgr := newTestManager(t, httpClient)
	require.NoError(t, mgr.registry.Register(&ttlSchemaModel{}))

	client, err := mgr.session.Client()
	require.NoError(t, err)

	metadata, err := mgr.registry.GetMetadata(&ttlSchemaModel{})
	require.NoError(t, err)

	err = mgr.syncModelTTL(context.Background(), client, metadata)
	require.ErrorContains(t, err, "failed to enable ttl on table ttl_records")
}

func TestManager_CreateTable_ExistingTableWithTTLAlreadyEnabled_COV6(t *testing.T) {
	httpClient := newCapturingHTTPClient(nil)
	httpClient.SetResponseSequence("DynamoDB_20120810.CreateTable", []stubbedResponse{
		stubbedAWSError("ResourceInUseException", "exists"),
	})
	httpClient.SetResponseSequence("DynamoDB_20120810.UpdateTimeToLive", []stubbedResponse{
		stubbedAWSError("ValidationException", "TimeToLive is already enabled"),
	})
	httpClient.SetResponseSequence("DynamoDB_20120810.DescribeTimeToLive", []stubbedResponse{
		{body: `{"TimeToLiveDescription":{"TimeToLiveStatus":"ENABLED","AttributeName":"expiresAt"}}`},
	})

	mgr := newTestManager(t, httpClient)
	require.NoError(t, mgr.registry.Register(&ttlSchemaModel{}))

	require.NoError(t, mgr.CreateTable(&ttlSchemaModel{}))

	reqs := httpClient.Requests()
	require.Equal(t, 1, countRequestsByTarget(reqs, "DynamoDB_20120810.CreateTable"))
	require.Equal(t, 1, countRequestsByTarget(reqs, "DynamoDB_20120810.UpdateTimeToLive"))
	require.Equal(t, 1, countRequestsByTarget(reqs, "DynamoDB_20120810.DescribeTimeToLive"))
}

func TestManager_EnableTTL_AlreadyEnabledOnDifferentAttribute_Errors_COV6(t *testing.T) {
	httpClient := newCapturingHTTPClient(nil)
	httpClient.SetResponseSequence("DynamoDB_20120810.UpdateTimeToLive", []stubbedResponse{
		stubbedAWSError("ValidationException", "TimeToLive is active on a different AttributeName"),
	})
	httpClient.SetResponseSequence("DynamoDB_20120810.DescribeTimeToLive", []stubbedResponse{
		{body: `{"TimeToLiveDescription":{"TimeToLiveStatus":"ENABLED","AttributeName":"legacyTtl"}}`},
	})

	mgr := newTestManager(t, httpClient)
	require.NoError(t, mgr.registry.Register(&ttlSchemaModel{}))

	err := mgr.EnableTTL(&ttlSchemaModel{})
	require.ErrorIs(t, err, ErrTTLAttributeMismatch)
	require.EqualError(t, err,
		"failed to enable ttl on table ttl_records using attribute expiresAt: ttl is already enabled on a different attribute legacyTtl: operation error DynamoDB: UpdateTimeToLive, https response error StatusCode: 400, RequestID: , api error ValidationException: TimeToLive is active on a different AttributeName")
	require.ErrorContains(t, err, "different attribute")
	require.ErrorContains(t, err, "expiresAt")
	require.ErrorContains(t, err, "legacyTtl")
	require.ErrorContains(t, err, "TimeToLive is active on a different AttributeName")
}

func TestManager_EnableTTL_ConflictErrorKeepsSingleStepUnwrap_COV6(t *testing.T) {
	httpClient := newCapturingHTTPClient(nil)
	httpClient.SetResponseSequence("DynamoDB_20120810.UpdateTimeToLive", []stubbedResponse{
		stubbedAWSError("ValidationException", "TimeToLive is active on a different AttributeName"),
	})
	httpClient.SetResponseSequence("DynamoDB_20120810.DescribeTimeToLive", []stubbedResponse{
		{body: `{"TimeToLiveDescription":{"TimeToLiveStatus":"ENABLED","AttributeName":"legacyTtl"}}`},
	})

	mgr := newTestManager(t, httpClient)
	require.NoError(t, mgr.registry.Register(&ttlSchemaModel{}))

	err := mgr.EnableTTL(&ttlSchemaModel{})
	require.Error(t, err)

	cause := errors.Unwrap(err)
	require.IsType(t, &smithy.OperationError{}, cause, "errors.Unwrap must return the AWS cause in one step")
	require.Same(t, cause, errors.Unwrap(err))

	require.ErrorIs(t, err, ErrTTLAttributeMismatch)
	require.ErrorIs(t, err, cause)

	var operationErr *smithy.OperationError
	require.ErrorAs(t, err, &operationErr)
	require.Same(t, cause, operationErr)

	var validationErr smithy.APIError
	require.ErrorAs(t, err, &validationErr)
	require.Equal(t, "ValidationException", validationErr.ErrorCode())

	conflictChain := unwrapChain(err)
	baseChain := unwrapChain(fmt.Errorf("%w", cause))
	require.Equal(t, len(baseChain), len(conflictChain), "manual unwrap depth must match the pre-typed fmt.Errorf chain")
	require.Equal(t, baseChain[1:], conflictChain[1:])
}

func unwrapChain(err error) []error {
	var chain []error
	for err != nil {
		chain = append(chain, err)
		err = errors.Unwrap(err)
	}
	return chain
}

func TestManager_EnableTTL_DescribeFailureKeepsOriginalError_COV6(t *testing.T) {
	httpClient := newCapturingHTTPClient(nil)
	httpClient.SetResponseSequence("DynamoDB_20120810.UpdateTimeToLive", []stubbedResponse{
		stubbedAWSError("ValidationException", "TimeToLive is already enabled"),
	})
	httpClient.SetResponseSequence("DynamoDB_20120810.DescribeTimeToLive", []stubbedResponse{
		stubbedAWSError("InternalServerError", "describe failed"),
	})

	mgr := newTestManager(t, httpClient)
	require.NoError(t, mgr.registry.Register(&ttlSchemaModel{}))

	err := mgr.EnableTTL(&ttlSchemaModel{})
	require.ErrorContains(t, err, "TimeToLive is already enabled")
	require.NotContains(t, err.Error(), "different attribute")
}

func TestManager_EnableTTL_NonValidationErrorSkipsDescribe_COV6(t *testing.T) {
	httpClient := newCapturingHTTPClient(nil)
	httpClient.SetResponseSequence("DynamoDB_20120810.UpdateTimeToLive", []stubbedResponse{
		stubbedAWSError("ResourceNotFoundException", "table is gone"),
	})

	mgr := newTestManager(t, httpClient)
	require.NoError(t, mgr.registry.Register(&ttlSchemaModel{}))

	err := mgr.EnableTTL(&ttlSchemaModel{})
	require.ErrorContains(t, err, "table is gone")
	require.Equal(t, 0, countRequestsByTarget(httpClient.Requests(), "DynamoDB_20120810.DescribeTimeToLive"))
}

func modelRegistryWith(t *testing.T, values ...any) *model.Registry {
	t.Helper()

	registry := model.NewRegistry()
	for _, value := range values {
		require.NoError(t, registry.Register(value))
	}
	return registry
}
