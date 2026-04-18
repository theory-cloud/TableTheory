package reflectutil_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/internal/reflectutil"
	"github.com/theory-cloud/tabletheory/pkg/naming"
)

func TestBuildVisibleFieldPlanIncludesPromotedFields(t *testing.T) {
	type BaseObject struct {
		ID   string
		Type string
		To   []string
	}
	//nolint:govet // Field order mirrors the anonymous-embed contract fixture under test.
	type Activity struct {
		BaseObject
		Actor  string
		Object any
	}

	plans, err := reflectutil.BuildVisibleFieldPlan(reflect.TypeOf(Activity{}), legacyAliasResolver(naming.CamelCase))
	require.NoError(t, err)

	require.Equal(t, []string{"ID", "Type", "To", "Actor", "Object"}, planNames(plans))
	require.Equal(t, []int{0, 0}, plans[0].IndexPath)
	require.Equal(t, []int{0, 1}, plans[1].IndexPath)
	require.Equal(t, []int{0, 2}, plans[2].IndexPath)
	require.Equal(t, []int{1}, plans[3].IndexPath)
	require.Equal(t, []int{2}, plans[4].IndexPath)

	for _, idx := range []int{0, 1, 2} {
		require.Len(t, plans[idx].LegacyContainers, 1)
		require.Equal(t, "BaseObject", plans[idx].LegacyContainers[0].Field.Name)
		require.Equal(t, []int{0}, plans[idx].LegacyContainers[0].IndexPath)
		require.Equal(t, []string{"baseObject", "BaseObject"}, plans[idx].LegacyContainers[0].Aliases)
	}

	require.Empty(t, plans[3].LegacyContainers)
	require.Empty(t, plans[4].LegacyContainers)
}

func TestBuildVisibleFieldPlanSupportsNestedAnonymousContainers(t *testing.T) {
	type Envelope struct {
		ID string
	}
	type BaseObject struct {
		Envelope
	}
	type Activity struct {
		BaseObject
		Actor string
	}

	plans, err := reflectutil.BuildVisibleFieldPlan(reflect.TypeOf(Activity{}), legacyAliasResolver(naming.SnakeCase))
	require.NoError(t, err)

	require.Equal(t, []string{"ID", "Actor"}, planNames(plans))
	require.Equal(t, []int{0, 0, 0}, plans[0].IndexPath)
	require.Len(t, plans[0].LegacyContainers, 2)
	require.Equal(t, "BaseObject", plans[0].LegacyContainers[0].Field.Name)
	require.Equal(t, []int{0}, plans[0].LegacyContainers[0].IndexPath)
	require.Equal(t, []string{"base_object", "BaseObject"}, plans[0].LegacyContainers[0].Aliases)
	require.Equal(t, "Envelope", plans[0].LegacyContainers[1].Field.Name)
	require.Equal(t, []int{0, 0}, plans[0].LegacyContainers[1].IndexPath)
	require.Equal(t, []string{"envelope", "Envelope"}, plans[0].LegacyContainers[1].Aliases)
}

func TestBuildVisibleFieldPlanRespectsGoFieldShadowing(t *testing.T) {
	type BaseObject struct {
		ID   string
		Type string
	}
	type Activity struct {
		Type string
		BaseObject
		Actor string
	}

	plans, err := reflectutil.BuildVisibleFieldPlan(reflect.TypeOf(Activity{}), legacyAliasResolver(naming.CamelCase))
	require.NoError(t, err)

	require.Equal(t, []string{"Type", "ID", "Actor"}, planNames(plans))
	require.Equal(t, []int{0}, plans[0].IndexPath)
	require.Equal(t, []int{1, 0}, plans[1].IndexPath)
	require.Len(t, plans[1].LegacyContainers, 1)
	require.Equal(t, []string{"baseObject", "BaseObject"}, plans[1].LegacyContainers[0].Aliases)
}

func TestBuildVisibleFieldPlanSkipsUnexportedAnonymousStructContainers(t *testing.T) {
	type baseObject struct {
		ID string
	}
	type Activity struct {
		baseObject
		Actor string
	}

	plans, err := reflectutil.BuildVisibleFieldPlan(reflect.TypeOf(Activity{}), legacyAliasResolver(naming.CamelCase))
	require.NoError(t, err)

	require.Equal(t, []string{"Actor"}, planNames(plans))
}

func TestBuildVisibleFieldPlanPropagatesAliasResolverErrors(t *testing.T) {
	type BaseObject struct {
		ID string
	}
	type Activity struct {
		BaseObject
	}

	_, err := reflectutil.BuildVisibleFieldPlan(reflect.TypeOf(Activity{}), func(field reflect.StructField) ([]string, bool, error) {
		return nil, false, fmt.Errorf("boom: %s", field.Name)
	})
	require.ErrorContains(t, err, "boom: BaseObject")
}

func TestBuildVisibleFieldPlanRejectsNonStructTypes(t *testing.T) {
	_, err := reflectutil.BuildVisibleFieldPlan(reflect.TypeOf(0), nil)
	require.ErrorContains(t, err, "model type must be a struct or pointer to struct")
}

func legacyAliasResolver(convention naming.Convention) reflectutil.LegacyAliasResolver {
	return func(field reflect.StructField) ([]string, bool, error) {
		return []string{naming.ConvertAttrName(field.Name, convention), field.Name, naming.ConvertAttrName(field.Name, convention), ""}, false, nil
	}
}

func planNames(plans []reflectutil.VisibleFieldPlan) []string {
	names := make([]string, 0, len(plans))
	for _, plan := range plans {
		names = append(names, plan.Field.Name)
	}
	return names
}
