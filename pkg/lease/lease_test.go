package lease

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/mocks"
)

func TestManager_Acquire_BuildsConditionalPut(t *testing.T) {
	mockClient := new(mocks.MockDynamoDBClient)

	fixed := time.Unix(1000, 0)
	mgr, err := NewManager(
		mockClient,
		"tbl",
		WithNow(func() time.Time { return fixed }),
		WithTokenGenerator(func() string { return "tok" }),
		WithTTLBuffer(10*time.Second),
	)
	require.NoError(t, err)

	mockClient.
		On(
			"PutItem",
			mock.Anything,
			mock.MatchedBy(func(in *dynamodb.PutItemInput) bool {
				if in == nil {
					return false
				}
				if in.TableName == nil || *in.TableName != "tbl" {
					return false
				}
				if in.ConditionExpression == nil || *in.ConditionExpression != "attribute_not_exists(#pk) OR #lease_expires_at <= :now" {
					return false
				}
				if in.ExpressionAttributeNames["#pk"] != "pk" {
					return false
				}
				if in.ExpressionAttributeNames["#lease_expires_at"] != "lease_expires_at" {
					return false
				}

				nowAV, ok := in.ExpressionAttributeValues[":now"].(*types.AttributeValueMemberN)
				if !ok || nowAV.Value != "1000" {
					return false
				}

				pkAV, ok := in.Item["pk"].(*types.AttributeValueMemberS)
				if !ok || pkAV.Value != "CACHE#A" {
					return false
				}
				skAV, ok := in.Item["sk"].(*types.AttributeValueMemberS)
				if !ok || skAV.Value != DefaultLockSortKey {
					return false
				}

				tokenAV, ok := in.Item["lease_token"].(*types.AttributeValueMemberS)
				if !ok || tokenAV.Value != "tok" {
					return false
				}
				expAV, ok := in.Item["lease_expires_at"].(*types.AttributeValueMemberN)
				if !ok || expAV.Value != "1030" {
					return false
				}
				ttlAV, ok := in.Item["ttl"].(*types.AttributeValueMemberN)
				if !ok || ttlAV.Value != "1040" {
					return false
				}

				return true
			}),
			mock.Anything,
		).
		Return(&dynamodb.PutItemOutput{}, nil).
		Once()

	lease, err := mgr.Acquire(context.Background(), "CACHE#A", 30*time.Second)
	require.NoError(t, err)
	require.NotNil(t, lease)
	require.Equal(t, "CACHE#A", lease.Key.PK)
	require.Equal(t, DefaultLockSortKey, lease.Key.SK)
	require.Equal(t, "tok", lease.Token)
	require.Equal(t, int64(1030), lease.ExpiresAt)
	mockClient.AssertExpectations(t)
}

func TestManager_Acquire_ReturnsLeaseHeldOnConditionalFailure(t *testing.T) {
	mockClient := new(mocks.MockDynamoDBClient)

	fixed := time.Unix(1000, 0)
	mgr, err := NewManager(
		mockClient,
		"tbl",
		WithNow(func() time.Time { return fixed }),
		WithTokenGenerator(func() string { return "tok" }),
	)
	require.NoError(t, err)

	mockClient.
		On("PutItem", mock.Anything, mock.Anything, mock.Anything).
		Return((*dynamodb.PutItemOutput)(nil), &types.ConditionalCheckFailedException{}).
		Once()

	lease, err := mgr.Acquire(context.Background(), "CACHE#A", 30*time.Second)
	require.Nil(t, lease)
	require.Error(t, err)
	require.True(t, IsLeaseHeld(err))
	mockClient.AssertExpectations(t)
}

func TestManager_Refresh_ConditionedOnTokenAndUnexpired(t *testing.T) {
	mockClient := new(mocks.MockDynamoDBClient)

	fixed := time.Unix(1000, 0)
	mgr, err := NewManager(
		mockClient,
		"tbl",
		WithNow(func() time.Time { return fixed }),
		WithTTLBuffer(10*time.Second),
	)
	require.NoError(t, err)

	mockClient.
		On(
			"UpdateItem",
			mock.Anything,
			mock.MatchedBy(func(in *dynamodb.UpdateItemInput) bool {
				if in == nil {
					return false
				}
				if in.TableName == nil || *in.TableName != "tbl" {
					return false
				}
				if in.ConditionExpression == nil || *in.ConditionExpression != "#lease_token = :token AND #lease_expires_at > :now" {
					return false
				}
				if in.UpdateExpression == nil || *in.UpdateExpression != "SET #lease_expires_at = :exp, #ttl = :ttl" {
					return false
				}

				if in.ExpressionAttributeNames["#lease_token"] != "lease_token" {
					return false
				}
				if in.ExpressionAttributeNames["#lease_expires_at"] != "lease_expires_at" {
					return false
				}
				if in.ExpressionAttributeNames["#ttl"] != "ttl" {
					return false
				}

				tokenAV, ok := in.ExpressionAttributeValues[":token"].(*types.AttributeValueMemberS)
				if !ok || tokenAV.Value != "tok" {
					return false
				}
				nowAV, ok := in.ExpressionAttributeValues[":now"].(*types.AttributeValueMemberN)
				if !ok || nowAV.Value != "1000" {
					return false
				}
				expAV, ok := in.ExpressionAttributeValues[":exp"].(*types.AttributeValueMemberN)
				if !ok || expAV.Value != "1060" {
					return false
				}
				ttlAV, ok := in.ExpressionAttributeValues[":ttl"].(*types.AttributeValueMemberN)
				if !ok || ttlAV.Value != "1070" {
					return false
				}

				return true
			}),
			mock.Anything,
		).
		Return(&dynamodb.UpdateItemOutput{}, nil).
		Once()

	out, err := mgr.Refresh(
		context.Background(),
		Lease{
			Key:   Key{PK: "CACHE#A", SK: DefaultLockSortKey},
			Token: "tok",
		},
		60*time.Second,
	)
	require.NoError(t, err)
	require.NotNil(t, out)
	require.Equal(t, int64(1060), out.ExpiresAt)
	mockClient.AssertExpectations(t)
}

func TestManager_Release_IsBestEffortOnConditionalFailure(t *testing.T) {
	mockClient := new(mocks.MockDynamoDBClient)

	mgr, err := NewManager(mockClient, "tbl")
	require.NoError(t, err)

	mockClient.
		On("DeleteItem", mock.Anything, mock.Anything, mock.Anything).
		Return((*dynamodb.DeleteItemOutput)(nil), &types.ConditionalCheckFailedException{}).
		Once()

	err = mgr.Release(
		context.Background(),
		Lease{
			Key:   Key{PK: "CACHE#A", SK: DefaultLockSortKey},
			Token: "tok",
		},
	)
	require.NoError(t, err)
	mockClient.AssertExpectations(t)
}
