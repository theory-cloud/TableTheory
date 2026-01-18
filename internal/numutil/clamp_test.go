package numutil_test

import (
	"math"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/internal/numutil"
)

func TestClampIntToInt32(t *testing.T) {
	require.Equal(t, int32(0), numutil.ClampIntToInt32(0))
	require.Equal(t, int32(123), numutil.ClampIntToInt32(123))
	require.Equal(t, int32(-123), numutil.ClampIntToInt32(-123))

	require.Equal(t, int32(math.MaxInt32), numutil.ClampIntToInt32(math.MaxInt32))
	require.Equal(t, int32(math.MinInt32), numutil.ClampIntToInt32(math.MinInt32))

	if strconv.IntSize == 64 {
		require.Equal(t, int32(math.MaxInt32), numutil.ClampIntToInt32(int(int64(math.MaxInt32)+1)))
		require.Equal(t, int32(math.MinInt32), numutil.ClampIntToInt32(int(int64(math.MinInt32)-1)))
	}
}
