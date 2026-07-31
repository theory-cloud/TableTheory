package transaction

import (
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	customerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/model"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
	"github.com/theory-cloud/tabletheory/v3/pkg/testing/fakedb"
	pkgTypes "github.com/theory-cloud/tabletheory/v3/pkg/types"
)

type transactionalLifecycleRecord struct {
	CreatedAt time.Time `theorydb:"created_at,attr:createdAt" json:"createdAt"`
	UpdatedAt time.Time `theorydb:"updated_at,attr:updatedAt" json:"updatedAt"`
	PK        string    `theorydb:"pk,attr:PK" json:"PK"`
	SK        string    `theorydb:"sk,attr:SK" json:"SK"`
	Value     string    `theorydb:"attr:value" json:"value"`
	Version   int       `theorydb:"version,attr:version" json:"version"`
}

func TestBuilderUpdateRejectsLifecycleOwnedFields(t *testing.T) {
	tests := []struct {
		field       string
		messagePart string
	}{
		{field: "CreatedAt", messagePart: "cannot update lifecycle timestamp field CreatedAt"},
		{field: "UpdatedAt", messagePart: "cannot update lifecycle timestamp field UpdatedAt"},
		{field: "Version", messagePart: "do not include version in update fields: Version"},
	}

	for _, test := range tests {
		t.Run(test.field, func(t *testing.T) {
			registry := model.NewRegistry()
			require.NoError(t, registry.Register(&transactionalLifecycleRecord{}))

			builder := NewBuilder(&session.Session{}, registry, pkgTypes.NewConverter())
			builder.Update(&transactionalLifecycleRecord{
				PK:        "USER#lifecycle",
				SK:        "PROFILE",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
				Version:   7,
			}, []string{test.field})

			items, err := builder.materializeOperations()
			require.ErrorIs(t, err, customerrors.ErrInvalidModel)
			require.ErrorContains(t, err, test.messagePart)
			require.Nil(t, items, "invalid lifecycle selection must not produce a DynamoDB write")
		})
	}
}

func (transactionalLifecycleRecord) TableName() string {
	return "transactional_lifecycle_records"
}

func TestBuilderUpdateRefreshesUpdatedAt(t *testing.T) {
	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&transactionalLifecycleRecord{}))

	builder := NewBuilder(&session.Session{}, registry, pkgTypes.NewConverter())
	builder.Update(&transactionalLifecycleRecord{
		PK:    "USER#lifecycle",
		SK:    "PROFILE",
		Value: "changed",
	}, []string{"Value"})

	items, err := builder.materializeOperations()
	require.NoError(t, err)
	require.Len(t, items, 1)
	update := items[0].Update
	require.NotNil(t, update)
	require.Contains(t, aws.ToString(update.UpdateExpression), "SET")
	require.Contains(t, attributeNames(update.ExpressionAttributeNames), "value")
	require.Contains(t, attributeNames(update.ExpressionAttributeNames), "updatedAt")
	require.NotContains(t, attributeNames(update.ExpressionAttributeNames), "createdAt")
	require.Len(t, update.ExpressionAttributeValues, 2)
}

type legacyTransactionalLifecycleRecord struct {
	UpdatedAt time.Time `theorydb:"updated_at,attr:updatedAt" json:"updatedAt"`
	PK        string    `theorydb:"pk,attr:PK" json:"PK"`
	SK        string    `theorydb:"sk,attr:SK" json:"SK"`
	Value     string    `theorydb:"attr:value" json:"value"`
	Optional  string    `theorydb:"attr:optional,omitempty" json:"optional,omitempty"`
}

func (legacyTransactionalLifecycleRecord) TableName() string {
	return "legacy_transactional_lifecycle_records"
}

func TestTransactionUpdateLegacyOverlapExpressionProbe(t *testing.T) {
	fake := fakedb.New()
	sess, err := session.NewSessionWithClient(&session.Config{Region: "us-east-1"}, fake)
	require.NoError(t, err)

	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&legacyTransactionalLifecycleRecord{}))

	tx := NewTransaction(sess, registry, pkgTypes.NewConverter())
	require.NoError(t, tx.Update(&legacyTransactionalLifecycleRecord{
		PK:    "USER#legacy-overlap",
		SK:    "PROFILE",
		Value: "changed",
	}))
	require.Len(t, tx.writes, 1)
	require.NotNil(t, tx.writes[0].Update)

	update := tx.writes[0].Update
	expression := aws.ToString(update.UpdateExpression)
	t.Logf("legacy overlap update expression: %s", expression)
	require.Equal(t, 1, updatePathOccurrences(expression, update.ExpressionAttributeNames, "updatedAt"),
		"the library-managed updated_at path must appear exactly once")
	require.NoError(t, tx.Commit(), "the built legacy transaction must validate and execute against the DynamoDBAPI fake")

	items := fake.Items((legacyTransactionalLifecycleRecord{}).TableName())
	require.Len(t, items, 1)
	_, ok := items[0]["updatedAt"].(*types.AttributeValueMemberS)
	require.True(t, ok, "the committed transaction must write the library-managed timestamp")
}

func TestTransactionUpdateLegacyRejectsCallerSetUpdatedAtLikeExplicitPath(t *testing.T) {
	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&legacyTransactionalLifecycleRecord{}))
	record := &legacyTransactionalLifecycleRecord{
		PK:        "USER#legacy-caller-timestamp",
		SK:        "PROFILE",
		Value:     "changed",
		UpdatedAt: time.Now(),
	}

	explicit := NewBuilder(&session.Session{}, registry, pkgTypes.NewConverter())
	explicit.Update(record, []string{"UpdatedAt"})
	items, explicitErr := explicit.materializeOperations()
	require.ErrorIs(t, explicitErr, customerrors.ErrInvalidModel)
	require.Nil(t, items)

	legacy := NewTransaction(&session.Session{}, registry, pkgTypes.NewConverter())
	legacyErr := legacy.Update(record)
	require.ErrorIs(t, legacyErr, customerrors.ErrInvalidModel)
	require.EqualError(t, legacyErr, explicitErr.Error())
	require.Empty(t, legacy.writes, "invalid lifecycle selection must not produce a DynamoDB write")
}

func TestTransactionUpdateLegacyPreservesSparseSetSemanticsAndRefreshesUpdatedAt(t *testing.T) {
	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&legacyTransactionalLifecycleRecord{}))

	tx := NewTransaction(&session.Session{}, registry, pkgTypes.NewConverter())
	require.NoError(t, tx.Update(&legacyTransactionalLifecycleRecord{
		PK:    "USER#legacy-normal",
		SK:    "PROFILE",
		Value: "changed",
	}))
	require.Len(t, tx.writes, 1)
	require.NotNil(t, tx.writes[0].Update)

	update := tx.writes[0].Update
	expression := aws.ToString(update.UpdateExpression)
	require.Contains(t, expression, "SET")
	require.NotContains(t, expression, "REMOVE",
		"legacy implicit updates keep zero-valued omitempty fields unselected rather than removing them")
	require.ElementsMatch(t, []string{"value", "updatedAt"}, attributeNames(update.ExpressionAttributeNames))
	require.Len(t, update.ExpressionAttributeValues, 2)
	require.Equal(t, 1, updatePathOccurrences(expression, update.ExpressionAttributeNames, "updatedAt"))
}

func updatePathOccurrences(expression string, names map[string]string, path string) int {
	count := 0
	for placeholder, attribute := range names {
		if attribute == path {
			count += strings.Count(expression, placeholder)
		}
	}
	return count
}
