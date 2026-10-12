package anonymous

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

type hiddenEmbed struct {
	IsAdmin bool
}

//nolint:govet // Field order and the anonymous embed mirror the production fixture under test.
type flatExcludedEmbed struct {
	hiddenEmbed `theorydb:"-"`
	Name        string
}

//nolint:govet // Field order and the anonymous embed mirror the production fixture under test.
type flatVisibleEmbed struct {
	hiddenEmbed
	Name string
}

func exclusionResolve(field reflect.StructField) (string, bool) {
	return field.Name, field.Tag.Get("theorydb") == "-" || field.Tag.Get("json") == "-"
}

func TestMarshalContainerNamesForFieldHonorsExclusionInFlatMode(t *testing.T) {
	typ := reflect.TypeOf(flatExcludedEmbed{})

	for _, flatten := range []bool{true, false} {
		names, skip := MarshalContainerNamesForField(typ, []int{0, 0}, exclusionResolve, flatten)
		require.True(t, skip, "excluded ancestor must suppress the field when flatten=%v", flatten)
		require.Nil(t, names)
	}
}

func TestMarshalContainerNamesForFieldKeepsOrdinaryEmbedInFlatMode(t *testing.T) {
	typ := reflect.TypeOf(flatVisibleEmbed{})

	names, skip := MarshalContainerNamesForField(typ, []int{0, 0}, exclusionResolve, true)
	require.False(t, skip)
	require.Nil(t, names, "flat mode writes promoted fields without a nested container")

	names, skip = MarshalContainerNamesForField(typ, []int{0, 0}, exclusionResolve, false)
	require.False(t, skip)
	require.Equal(t, []string{"hiddenEmbed"}, names)
}
