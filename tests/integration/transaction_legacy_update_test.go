package integration

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v3/pkg/model"
	"github.com/theory-cloud/tabletheory/v3/pkg/query"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
	"github.com/theory-cloud/tabletheory/v3/pkg/transaction"
	pkgTypes "github.com/theory-cloud/tabletheory/v3/pkg/types"
)

type legacyTransactionLifecycleRecord struct {
	CreatedAt time.Time `theorydb:"created_at,attr:createdAt" json:"createdAt"`
	UpdatedAt time.Time `theorydb:"updated_at,attr:updatedAt" json:"updatedAt"`
	PK        string    `theorydb:"pk,attr:PK" json:"PK"`
	SK        string    `theorydb:"sk,attr:SK" json:"SK"`
	Value     string    `theorydb:"attr:value,omitempty" json:"value,omitempty"`
	Version   int       `theorydb:"version,attr:version" json:"version"`
}

func (legacyTransactionLifecycleRecord) TableName() string {
	return "legacy_transaction_lifecycle_integration"
}

func TestLegacyTransactionUpdateExecutesWithoutLifecycleOverlap(t *testing.T) {
	testCtx := InitTestDB(t)
	testCtx.CreateTableIfNotExists(t, &legacyTransactionLifecycleRecord{})
	t.Cleanup(func() {
		testCtx.DeleteTable(t, (legacyTransactionLifecycleRecord{}).TableName())
	})

	sess, err := session.NewSessionWithClient(&session.Config{Region: "us-east-1"}, testCtx.DynamoDBClient)
	require.NoError(t, err)
	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&legacyTransactionLifecycleRecord{}))

	seedCreatedAt := time.Date(2020, time.March, 4, 5, 6, 7, 0, time.UTC).Format(time.RFC3339Nano)
	seedUpdatedAt := time.Date(2020, time.March, 5, 6, 7, 8, 0, time.UTC).Format(time.RFC3339Nano)
	seed := func(t *testing.T, pk string, version int) {
		t.Helper()
		item := map[string]types.AttributeValue{
			"PK":        &types.AttributeValueMemberS{Value: pk},
			"SK":        &types.AttributeValueMemberS{Value: "PROFILE"},
			"value":     &types.AttributeValueMemberS{Value: "original"},
			"createdAt": &types.AttributeValueMemberS{Value: seedCreatedAt},
			"updatedAt": &types.AttributeValueMemberS{Value: seedUpdatedAt},
		}
		if version > 0 {
			item["version"] = &types.AttributeValueMemberN{Value: strconv.Itoa(version)}
		}
		_, err := testCtx.DynamoDBClient.PutItem(context.Background(), &dynamodb.PutItemInput{
			TableName: aws.String((legacyTransactionLifecycleRecord{}).TableName()),
			Item:      item,
		})
		require.NoError(t, err)
	}

	load := func(t *testing.T, pk string) map[string]types.AttributeValue {
		t.Helper()
		output, err := testCtx.DynamoDBClient.GetItem(context.Background(), &dynamodb.GetItemInput{
			TableName: aws.String((legacyTransactionLifecycleRecord{}).TableName()),
			Key: map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: pk},
				"SK": &types.AttributeValueMemberS{Value: "PROFILE"},
			},
			ConsistentRead: aws.Bool(true),
		})
		require.NoError(t, err)
		require.NotEmpty(t, output.Item)
		return output.Item
	}

	t.Run("versioned managed-only update has well-formed SET", func(t *testing.T) {
		const pk = "USER#legacy-managed-only"
		seed(t, pk, 7)

		tx := transaction.NewTransaction(sess, registry, pkgTypes.NewConverter())
		require.NoError(t, tx.Update(&legacyTransactionLifecycleRecord{
			PK:      pk,
			SK:      "PROFILE",
			Version: 7,
		}))
		require.NoError(t, tx.Commit(),
			"DynamoDB Local must accept the SET clause built entirely from version and updated_at assignments")

		item := load(t, pk)
		value, ok := item["value"].(*types.AttributeValueMemberS)
		require.True(t, ok)
		require.Equal(t, "original", value.Value, "no caller-owned field was selected")
		version, ok := item["version"].(*types.AttributeValueMemberN)
		require.True(t, ok)
		require.Equal(t, "8", version.Value)
		createdAt, ok := item["createdAt"].(*types.AttributeValueMemberS)
		require.True(t, ok)
		require.Equal(t, seedCreatedAt, createdAt.Value)
		updatedAt, ok := item["updatedAt"].(*types.AttributeValueMemberS)
		require.True(t, ok)
		require.NotEqual(t, seedUpdatedAt, updatedAt.Value)
	})

	t.Run("versioned update has no overlapping paths", func(t *testing.T) {
		const pk = "USER#legacy-versioned"
		seed(t, pk, 7)

		tx := transaction.NewTransaction(sess, registry, pkgTypes.NewConverter())
		require.NoError(t, tx.Update(&legacyTransactionLifecycleRecord{
			PK:      pk,
			SK:      "PROFILE",
			Value:   "changed",
			Version: 7,
		}))
		require.NoError(t, tx.Commit())

		item := load(t, pk)
		value, ok := item["value"].(*types.AttributeValueMemberS)
		require.True(t, ok)
		require.Equal(t, "changed", value.Value)
		version, ok := item["version"].(*types.AttributeValueMemberN)
		require.True(t, ok)
		require.Equal(t, "8", version.Value)
		createdAt, ok := item["createdAt"].(*types.AttributeValueMemberS)
		require.True(t, ok)
		require.Equal(t, seedCreatedAt, createdAt.Value)
		updatedAt, ok := item["updatedAt"].(*types.AttributeValueMemberS)
		require.True(t, ok)
		require.NotEmpty(t, updatedAt.Value)
	})

	t.Run("zero created_at does not clobber stored value", func(t *testing.T) {
		const pk = "USER#legacy-created-at"
		seed(t, pk, 0)

		tx := transaction.NewTransaction(sess, registry, pkgTypes.NewConverter())
		require.NoError(t, tx.Update(&legacyTransactionLifecycleRecord{
			PK:    pk,
			SK:    "PROFILE",
			Value: "changed",
		}))
		require.NoError(t, tx.Commit())

		item := load(t, pk)
		createdAt, ok := item["createdAt"].(*types.AttributeValueMemberS)
		require.True(t, ok)
		require.Equal(t, seedCreatedAt, createdAt.Value)
	})

	t.Run("loaded lifecycle values support read modify write", func(t *testing.T) {
		const pk = "USER#legacy-rmw"
		seed(t, pk, 7)

		var loaded legacyTransactionLifecycleRecord
		require.NoError(t, query.UnmarshalItem(load(t, pk), &loaded))
		require.False(t, loaded.CreatedAt.IsZero())
		require.False(t, loaded.UpdatedAt.IsZero())
		loaded.Value = "changed-after-load"

		tx := transaction.NewTransaction(sess, registry, pkgTypes.NewConverter())
		require.NoError(t, tx.Update(&loaded),
			"the implicit whole-model surface must accept lifecycle values populated by a read")
		require.NoError(t, tx.Commit())

		item := load(t, pk)
		value, ok := item["value"].(*types.AttributeValueMemberS)
		require.True(t, ok)
		require.Equal(t, "changed-after-load", value.Value)
		createdAt, ok := item["createdAt"].(*types.AttributeValueMemberS)
		require.True(t, ok)
		require.Equal(t, seedCreatedAt, createdAt.Value,
			"created_at must be preserved byte-exact even though the loaded model carries it")
		updatedAt, ok := item["updatedAt"].(*types.AttributeValueMemberS)
		require.True(t, ok)
		require.NotEqual(t, seedUpdatedAt, updatedAt.Value,
			"updated_at must be refreshed rather than reusing the loaded value")
	})
}
