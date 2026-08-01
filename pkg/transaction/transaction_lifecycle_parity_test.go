package transaction

import (
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	dynamodbTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	customerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/model"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
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
	CreatedAt time.Time `theorydb:"created_at,attr:createdAt" json:"createdAt"`
	UpdatedAt time.Time `theorydb:"updated_at,attr:updatedAt" json:"updatedAt"`
	PK        string    `theorydb:"pk,attr:PK" json:"PK"`
	SK        string    `theorydb:"sk,attr:SK" json:"SK"`
	Value     string    `theorydb:"attr:value" json:"value"`
	Optional  string    `theorydb:"attr:optional,omitempty" json:"optional,omitempty"`
	Version   int       `theorydb:"version,attr:version" json:"version"`
}

func (legacyTransactionalLifecycleRecord) TableName() string {
	return "legacy_transactional_lifecycle_records"
}

type legacyManagedOnlyVersionedRecord struct {
	CreatedAt time.Time `theorydb:"created_at,attr:createdAt" json:"createdAt"`
	UpdatedAt time.Time `theorydb:"updated_at,attr:updatedAt" json:"updatedAt"`
	PK        string    `theorydb:"pk,attr:PK" json:"PK"`
	SK        string    `theorydb:"sk,attr:SK" json:"SK"`
	Version   int       `theorydb:"version,attr:version" json:"version"`
}

func (legacyManagedOnlyVersionedRecord) TableName() string {
	return "legacy_managed_only_versioned_records"
}

type legacyCreatedAtOnlyRecord struct {
	CreatedAt time.Time `theorydb:"created_at,attr:createdAt" json:"createdAt"`
	PK        string    `theorydb:"pk,attr:PK" json:"PK"`
	SK        string    `theorydb:"sk,attr:SK" json:"SK"`
}

func (legacyCreatedAtOnlyRecord) TableName() string {
	return "legacy_created_at_only_records"
}

func TestTransactionUpdateLegacyOverlapExpressionProbe(t *testing.T) {
	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&legacyTransactionalLifecycleRecord{}))

	tx := NewTransaction(&session.Session{}, registry, pkgTypes.NewConverter())
	require.NoError(t, tx.Update(&legacyTransactionalLifecycleRecord{
		PK:      "USER#legacy-overlap",
		SK:      "PROFILE",
		Value:   "changed",
		Version: 7,
	}))
	require.Len(t, tx.writes, 1)
	require.NotNil(t, tx.writes[0].Update)

	update := tx.writes[0].Update
	expression := aws.ToString(update.UpdateExpression)
	t.Logf("legacy overlap update expression: %s", expression)
	require.ElementsMatch(t, []string{"value", "version", "updatedAt"},
		updateDocumentPaths(expression, update.ExpressionAttributeNames))
	require.Equal(t, 1, updatePathOccurrences(expression, update.ExpressionAttributeNames, "version"),
		"the library-managed version path must appear exactly once")
	require.Equal(t, 1, updatePathOccurrences(expression, update.ExpressionAttributeNames, "updatedAt"),
		"the library-managed updated_at path must appear exactly once")
	require.Zero(t, updatePathOccurrences(expression, update.ExpressionAttributeNames, "createdAt"),
		"created_at must not be selected by the implicit update")
	require.Equal(t, "#ver = :currentVer", aws.ToString(update.ConditionExpression))
}

func TestTransactionUpdateLegacyLocksZeroVersion(t *testing.T) {
	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&legacyTransactionalLifecycleRecord{}))

	tx := NewTransaction(&session.Session{}, registry, pkgTypes.NewConverter())
	require.NoError(t, tx.Update(&legacyTransactionalLifecycleRecord{
		PK:      "USER#legacy-version-zero",
		SK:      "PROFILE",
		Value:   "changed",
		Version: 0,
	}))
	require.Len(t, tx.writes, 1)

	update := tx.writes[0].Update
	require.NotNil(t, update)
	require.Equal(t, "#ver = :currentVer", aws.ToString(update.ConditionExpression))
	require.Contains(t, aws.ToString(update.UpdateExpression), "#ver = :newVer")
	require.Equal(t, "0", numericAttributeValue(t, update.ExpressionAttributeValues[":currentVer"]))
	require.Equal(t, "1", numericAttributeValue(t, update.ExpressionAttributeValues[":newVer"]))
}

func numericAttributeValue(t *testing.T, value dynamodbTypes.AttributeValue) string {
	t.Helper()
	number, ok := value.(*dynamodbTypes.AttributeValueMemberN)
	require.True(t, ok)
	return number.Value
}

func TestTransactionUpdateLegacyBuildsSetFromManagedOnlyAssignments(t *testing.T) {
	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&legacyManagedOnlyVersionedRecord{}))

	tx := NewTransaction(&session.Session{}, registry, pkgTypes.NewConverter())
	require.NoError(t, tx.Update(&legacyManagedOnlyVersionedRecord{
		PK:      "USER#legacy-managed-only",
		SK:      "PROFILE",
		Version: 7,
	}))
	require.Len(t, tx.writes, 1)

	update := tx.writes[0].Update
	require.NotNil(t, update)
	expression := aws.ToString(update.UpdateExpression)
	require.Equal(t, "SET #ver = :newVer, #upd = :updTime", expression)
	require.NotContains(t, expression, "SET ,")
	require.ElementsMatch(t, []string{"version", "updatedAt"}, updateDocumentPaths(expression, update.ExpressionAttributeNames))
	require.Equal(t, "#ver = :currentVer", aws.ToString(update.ConditionExpression))
}

func TestTransactionUpdateLegacyOmitsEmptySetClause(t *testing.T) {
	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&legacyCreatedAtOnlyRecord{}))

	tx := NewTransaction(&session.Session{}, registry, pkgTypes.NewConverter())
	err := tx.Update(&legacyCreatedAtOnlyRecord{
		PK: "USER#legacy-created-at-only",
		SK: "PROFILE",
	})
	require.EqualError(t, err, "no non-key fields to update")
	require.Empty(t, tx.writes, "an empty implicit update must not queue an invalid DynamoDB write")
}

func TestTransactionUpdateLegacyOverridesCallerSetUpdatedAtForImplicitRMW(t *testing.T) {
	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&legacyTransactionalLifecycleRecord{}))
	callerUpdatedAt := time.Date(2020, time.January, 2, 3, 4, 5, 0, time.UTC)
	record := &legacyTransactionalLifecycleRecord{
		PK:        "USER#legacy-caller-timestamp",
		SK:        "PROFILE",
		Value:     "changed",
		UpdatedAt: callerUpdatedAt,
	}

	// The legacy whole-model surface must remain read-modify-write compatible. Lifecycle
	// rejection is reserved for explicit named-field surfaces across every runtime; the
	// normative implicit surface skips caller values and lets the library refresh updated_at.
	legacy := NewTransaction(&session.Session{}, registry, pkgTypes.NewConverter())
	require.NoError(t, legacy.Update(record))
	require.Len(t, legacy.writes, 1)

	update := legacy.writes[0].Update
	require.NotNil(t, update)
	expression := aws.ToString(update.UpdateExpression)
	require.ElementsMatch(t, []string{"value", "version", "updatedAt"},
		updateDocumentPaths(expression, update.ExpressionAttributeNames))
	updatedAt, ok := update.ExpressionAttributeValues[":updTime"].(*dynamodbTypes.AttributeValueMemberS)
	require.True(t, ok)
	require.NotEqual(t, callerUpdatedAt.Format(time.RFC3339Nano), updatedAt.Value,
		"the caller-supplied updated_at must not win over the library timestamp")
}

func TestTransactionUpdateLegacyIgnoresCallerSetCreatedAtForImplicitRMW(t *testing.T) {
	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&legacyTransactionalLifecycleRecord{}))
	record := &legacyTransactionalLifecycleRecord{
		PK:        "USER#legacy-caller-created-at",
		SK:        "PROFILE",
		Value:     "changed",
		CreatedAt: time.Now(),
	}

	// Loaded models naturally carry created_at. The legacy implicit surface therefore
	// excludes it without error; only explicit named-field mutation is rejected.
	legacy := NewTransaction(&session.Session{}, registry, pkgTypes.NewConverter())
	require.NoError(t, legacy.Update(record))
	require.Len(t, legacy.writes, 1)

	update := legacy.writes[0].Update
	require.NotNil(t, update)
	expression := aws.ToString(update.UpdateExpression)
	require.ElementsMatch(t, []string{"value", "version", "updatedAt"},
		updateDocumentPaths(expression, update.ExpressionAttributeNames))
	require.Zero(t, updatePathOccurrences(expression, update.ExpressionAttributeNames, "createdAt"),
		"caller-supplied created_at must be excluded so the stored lifecycle value is preserved")
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
	require.ElementsMatch(t, []string{"value", "version", "updatedAt"}, attributeNames(update.ExpressionAttributeNames))
	require.Len(t, update.ExpressionAttributeValues, 4)
	require.Zero(t, updatePathOccurrences(expression, update.ExpressionAttributeNames, "createdAt"),
		"a zero created_at must not overwrite the stored lifecycle timestamp")
	require.Equal(t, 1, updatePathOccurrences(expression, update.ExpressionAttributeNames, "version"),
		"the persisted zero version must be incremented by the implicit update")
	require.Equal(t, 1, updatePathOccurrences(expression, update.ExpressionAttributeNames, "updatedAt"))
}

func updatePathOccurrences(expression string, names map[string]string, path string) int {
	count := 0
	for _, attribute := range updateDocumentPaths(expression, names) {
		if attribute == path {
			count++
		}
	}
	return count
}

func updateDocumentPaths(expression string, names map[string]string) []string {
	setExpression := strings.TrimSpace(strings.TrimPrefix(expression, "SET"))
	if setExpression == "" {
		return nil
	}

	assignments := strings.Split(setExpression, ",")
	paths := make([]string, 0, len(assignments))
	for _, assignment := range assignments {
		parts := strings.SplitN(assignment, "=", 2)
		if len(parts) != 2 {
			continue
		}
		path := strings.TrimSpace(parts[0])
		if attribute, ok := names[path]; ok {
			path = attribute
		}
		paths = append(paths, path)
	}
	return paths
}
