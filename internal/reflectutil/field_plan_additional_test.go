package reflectutil_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v3/internal/reflectutil"
)

func TestBuildVisibleFieldPlanRejectsInvalidTypes(t *testing.T) {
	_, err := reflectutil.BuildVisibleFieldPlan(nil, nil)
	require.Error(t, err)

	_, err = reflectutil.BuildVisibleFieldPlan(reflect.TypeOf(42), nil)
	require.Error(t, err)
}

func TestBuildVisibleFieldPlanSanitizesLegacyAliases(t *testing.T) {
	type BaseObject struct {
		ID string
	}

	//nolint:govet // Field order mirrors the anonymous-embed contract fixture under test.
	type Activity struct {
		BaseObject
		Actor string
	}

	plans, err := reflectutil.BuildVisibleFieldPlan(reflect.TypeOf(Activity{}), func(field reflect.StructField) ([]string, bool, error) {
		if field.Name != "BaseObject" {
			return nil, false, nil
		}
		return []string{"", "baseObject", "baseObject", "-", "BaseObject"}, false, nil
	})
	require.NoError(t, err)
	require.Len(t, plans, 2)
	require.Len(t, plans[0].LegacyContainers, 1)
	require.Equal(t, []string{"baseObject", "BaseObject"}, plans[0].LegacyContainers[0].Aliases)
}
