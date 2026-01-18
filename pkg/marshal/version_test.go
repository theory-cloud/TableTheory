package marshal

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVersionNumberFromValue(t *testing.T) {
	t.Run("signed ints", func(t *testing.T) {
		got, err := versionNumberFromValue(reflect.ValueOf(int64(-7)))
		require.NoError(t, err)
		require.Equal(t, int64(-7), got)

		got, err = versionNumberFromValue(reflect.ValueOf(int(5)))
		require.NoError(t, err)
		require.Equal(t, int64(5), got)
	})

	t.Run("unsigned ints", func(t *testing.T) {
		got, err := versionNumberFromValue(reflect.ValueOf(uint64(9)))
		require.NoError(t, err)
		require.Equal(t, int64(9), got)

		got, err = versionNumberFromValue(reflect.ValueOf(uint(0)))
		require.NoError(t, err)
		require.Equal(t, int64(0), got)
	})

	t.Run("rejects overflow", func(t *testing.T) {
		_, err := versionNumberFromValue(reflect.ValueOf(maxInt64AsUint64 + 1))
		require.Error(t, err)
	})

	t.Run("rejects unsupported kinds", func(t *testing.T) {
		_, err := versionNumberFromValue(reflect.ValueOf("nope"))
		require.Error(t, err)
	})
}

func TestMarshalItem_VersionField_NonInt64Kinds(t *testing.T) {
	t.Run("handles int kind", func(t *testing.T) {
		type item struct {
			ID      string
			Version int
		}

		typ := reflect.TypeOf(item{})
		metadata := createMetadata(
			createFieldMetadata(typ, "ID", "id", reflect.TypeOf("")),
			createFieldMetadata(typ, "Version", "version", reflect.TypeOf(int(0)), withVersion()),
		)

		safe := NewSafeMarshaler()
		out, err := safe.MarshalItem(item{ID: "id-1", Version: 0}, metadata)
		require.NoError(t, err)
		require.Equal(t, "0", requireAVN(t, out["version"]).Value)

		unsafeMarshaler := New(nil)
		out, err = unsafeMarshaler.MarshalItem(item{ID: "id-1", Version: 3}, metadata)
		require.NoError(t, err)
		require.Equal(t, "3", requireAVN(t, out["version"]).Value)
	})

	t.Run("errors on unsupported version kind", func(t *testing.T) {
		type item struct {
			ID      string
			Version string
		}

		typ := reflect.TypeOf(item{})
		metadata := createMetadata(
			createFieldMetadata(typ, "ID", "id", reflect.TypeOf("")),
			createFieldMetadata(typ, "Version", "version", reflect.TypeOf(""), withVersion()),
		)

		safe := NewSafeMarshaler()
		_, err := safe.MarshalItem(item{ID: "id-1", Version: "x"}, metadata)
		require.Error(t, err)

		unsafeMarshaler := New(nil)
		_, err = unsafeMarshaler.MarshalItem(item{ID: "id-1", Version: "x"}, metadata)
		require.Error(t, err)
	})
}
