package transaction

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
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
