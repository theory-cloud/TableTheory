package theorydb

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/core"
)

func TestWriteCountResult_COV7(t *testing.T) {
	t.Run("nil_destination_errors", func(t *testing.T) {
		require.Error(t, writeCountResult(nil, 1, 0))
	})

	t.Run("non_pointer_destination_errors", func(t *testing.T) {
		var dest int64
		require.Error(t, writeCountResult(dest, 1, 0))
	})

	t.Run("int_pointer_destination", func(t *testing.T) {
		var dest int
		require.NoError(t, writeCountResult(&dest, 42, 0))
		require.Equal(t, 42, dest)
	})

	t.Run("uint_pointer_destination_negative_errors", func(t *testing.T) {
		var dest uint64
		require.Error(t, writeCountResult(&dest, -1, 0))
	})

	t.Run("struct_destination_sets_fields", func(t *testing.T) {
		type counts struct {
			Count        uint
			ScannedCount int64
		}

		var dest counts
		require.NoError(t, writeCountResult(&dest, 5, -3))
		require.EqualValues(t, 5, dest.Count)
		require.EqualValues(t, -3, dest.ScannedCount)
	})

	t.Run("struct_destination_uint_field_ignores_negative", func(t *testing.T) {
		type counts struct {
			Count        int64
			ScannedCount uint
		}

		var dest counts
		require.NoError(t, writeCountResult(&dest, 5, -3))
		require.EqualValues(t, 5, dest.Count)
		require.EqualValues(t, 0, dest.ScannedCount)
	})

	t.Run("unsupported_destination_type_errors", func(t *testing.T) {
		var dest string
		require.Error(t, writeCountResult(&dest, 1, 0))
	})
}

func TestCompiledQueryLimit_COV7(t *testing.T) {
	limit, ok := compiledQueryLimit(nil)
	require.False(t, ok)
	require.Zero(t, limit)

	limit, ok = compiledQueryLimit(&core.CompiledQuery{})
	require.False(t, ok)
	require.Zero(t, limit)

	limitZero := int32(0)
	limit, ok = compiledQueryLimit(&core.CompiledQuery{Limit: &limitZero})
	require.True(t, ok)
	require.Zero(t, limit)

	limitPositive := int32(12)
	limit, ok = compiledQueryLimit(&core.CompiledQuery{Limit: &limitPositive})
	require.True(t, ok)
	require.Equal(t, 12, limit)
}
