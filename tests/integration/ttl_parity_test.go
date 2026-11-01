package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v6/pkg/schema"
)

type ttlParityRecord struct {
	ID        string `theorydb:"pk"`
	ExpiresAt int64  `theorydb:"ttl"`
}

func (ttlParityRecord) TableName() string { return "tabletheory_it_ttl_parity_go" }

func createTTLParityTable(t *testing.T, ctx context.Context, testCtx *TestContext) {
	t.Helper()

	_, err := testCtx.DynamoDBClient.DeleteTable(ctx, &dynamodb.DeleteTableInput{
		TableName: aws.String(ttlParityRecord{}.TableName()),
	})
	if err != nil {
		var notFound *types.ResourceNotFoundException
		require.True(t, errors.As(err, &notFound), "delete table: %v", err)
	}
	waitNotExists := dynamodb.NewTableNotExistsWaiter(testCtx.DynamoDBClient)
	require.NoError(t, waitNotExists.Wait(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(ttlParityRecord{}.TableName()),
	}, 30*time.Second))

	_, err = testCtx.DynamoDBClient.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName:   aws.String(ttlParityRecord{}.TableName()),
		BillingMode: types.BillingModePayPerRequest,
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("ID"), KeyType: types.KeyTypeHash},
		},
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("ID"), AttributeType: types.ScalarAttributeTypeS},
		},
	})
	require.NoError(t, err)

	waitExists := dynamodb.NewTableExistsWaiter(testCtx.DynamoDBClient)
	require.NoError(t, waitExists.Wait(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(ttlParityRecord{}.TableName()),
	}, 30*time.Second))

	t.Cleanup(func() {
		_, err := testCtx.DynamoDBClient.DeleteTable(context.Background(), &dynamodb.DeleteTableInput{
			TableName: aws.String(ttlParityRecord{}.TableName()),
		})
		if err != nil {
			var notFound *types.ResourceNotFoundException
			require.True(t, errors.As(err, &notFound), "cleanup delete table: %v", err)
		}
	})
}

// TestEnsureTable_ExistingTableWithTTLDisabled_LeavesTTLDisabled pins the Go
// reference behaviour: EnsureTable never touches TTL on a table that already
// exists.
func TestEnsureTable_ExistingTableWithTTLDisabled_LeavesTTLDisabled(t *testing.T) {
	testCtx := InitTestDB(t)
	ctx := context.Background()

	createTTLParityTable(t, ctx, testCtx)

	require.NoError(t, testCtx.DB.EnsureTable(&ttlParityRecord{}))

	ttl, err := testCtx.DynamoDBClient.DescribeTimeToLive(ctx, &dynamodb.DescribeTimeToLiveInput{
		TableName: aws.String(ttlParityRecord{}.TableName()),
	})
	require.NoError(t, err)
	require.NotNil(t, ttl.TimeToLiveDescription)
	require.Equal(t, types.TimeToLiveStatusDisabled, ttl.TimeToLiveDescription.TimeToLiveStatus)
}

// TestCreateTable_ExistingTableWithoutTTL_EnablesTTL pins the Go reference
// convergence path used when CreateTable meets an existing table.
func TestCreateTable_ExistingTableWithoutTTL_EnablesTTL(t *testing.T) {
	testCtx := InitTestDB(t)
	ctx := context.Background()

	createTTLParityTable(t, ctx, testCtx)

	require.NoError(t, testCtx.DB.CreateTable(&ttlParityRecord{}))

	ttl, err := testCtx.DynamoDBClient.DescribeTimeToLive(ctx, &dynamodb.DescribeTimeToLiveInput{
		TableName: aws.String(ttlParityRecord{}.TableName()),
	})
	require.NoError(t, err)
	require.NotNil(t, ttl.TimeToLiveDescription)
	require.Equal(t, types.TimeToLiveStatusEnabled, ttl.TimeToLiveDescription.TimeToLiveStatus)
	require.Equal(t, "expiresAt", aws.ToString(ttl.TimeToLiveDescription.AttributeName))
}

// TestCreateTable_ExistingTableWithTTLOnDifferentAttribute_MatchesSentinel pins
// the typed conflict that Go exposes for callers that use errors.Is.
func TestCreateTable_ExistingTableWithTTLOnDifferentAttribute_MatchesSentinel(t *testing.T) {
	testCtx := InitTestDB(t)
	ctx := context.Background()

	createTTLParityTable(t, ctx, testCtx)

	_, err := testCtx.DynamoDBClient.UpdateTimeToLive(ctx, &dynamodb.UpdateTimeToLiveInput{
		TableName: aws.String(ttlParityRecord{}.TableName()),
		TimeToLiveSpecification: &types.TimeToLiveSpecification{
			AttributeName: aws.String("LegacyTtl"),
			Enabled:       aws.Bool(true),
		},
	})
	require.NoError(t, err)

	err = testCtx.DB.CreateTable(&ttlParityRecord{})
	require.Error(t, err)
	require.ErrorIs(t, err, schema.ErrTTLAttributeMismatch)
	require.Contains(t, err.Error(), "expiresAt")
	require.Contains(t, err.Error(), "LegacyTtl")
}
