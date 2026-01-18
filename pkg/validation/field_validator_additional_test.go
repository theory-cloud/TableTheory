package validation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateFieldPartListSyntax(t *testing.T) {
	tests := []struct {
		name    string
		part    string
		wantErr string
	}{
		{
			name: "valid index access",
			part: "items[0]",
		},
		{
			name: "valid multi-digit index",
			part: "results[123]",
		},
		{
			name:    "missing index digits",
			part:    "items[]",
			wantErr: "list index must be a number",
		},
		{
			name:    "trailing characters after index",
			part:    "items[0]extra",
			wantErr: "unexpected characters after list index",
		},
		{
			name:    "invalid field prefix",
			part:    "9items[0]",
			wantErr: "field name part must start with letter or underscore",
		},
		{
			name:    "empty field part",
			part:    "",
			wantErr: "field part cannot be empty",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateFieldPart(tc.part)
			if tc.wantErr == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
			}
		})
	}
}

func TestValidateValueCollectionErrors(t *testing.T) {
	t.Run("slice reports invalid member", func(t *testing.T) {
		err := ValidateValue([]any{"safe", "javascript:alert(1)"})
		require.Error(t, err)

		var secErr *SecurityError
		require.ErrorAs(t, err, &secErr)
		assert.Equal(t, "InvalidValue", secErr.Type)
		assert.Contains(t, secErr.Detail, "invalid item in collection")
	})

	t.Run("typed slice length limit enforced", func(t *testing.T) {
		values := make([]int, 101)
		err := ValidateValue(values)
		require.Error(t, err)

		var secErr *SecurityError
		require.ErrorAs(t, err, &secErr)
		assert.Equal(t, "InvalidValue", secErr.Type)
		assert.Contains(t, secErr.Detail, "slice value exceeds maximum length")
	})
}

func TestValidateValueMapEdgeCases(t *testing.T) {
	t.Run("map with int keys is allowed", func(t *testing.T) {
		// map[int]string is now allowed - DynamoDB marshaler will convert int keys to strings
		err := ValidateValue(map[int]string{1: "one"})
		assert.NoError(t, err)
	})

	t.Run("typed map propagates key validation", func(t *testing.T) {
		err := ValidateValue(map[string]string{"delete_flag": "safe"})
		assert.NoError(t, err)

		err = ValidateValue(map[string]string{"delete;drop": "bad"})
		require.Error(t, err)

		var secErr *SecurityError
		require.ErrorAs(t, err, &secErr)
		assert.Equal(t, "InvalidValue", secErr.Type)
		assert.Contains(t, secErr.Detail, "invalid map key")
	})
}

func TestValidateValueBasicUnsupportedType(t *testing.T) {
	// Structs are now allowed - they will be marshaled as DynamoDB maps
	err := ValidateValue(struct{}{})
	assert.NoError(t, err)
}
