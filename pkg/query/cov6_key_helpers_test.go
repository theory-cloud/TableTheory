package query

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFillKeyValuesByName(t *testing.T) {
	type keyModel struct {
		PK string
		SK string
	}

	q := &Query{}

	t.Run("sets pk and sk when present", func(t *testing.T) {
		modelValue := reflect.ValueOf(keyModel{PK: "p", SK: "s"})

		var pkValue any
		pkFound := false
		var skValue any
		skFound := false

		q.fillKeyValuesByName(modelValue, "PK", "SK", &pkValue, &pkFound, &skValue, &skFound)

		assert.True(t, pkFound)
		assert.Equal(t, "p", pkValue)
		assert.True(t, skFound)
		assert.Equal(t, "s", skValue)
	})

	t.Run("does not override existing values", func(t *testing.T) {
		modelValue := reflect.ValueOf(keyModel{PK: "new-pk", SK: "new-sk"})

		pkValue := any("existing-pk")
		pkFound := true
		skValue := any("existing-sk")
		skFound := true

		q.fillKeyValuesByName(modelValue, "PK", "SK", &pkValue, &pkFound, &skValue, &skFound)

		assert.Equal(t, "existing-pk", pkValue)
		assert.Equal(t, "existing-sk", skValue)
	})

	t.Run("skips zero values and empty sk name", func(t *testing.T) {
		modelValue := reflect.ValueOf(keyModel{})

		var pkValue any
		pkFound := false
		var skValue any
		skFound := false

		q.fillKeyValuesByName(modelValue, "PK", "", &pkValue, &pkFound, &skValue, &skFound)

		assert.False(t, pkFound)
		assert.Nil(t, pkValue)
		assert.False(t, skFound)
		assert.Nil(t, skValue)
	})
}

func TestTTLUnixSecondsIfTime(t *testing.T) {
	t.Run("returns value when field is not time.Time", func(t *testing.T) {
		out, err := ttlUnixSecondsIfTime("ttl", reflect.ValueOf(123), "x")
		require.NoError(t, err)
		assert.Equal(t, "x", out)
	})

	t.Run("returns value when time is zero", func(t *testing.T) {
		out, err := ttlUnixSecondsIfTime("ttl", reflect.ValueOf(time.Time{}), "x")
		require.NoError(t, err)
		assert.Equal(t, "x", out)
	})

	t.Run("converts time.Time to unix seconds", func(t *testing.T) {
		ttlTime := time.Unix(123, 0).UTC()
		out, err := ttlUnixSecondsIfTime("ttl", reflect.ValueOf(ttlTime), ttlTime)
		require.NoError(t, err)
		assert.Equal(t, int64(123), out)
	})

	t.Run("errors when value is not time.Time for time.Time field", func(t *testing.T) {
		ttlTime := time.Unix(123, 0).UTC()
		_, err := ttlUnixSecondsIfTime("ttl", reflect.ValueOf(ttlTime), "not-a-time")
		require.Error(t, err)
	})
}
