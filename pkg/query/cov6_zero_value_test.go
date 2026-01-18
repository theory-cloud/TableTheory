package query

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestIsZeroValue_CoversAllKinds_COV6(t *testing.T) {
	t.Run("string/slice/map/array", func(t *testing.T) {
		require.True(t, isZeroValue(reflect.ValueOf("")))
		require.False(t, isZeroValue(reflect.ValueOf("x")))

		require.True(t, isZeroValue(reflect.ValueOf([]int(nil))))
		require.True(t, isZeroValue(reflect.ValueOf([]int{})))
		require.False(t, isZeroValue(reflect.ValueOf([]int{1})))

		require.True(t, isZeroValue(reflect.ValueOf(map[string]int(nil))))
		require.True(t, isZeroValue(reflect.ValueOf(map[string]int{})))
		require.False(t, isZeroValue(reflect.ValueOf(map[string]int{"a": 1})))

		require.True(t, isZeroValue(reflect.ValueOf([0]byte{})))
		require.False(t, isZeroValue(reflect.ValueOf([1]byte{1})))
	})

	t.Run("bool and numbers", func(t *testing.T) {
		require.True(t, isZeroValue(reflect.ValueOf(false)))
		require.False(t, isZeroValue(reflect.ValueOf(true)))

		require.True(t, isZeroValue(reflect.ValueOf(int64(0))))
		require.False(t, isZeroValue(reflect.ValueOf(int64(1))))

		require.True(t, isZeroValue(reflect.ValueOf(uint64(0))))
		require.False(t, isZeroValue(reflect.ValueOf(uint64(1))))

		require.True(t, isZeroValue(reflect.ValueOf(float64(0))))
		require.False(t, isZeroValue(reflect.ValueOf(float64(0.1))))
	})

	t.Run("ptr and interface", func(t *testing.T) {
		var p *int
		require.True(t, isZeroValue(reflect.ValueOf(p)))
		v := 1
		p = &v
		require.False(t, isZeroValue(reflect.ValueOf(p)))

		type wrap struct {
			I any
		}
		w := wrap{}
		require.True(t, isZeroValue(reflect.ValueOf(w).FieldByName("I")))
		w.I = "x"
		require.False(t, isZeroValue(reflect.ValueOf(w).FieldByName("I")))
	})

	t.Run("time.Time and general structs", func(t *testing.T) {
		var tm time.Time
		require.True(t, isZeroValue(reflect.ValueOf(tm)))
		require.False(t, isZeroValue(reflect.ValueOf(time.Now())))

		type nested struct {
			B string
			A int
		}
		require.True(t, isZeroValue(reflect.ValueOf(nested{})))
		require.False(t, isZeroValue(reflect.ValueOf(nested{A: 1})))
	})

	t.Run("default kinds (chan/func)", func(t *testing.T) {
		var ch chan int
		require.True(t, isZeroValue(reflect.ValueOf(ch)))
		ch = make(chan int)
		require.False(t, isZeroValue(reflect.ValueOf(ch)))

		var fn func()
		require.True(t, isZeroValue(reflect.ValueOf(fn)))
		fn = func() {}
		require.False(t, isZeroValue(reflect.ValueOf(fn)))
	})
}
