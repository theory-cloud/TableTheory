package numutil_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v4/internal/numutil"
)

func TestParseNumberPreservesExactValues(t *testing.T) {
	cases := []struct { //nolint:govet // Positional fixture rows; field alignment is irrelevant for test data.
		in   string
		want any
	}{
		{"0", int64(0)},
		{"42", int64(42)},
		{"-7", int64(-7)},
		{"9223372036854775807", int64(9223372036854775807)},
		{"2.5", 2.5},
		{"-3.25", -3.25},
		{"9223372036854775809", json.Number("9223372036854775809")},
		{"0.123456789012345678", json.Number("0.123456789012345678")},
		{"1e400", json.Number("1e400")},
	}

	for _, tc := range cases {
		got, err := numutil.ParseNumber(tc.in)
		require.NoError(t, err, tc.in)
		require.Equal(t, tc.want, got, tc.in)
	}

	_, err := numutil.ParseNumber("not-a-number")
	require.Error(t, err)
}

func TestParseNumberSet(t *testing.T) {
	got, err := numutil.ParseNumberSet([]string{"1", "2.5", "9223372036854775809"})
	require.NoError(t, err)
	require.Equal(t, []any{int64(1), 2.5, json.Number("9223372036854775809")}, got)

	_, err = numutil.ParseNumberSet([]string{"1", "bad"})
	require.Error(t, err)
}
