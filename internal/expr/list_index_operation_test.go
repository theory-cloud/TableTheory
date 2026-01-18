package expr

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/validation"
)

func TestParseListIndexOperation(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		fieldName, index, ok, err := parseListIndexOperation("items[12]")
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, "items", fieldName)
		require.Equal(t, 12, index)
	})

	t.Run("not a list index operation", func(t *testing.T) {
		_, _, ok, err := parseListIndexOperation("items")
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("invalid bracket syntax", func(t *testing.T) {
		_, _, ok, err := parseListIndexOperation("[0]")
		require.False(t, ok)
		var secErr *validation.SecurityError
		require.ErrorAs(t, err, &secErr)
	})

	t.Run("empty index", func(t *testing.T) {
		_, _, ok, err := parseListIndexOperation("items[]")
		require.False(t, ok)
		var secErr *validation.SecurityError
		require.ErrorAs(t, err, &secErr)
	})

	t.Run("non-numeric index", func(t *testing.T) {
		_, _, ok, err := parseListIndexOperation("items[x]")
		require.False(t, ok)
		var secErr *validation.SecurityError
		require.ErrorAs(t, err, &secErr)
	})

	t.Run("index out of range", func(t *testing.T) {
		// Keep the total field <= validation.MaxFieldNameLength so production paths can reach this.
		index := strings.Repeat("9", validation.MaxFieldNameLength-len("items[]"))
		_, _, ok, err := parseListIndexOperation("items[" + index + "]")
		require.False(t, ok)
		var secErr *validation.SecurityError
		require.ErrorAs(t, err, &secErr)
	})
}
