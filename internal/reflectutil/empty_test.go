package reflectutil_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/internal/reflectutil"
)

func TestIsEmpty(t *testing.T) {
	type inner struct {
		B string
		A int
	}
	type outer struct {
		Inner inner
	}

	now := time.Now()

	var nilIntPtr *int

	cases := []struct {
		name string
		v    reflect.Value
		want bool
	}{
		{name: "invalid value", v: reflect.Value{}, want: true},

		{name: "empty array", v: reflect.ValueOf([2]int{0, 0}), want: true},
		{name: "non-empty array", v: reflect.ValueOf([2]int{0, 1}), want: false},

		{name: "empty map", v: reflect.ValueOf(map[string]int{}), want: true},
		{name: "non-empty map", v: reflect.ValueOf(map[string]int{"a": 1}), want: false},

		{name: "empty slice", v: reflect.ValueOf([]int{}), want: true},
		{name: "non-empty slice", v: reflect.ValueOf([]int{0}), want: false},

		{name: "empty string", v: reflect.ValueOf(""), want: true},
		{name: "non-empty string", v: reflect.ValueOf("x"), want: false},

		{name: "false bool", v: reflect.ValueOf(false), want: true},
		{name: "true bool", v: reflect.ValueOf(true), want: false},

		{name: "zero int", v: reflect.ValueOf(0), want: true},
		{name: "non-zero int", v: reflect.ValueOf(1), want: false},

		{name: "zero uint", v: reflect.ValueOf(uint(0)), want: true},
		{name: "non-zero uint", v: reflect.ValueOf(uint(1)), want: false},

		{name: "zero float", v: reflect.ValueOf(0.0), want: true},
		{name: "non-zero float", v: reflect.ValueOf(0.1), want: false},

		{name: "nil pointer", v: reflect.ValueOf(nilIntPtr), want: true},
		{name: "non-nil pointer", v: reflect.ValueOf(new(int)), want: false},

		{name: "empty struct", v: reflect.ValueOf(inner{}), want: true},
		{name: "non-empty struct", v: reflect.ValueOf(inner{B: "x"}), want: false},
		{name: "empty nested struct", v: reflect.ValueOf(outer{}), want: true},
		{name: "non-empty nested struct", v: reflect.ValueOf(outer{Inner: inner{A: 1}}), want: false},

		{name: "zero time", v: reflect.ValueOf(time.Time{}), want: true},
		{name: "non-zero time", v: reflect.ValueOf(now), want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, reflectutil.IsEmpty(tc.v))
		})
	}

	t.Run("interface kind: nil", func(t *testing.T) {
		var i any
		v := reflect.ValueOf(&i).Elem()
		require.Equal(t, reflect.Interface, v.Kind())
		require.True(t, reflectutil.IsEmpty(v))
	})

	t.Run("interface kind: non-nil", func(t *testing.T) {
		var i any = 0
		v := reflect.ValueOf(&i).Elem()
		require.Equal(t, reflect.Interface, v.Kind())
		require.False(t, reflectutil.IsEmpty(v))
	})

	t.Run("interface kind: typed nil pointer", func(t *testing.T) {
		var p *int
		var i any = p
		v := reflect.ValueOf(&i).Elem()
		require.Equal(t, reflect.Interface, v.Kind())
		require.False(t, reflectutil.IsEmpty(v))
	})
}
