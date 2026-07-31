package query

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	theorydbErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
)

func TestUpdateBuilder_JSONNormalizationErrors_COV7(t *testing.T) {
	q := &Query{
		metadata: cov5Metadata{
			attributes: map[string]*core.AttributeMetadata{
				"payload": {
					Name:         "Payload",
					DynamoDBName: "payload",
					Tags:         map[string]string{"json": ""},
				},
			},
		},
	}

	invalidJSONCarrier := make(chan int)
	tests := []struct {
		name      string
		apply     func(*UpdateBuilder)
		wantError string
	}{
		{
			name: "Set",
			apply: func(ub *UpdateBuilder) {
				ub.Set("payload", invalidJSONCarrier)
			},
			wantError: "Set(payload)",
		},
		{
			name: "SetIfNotExists",
			apply: func(ub *UpdateBuilder) {
				ub.SetIfNotExists("payload", nil, invalidJSONCarrier)
			},
			wantError: "SetIfNotExists(payload)",
		},
		{
			name: "AppendToList",
			apply: func(ub *UpdateBuilder) {
				ub.AppendToList("payload", invalidJSONCarrier)
			},
			wantError: "AppendToList(payload)",
		},
		{
			name: "PrependToList",
			apply: func(ub *UpdateBuilder) {
				ub.PrependToList("payload", invalidJSONCarrier)
			},
			wantError: "PrependToList(payload)",
		},
		{
			name: "SetListElement",
			apply: func(ub *UpdateBuilder) {
				ub.SetListElement("payload", 1, invalidJSONCarrier)
			},
			wantError: "SetListElement(payload)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ub := NewUpdateBuilder(q).(*UpdateBuilder)
			tt.apply(ub)
			require.Error(t, ub.buildErr)
			require.Contains(t, ub.buildErr.Error(), tt.wantError)
		})
	}
}

func TestUpdateBuilder_EncryptedConditionGuardsAndVersionField_COV7(t *testing.T) {
	q := &Query{
		metadata: cov5Metadata{
			attributes: map[string]*core.AttributeMetadata{
				"secret": {
					Name:         "Secret",
					DynamoDBName: "secret",
					Tags:         map[string]string{"encrypted": ""},
				},
			},
			versionField: "Revision",
		},
	}

	t.Run("Condition rejects encrypted fields", func(t *testing.T) {
		ub := NewUpdateBuilder(q).(*UpdateBuilder)
		ub.Condition("secret", "=", "top-secret")

		require.ErrorIs(t, ub.buildErr, theorydbErrors.ErrEncryptedFieldNotQueryable)
		require.Contains(t, ub.buildErr.Error(), "Secret")
	})

	t.Run("OrCondition rejects encrypted fields", func(t *testing.T) {
		ub := NewUpdateBuilder(q).(*UpdateBuilder)
		ub.OrCondition("secret", "=", "top-secret")

		require.ErrorIs(t, ub.buildErr, theorydbErrors.ErrEncryptedFieldNotQueryable)
		require.Contains(t, ub.buildErr.Error(), "Secret")
	})

	t.Run("ConditionVersion uses metadata version field", func(t *testing.T) {
		ub := NewUpdateBuilder(q).(*UpdateBuilder)
		ub.ConditionVersion(7)

		require.NoError(t, ub.buildErr)
		require.Len(t, ub.conditions, 1)
		require.Equal(t, "Revision", ub.conditions[0].field)
		require.Equal(t, "=", ub.conditions[0].operator)
		require.EqualValues(t, 7, ub.conditions[0].value)
	})
}
