package reflectutil_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v4/internal/reflectutil"
	"github.com/theory-cloud/tabletheory/v4/pkg/naming"
)

// exclusionResolver mirrors the resolution rule the query/expr/types decode
// paths use: a "-" theorydb or json tag on a container excludes the subtree.
func exclusionResolver(field reflect.StructField) ([]string, bool, error) {
	if field.Tag.Get("theorydb") == "-" || field.Tag.Get("json") == "-" {
		return nil, true, nil
	}
	return []string{naming.ConvertAttrName(field.Name, naming.CamelCase), field.Name}, false, nil
}

//nolint:govet // Field order mirrors the production fixture under test.
type ExclusionPrivileges struct {
	IsAdmin bool
	Secret  string
}

//nolint:govet // Field order and the anonymous embeds mirror the production fixture under test.
type excludedByTheorydb struct {
	ExclusionPrivileges `theorydb:"-"`
	Name                string
}

//nolint:govet
type excludedByJSON struct {
	ExclusionPrivileges `json:"-"`
	Name                string
}

//nolint:govet
type visibleEmbed struct {
	ExclusionPrivileges
	Name string
}

func TestBuildVisibleFieldPlanExcludesTaggedAnonymousContainers(t *testing.T) {
	for _, tc := range []struct { //nolint:govet // Field alignment is irrelevant for two-field test rows.
		model any
		name  string
	}{
		{excludedByTheorydb{}, "theorydb"},
		{excludedByJSON{}, "json"},
	} {
		plans, err := reflectutil.BuildVisibleFieldPlan(reflect.TypeOf(tc.model), exclusionResolver)
		require.NoError(t, err, tc.name)
		require.Equal(t, []string{"Name"}, planNames(plans), "%s excluded embed must not expose promoted fields", tc.name)
	}

	visible, err := reflectutil.BuildVisibleFieldPlan(reflect.TypeOf(visibleEmbed{}), exclusionResolver)
	require.NoError(t, err)
	require.Equal(t, []string{"IsAdmin", "Secret", "Name"}, planNames(visible))
	require.Len(t, visible[0].LegacyContainers, 1)
	require.Equal(t, "ExclusionPrivileges", visible[0].LegacyContainers[0].Field.Name)
}
