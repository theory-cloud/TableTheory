package integration

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v3/tests"
)

type UpdateOmitEmptyItem struct {
	Attributes           map[string]string `theorydb:"attr:attributes,omitempty"`
	ID                   string            `theorydb:"pk"`
	SK                   string            `theorydb:"sk"`
	EncryptedPaymentData string            `theorydb:"attr:encryptedPaymentData"`
	UpdateTimestamp      string            `theorydb:"attr:updateTimestamp"`
	ProcessorTokens      []string          `theorydb:"attr:processorTokens,omitempty"`
}

func (UpdateOmitEmptyItem) TableName() string { return "UpdateOmitEmptyItems" }

type updateStructuredProfile struct {
	Source string `theorydb:"attr:source" json:"source"`
}

type updateStructuredItem struct {
	PK      string                  `theorydb:"pk,attr:PK" json:"PK"`
	SK      string                  `theorydb:"sk,attr:SK" json:"SK"`
	Profile updateStructuredProfile `theorydb:"attr:profile,omitempty" json:"profile,omitempty"`
	Status  string                  `theorydb:"attr:status,omitempty" json:"status,omitempty"`
	Values  [2]int                  `theorydb:"attr:values,omitempty" json:"values,omitempty"`
}

func (updateStructuredItem) TableName() string { return "UpdateStructuredItems" }

func TestUpdate_OmitEmptyDoesNotOverwriteEmptyCollections(t *testing.T) {
	tests.RequireDynamoDBLocal(t)

	testCtx := InitTestDB(t)
	testCtx.CreateTableIfNotExists(t, &UpdateOmitEmptyItem{})

	original := &UpdateOmitEmptyItem{
		ID:                   "pmt#1",
		SK:                   "token#1",
		ProcessorTokens:      []string{"tok_123"},
		Attributes:           map[string]string{"stripe": "tok_123"},
		EncryptedPaymentData: "enc_v1",
		UpdateTimestamp:      "ts_v1",
	}

	err := testCtx.DB.Model(original).Create()
	require.NoError(t, err)

	update := &UpdateOmitEmptyItem{
		ID:                   original.ID,
		SK:                   original.SK,
		ProcessorTokens:      []string{},          // empty-but-non-nil
		Attributes:           map[string]string{}, // empty-but-non-nil
		EncryptedPaymentData: "enc_v2",
		UpdateTimestamp:      "ts_v2",
	}

	err = testCtx.DB.Model(update).
		Where("ID", "=", original.ID).
		Where("SK", "=", original.SK).
		Update()
	require.NoError(t, err)

	var got UpdateOmitEmptyItem
	err = testCtx.DB.Model(&UpdateOmitEmptyItem{}).
		Where("ID", "=", original.ID).
		Where("SK", "=", original.SK).
		First(&got)
	require.NoError(t, err)

	assert.Equal(t, "enc_v2", got.EncryptedPaymentData)
	assert.Equal(t, "ts_v2", got.UpdateTimestamp)
	assert.Equal(t, []string{"tok_123"}, got.ProcessorTokens)
	assert.Equal(t, map[string]string{"stripe": "tok_123"}, got.Attributes)

	err = testCtx.DB.Model(update).
		Where("ID", "=", original.ID).
		Where("SK", "=", original.SK).
		Update("ProcessorTokens", "Attributes")
	require.NoError(t, err)

	var explicitlyCleared UpdateOmitEmptyItem
	err = testCtx.DB.Model(&UpdateOmitEmptyItem{}).
		Where("ID", "=", original.ID).
		Where("SK", "=", original.SK).
		First(&explicitlyCleared)
	require.NoError(t, err)

	assert.Empty(t, explicitlyCleared.ProcessorTokens)
	assert.Empty(t, explicitlyCleared.Attributes)

	raw, err := testCtx.DynamoDBClient.GetItem(context.Background(), &dynamodb.GetItemInput{
		TableName: aws.String(original.TableName()),
		Key: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: original.ID},
			"SK": &types.AttributeValueMemberS{Value: original.SK},
		},
		ConsistentRead: aws.Bool(true),
	})
	require.NoError(t, err)
	require.NotEmpty(t, raw.Item)
	require.NotContains(t, raw.Item, "processorTokens")
	require.NotContains(t, raw.Item, "attributes")
}

func TestUpdate_OmitEmptySparsePreservesStructAndFixedArray(t *testing.T) {
	tests.RequireDynamoDBLocal(t)

	testCtx := InitTestDB(t)
	testCtx.CreateTableIfNotExists(t, &updateStructuredItem{})

	original := &updateStructuredItem{
		PK:      "ACCOUNT#1",
		SK:      "PROFILE",
		Profile: updateStructuredProfile{Source: "present"},
		Status:  "before",
		Values:  [2]int{7, 8},
	}
	require.NoError(t, testCtx.DB.Model(original).Create())

	sparse := &updateStructuredItem{
		PK:     original.PK,
		SK:     original.SK,
		Status: "after",
	}
	require.NoError(t, testCtx.DB.Model(sparse).
		Where("PK", "=", original.PK).
		Where("SK", "=", original.SK).
		Update())

	var preserved updateStructuredItem
	require.NoError(t, testCtx.DB.Model(&updateStructuredItem{}).
		Where("PK", "=", original.PK).
		Where("SK", "=", original.SK).
		First(&preserved))
	require.Equal(t, "after", preserved.Status)
	require.Equal(t, updateStructuredProfile{Source: "present"}, preserved.Profile)
	require.Equal(t, [2]int{7, 8}, preserved.Values)

	raw := getRawUpdateStructuredItem(t, testCtx, original.PK, original.SK)
	profile := requireUpdateProfile(t, raw.Item)
	require.Equal(t, "present", requireUpdateProfileSource(t, profile).Value)
	values := requireUpdateValues(t, raw.Item)
	require.Equal(t, "7", requireUpdateNumber(t, values.Value[0]).Value)
	require.Equal(t, "8", requireUpdateNumber(t, values.Value[1]).Value)

	explicit := &updateStructuredItem{PK: original.PK, SK: original.SK}
	require.NoError(t, testCtx.DB.Model(explicit).
		Where("PK", "=", original.PK).
		Where("SK", "=", original.SK).
		Update("Profile", "Values"))

	raw = getRawUpdateStructuredItem(t, testCtx, original.PK, original.SK)
	profile = requireUpdateProfile(t, raw.Item)
	require.Equal(t, "", requireUpdateProfileSource(t, profile).Value)
	values = requireUpdateValues(t, raw.Item)
	require.Len(t, values.Value, 2)
	require.Equal(t, "0", requireUpdateNumber(t, values.Value[0]).Value)
	require.Equal(t, "0", requireUpdateNumber(t, values.Value[1]).Value)
}

func getRawUpdateStructuredItem(t *testing.T, testCtx *TestContext, pk, sk string) *dynamodb.GetItemOutput {
	t.Helper()
	raw, err := testCtx.DynamoDBClient.GetItem(context.Background(), &dynamodb.GetItemInput{
		TableName: aws.String((updateStructuredItem{}).TableName()),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: sk},
		},
		ConsistentRead: aws.Bool(true),
	})
	require.NoError(t, err)
	require.NotEmpty(t, raw.Item)
	return raw
}

func requireUpdateProfile(t *testing.T, item map[string]types.AttributeValue) *types.AttributeValueMemberM {
	t.Helper()
	profile, ok := item["profile"].(*types.AttributeValueMemberM)
	require.True(t, ok, "profile should be DynamoDB M")
	return profile
}

func requireUpdateProfileSource(t *testing.T, profile *types.AttributeValueMemberM) *types.AttributeValueMemberS {
	t.Helper()
	source, ok := profile.Value["source"].(*types.AttributeValueMemberS)
	require.True(t, ok, "profile.source should be DynamoDB S")
	return source
}

func requireUpdateValues(t *testing.T, item map[string]types.AttributeValue) *types.AttributeValueMemberL {
	t.Helper()
	values, ok := item["values"].(*types.AttributeValueMemberL)
	require.True(t, ok, "values should be DynamoDB L")
	return values
}

func requireUpdateNumber(t *testing.T, value types.AttributeValue) *types.AttributeValueMemberN {
	t.Helper()
	number, ok := value.(*types.AttributeValueMemberN)
	require.True(t, ok, "array element should be DynamoDB N")
	return number
}
