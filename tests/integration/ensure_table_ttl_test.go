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

	theorydbErrors "github.com/theory-cloud/tabletheory/v6/pkg/errors"
	"github.com/theory-cloud/tabletheory/v6/tests"
)

type ensureTableTTLModel struct {
	ID        string `theorydb:"pk"`
	ExpiresAt int64  `theorydb:"ttl"`
}

func (ensureTableTTLModel) TableName() string { return "tabletheory_local_t16_ensure_table_ttl" }

func deleteEnsureTableTTLTable(t *testing.T, testCtx *TestContext) {
	t.Helper()

	ctx := context.Background()
	tableName := "tabletheory_local_t16_ensure_table_ttl"

	_, err := testCtx.DynamoDBClient.DeleteTable(ctx, &dynamodb.DeleteTableInput{
		TableName: aws.String(tableName),
	})
	if err != nil {
		var notFound *types.ResourceNotFoundException
		require.True(t, errors.As(err, &notFound), "delete existing table: %v", err)
	}
}

func resetEnsureTableTTLTable(t *testing.T, testCtx *TestContext) {
	t.Helper()

	deleteEnsureTableTTLTable(t, testCtx)

	ctx := context.Background()
	tableName := "tabletheory_local_t16_ensure_table_ttl"

	_, err := testCtx.DynamoDBClient.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName:   aws.String(tableName),
		BillingMode: types.BillingModePayPerRequest,
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("ID"), KeyType: types.KeyTypeHash},
		},
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("ID"), AttributeType: types.ScalarAttributeTypeS},
		},
	})
	require.NoError(t, err)

	waiter := dynamodb.NewTableExistsWaiter(testCtx.DynamoDBClient)
	require.NoError(t, waiter.Wait(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	}, 30*time.Second))

	t.Cleanup(func() {
		if _, err := testCtx.DynamoDBClient.DeleteTable(context.Background(), &dynamodb.DeleteTableInput{
			TableName: aws.String(tableName),
		}); err != nil {
			t.Logf("cleanup: delete table: %v", err)
		}
	})
}

func enableEnsureTableTTLAttribute(t *testing.T, testCtx *TestContext, attribute string) {
	t.Helper()

	ctx := context.Background()
	tableName := "tabletheory_local_t16_ensure_table_ttl"

	_, err := testCtx.DynamoDBClient.UpdateTimeToLive(ctx, &dynamodb.UpdateTimeToLiveInput{
		TableName: aws.String(tableName),
		TimeToLiveSpecification: &types.TimeToLiveSpecification{
			AttributeName: aws.String(attribute),
			Enabled:       aws.Bool(true),
		},
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		out, err := testCtx.DynamoDBClient.DescribeTimeToLive(ctx, &dynamodb.DescribeTimeToLiveInput{
			TableName: aws.String(tableName),
		})
		if err != nil || out.TimeToLiveDescription == nil {
			return false
		}
		return out.TimeToLiveDescription.TimeToLiveStatus == types.TimeToLiveStatusEnabled
	}, 30*time.Second, 100*time.Millisecond)
}

func TestEnsureTable_ExistingTableTTLOnDifferentAttribute_ReturnsClearError(t *testing.T) {
	tests.RequireDynamoDBLocal(t)
	testCtx := InitTestDB(t)

	resetEnsureTableTTLTable(t, testCtx)
	enableEnsureTableTTLAttribute(t, testCtx, "LegacyTtl")

	err := testCtx.DB.EnsureTable(&ensureTableTTLModel{})
	require.Error(t, err)
	require.ErrorIs(t, err, theorydbErrors.ErrTTLAttributeMismatch)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)
	require.Contains(t, err.Error(), "different attribute")
	require.Contains(t, err.Error(), "expiresAt")
	require.Contains(t, err.Error(), "LegacyTtl")
}

func TestEnsureTable_ExistingTableWithoutTTL_LeavesTTLDisabled(t *testing.T) {
	tests.RequireDynamoDBLocal(t)
	testCtx := InitTestDB(t)

	resetEnsureTableTTLTable(t, testCtx)

	require.NoError(t, testCtx.DB.EnsureTable(&ensureTableTTLModel{}))

	out, err := testCtx.DynamoDBClient.DescribeTimeToLive(context.Background(), &dynamodb.DescribeTimeToLiveInput{
		TableName: aws.String("tabletheory_local_t16_ensure_table_ttl"),
	})
	require.NoError(t, err)
	require.NotNil(t, out.TimeToLiveDescription)
	require.Equal(t, types.TimeToLiveStatusDisabled, out.TimeToLiveDescription.TimeToLiveStatus)
	require.Empty(t, aws.ToString(out.TimeToLiveDescription.AttributeName))
}

func TestEnsureTable_ExistingTableWithModelTTL_IsIdempotent(t *testing.T) {
	tests.RequireDynamoDBLocal(t)
	testCtx := InitTestDB(t)

	// A table this call creates gets TTL enabled at creation.
	deleteEnsureTableTTLTable(t, testCtx)
	require.NoError(t, testCtx.DB.CreateTable(&ensureTableTTLModel{}))

	// EnsureTable against the existing, already-configured table is a no-op.
	require.NoError(t, testCtx.DB.EnsureTable(&ensureTableTTLModel{}))

	out, err := testCtx.DynamoDBClient.DescribeTimeToLive(context.Background(), &dynamodb.DescribeTimeToLiveInput{
		TableName: aws.String("tabletheory_local_t16_ensure_table_ttl"),
	})
	require.NoError(t, err)
	require.NotNil(t, out.TimeToLiveDescription)
	require.Equal(t, types.TimeToLiveStatusEnabled, out.TimeToLiveDescription.TimeToLiveStatus)
	require.Equal(t, "expiresAt", aws.ToString(out.TimeToLiveDescription.AttributeName))
}
