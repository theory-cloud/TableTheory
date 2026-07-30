package driver

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	theorydbErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
)

func TestMapError_EncryptionCodes(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want ErrorCode
	}{
		{
			name: "encryption not configured",
			err:  fmt.Errorf("wrapped: %w", theorydbErrors.ErrEncryptionNotConfigured),
			want: ErrEncryptionNotConfigured,
		},
		{
			name: "encrypted field not queryable",
			err:  fmt.Errorf("wrapped: %w", theorydbErrors.ErrEncryptedFieldNotQueryable),
			want: ErrEncryptedFieldNotQueryable,
		},
		{
			name: "invalid encrypted envelope",
			err:  fmt.Errorf("wrapped: %w", theorydbErrors.ErrInvalidEncryptedEnvelope),
			want: ErrInvalidEncryptedEnvelope,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, MapError(tt.err))
		})
	}
}
