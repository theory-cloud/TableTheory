package query

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v3/internal/reflectutil"
)

func TestOmitEmptyPredicate_CoversFixedArrays_COV6(t *testing.T) {
	require.False(t, reflectutil.IsEmpty(reflect.ValueOf([2]int{0, 0})))
	require.False(t, reflectutil.IsEmpty(reflect.ValueOf([2]int{0, 1})))
	require.True(t, reflectutil.IsEmpty(reflect.ValueOf([0]int{})))
}
