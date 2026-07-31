package fieldcodec

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"
)

func TestNormalizeJSONFieldValue_CoversCarrierAndStructuredTypes(t *testing.T) {
	t.Run("string carriers preserve text", func(t *testing.T) {
		text, err := NormalizeJSONFieldValue(reflect.TypeOf(""), "plain-text")
		require.NoError(t, err)
		require.Equal(t, "plain-text", text)

		text, err = NormalizeJSONFieldValue(reflect.TypeOf([]byte{}), []byte(`{"copied":true}`))
		require.NoError(t, err)
		require.Equal(t, `{"copied":true}`, text)

		text, err = NormalizeJSONFieldValue(reflect.TypeOf(""), map[string]any{"ok": true})
		require.NoError(t, err)
		require.Equal(t, `{"ok":true}`, text)

		bytesValue, err := NormalizeJSONFieldValue(reflect.TypeOf([]byte{}), json.RawMessage(`{"ok":true}`))
		require.NoError(t, err)
		require.Equal(t, `{"ok":true}`, bytesValue)

		var nilCarrier []byte
		text, err = NormalizeJSONFieldValue(reflect.TypeOf([]byte{}), nilCarrier)
		require.NoError(t, err)
		require.Nil(t, text)

		rawText, err := NormalizeJSONFieldValue(nil, []byte(`{"count":2}`))
		require.NoError(t, err)
		require.Equal(t, `{"count":2}`, rawText)
	})

	t.Run("structured values decode into generic json-compatible shapes", func(t *testing.T) {
		value, err := NormalizeJSONFieldValue(reflect.TypeOf(map[string]any{}), `{"count":2,"ratio":1.5,"flags":[true,null]}`)
		require.NoError(t, err)

		decoded, ok := value.(map[string]any)
		require.True(t, ok)
		require.Equal(t, int64(2), decoded["count"])
		require.Equal(t, 1.5, decoded["ratio"])

		flags, ok := decoded["flags"].([]any)
		require.True(t, ok)
		require.Equal(t, true, flags[0])
		require.Nil(t, flags[1])

		listValue, err := NormalizeJSONFieldValue(reflect.TypeOf([]any{}), json.RawMessage(`[1,"two",false]`))
		require.NoError(t, err)
		require.Equal(t, []any{int64(1), "two", false}, listValue)
	})

	t.Run("invalid structured input returns an error", func(t *testing.T) {
		_, err := NormalizeJSONFieldValue(reflect.TypeOf(map[string]any{}), "")
		require.Error(t, err)

		_, err = NormalizeJSONFieldValue(reflect.TypeOf(map[string]any{}), "not-json")
		require.Error(t, err)

		_, err = NormalizeJSONFieldValue(reflect.TypeOf(map[string]any{}), make(chan int))
		require.Error(t, err)
	})
}

func TestNormalizeJSONReflectValue_UnwrapsPointersAndInterfaces(t *testing.T) {
	var nilMap *map[string]any
	value, err := NormalizeJSONReflectValue(reflect.TypeOf(map[string]any{}), reflect.ValueOf(nilMap))
	require.NoError(t, err)
	require.Nil(t, value)

	payload := map[string]any{"count": 3}
	var wrapped any = &payload
	value, err = NormalizeJSONReflectValue(reflect.TypeOf(map[string]any{}), reflect.ValueOf(wrapped))
	require.NoError(t, err)

	decoded, ok := value.(map[string]any)
	require.True(t, ok)
	require.Equal(t, int64(3), decoded["count"])
}

func TestUnmarshalJSONFieldValue_CoversStringCarriersAndFallbacks(t *testing.T) {
	t.Run("native documents can be re-encoded into string carriers", func(t *testing.T) {
		dest := ""
		err := UnmarshalJSONFieldValue(&types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
			"accepted": &types.AttributeValueMemberBOOL{Value: true},
			"count":    &types.AttributeValueMemberN{Value: "2"},
		}}, reflect.ValueOf(&dest).Elem(), nil)
		require.NoError(t, err)
		require.Equal(t, `{"accepted":true,"count":2}`, dest)
	})

	t.Run("pointer-backed binary carriers are allocated and filled", func(t *testing.T) {
		var dest *[]byte
		err := UnmarshalJSONFieldValue(&types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
			"accepted": &types.AttributeValueMemberBOOL{Value: true},
		}}, reflect.ValueOf(&dest).Elem(), nil)
		require.NoError(t, err)
		require.NotNil(t, dest)
		require.Equal(t, []byte(`{"accepted":true}`), *dest)
	})

	t.Run("string-backed attributes decode into structured destinations", func(t *testing.T) {
		var dest map[string]any
		err := UnmarshalJSONFieldValue(&types.AttributeValueMemberS{Value: `{"mode":"sync","count":4}`}, reflect.ValueOf(&dest).Elem(), nil)
		require.NoError(t, err)
		require.Equal(t, "sync", dest["mode"])
		require.Equal(t, int64(4), dest["count"])
	})

	t.Run("null attributes zero the destination", func(t *testing.T) {
		dest := "not-empty"
		err := UnmarshalJSONFieldValue(&types.AttributeValueMemberNULL{Value: true}, reflect.ValueOf(&dest).Elem(), nil)
		require.NoError(t, err)
		require.Empty(t, dest)
	})

	t.Run("fallback handles native structured values for typed destinations", func(t *testing.T) {
		type response struct {
			Accepted bool `json:"accepted"`
		}

		called := false
		var dest response
		err := UnmarshalJSONFieldValue(&types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
			"accepted": &types.AttributeValueMemberBOOL{Value: true},
		}}, reflect.ValueOf(&dest).Elem(), func() error {
			called = true
			dest.Accepted = true
			return nil
		})
		require.NoError(t, err)
		require.True(t, called)
		require.True(t, dest.Accepted)
	})

	t.Run("invalid json strings fall back when a handler is provided", func(t *testing.T) {
		called := false
		var dest struct{ Count int }
		err := UnmarshalJSONFieldValue(&types.AttributeValueMemberS{Value: "not-json"}, reflect.ValueOf(&dest).Elem(), func() error {
			called = true
			dest.Count = 7
			return nil
		})
		require.NoError(t, err)
		require.True(t, called)
		require.Equal(t, 7, dest.Count)
	})

	t.Run("invalid destinations and fallback errors propagate", func(t *testing.T) {
		err := UnmarshalJSONFieldValue(&types.AttributeValueMemberS{Value: "{}"}, reflect.Value{}, nil)
		require.Error(t, err)

		var dest struct{ Accepted bool }
		expected := json.InvalidUnmarshalError{Type: reflect.TypeOf(dest)}
		err = UnmarshalJSONFieldValue(&types.AttributeValueMemberM{}, reflect.ValueOf(&dest).Elem(), func() error {
			return &expected
		})
		require.ErrorIs(t, err, &expected)
	})
}

func TestAttributeValueJSONCompatibilityHelpers(t *testing.T) {
	av := &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
		"items": &types.AttributeValueMemberL{Value: []types.AttributeValue{
			&types.AttributeValueMemberS{Value: "alpha"},
			&types.AttributeValueMemberN{Value: "2"},
		}},
		"numbers": &types.AttributeValueMemberNS{Value: []string{"1", "2.5"}},
		"binaries": &types.AttributeValueMemberBS{Value: [][]byte{
			[]byte("a"),
			[]byte("b"),
		}},
		"bytes": &types.AttributeValueMemberB{Value: []byte("ok")},
		"nil":   &types.AttributeValueMemberNULL{Value: true},
	}}

	value, err := attributeValueToJSONCompatible(av)
	require.NoError(t, err)

	decoded, ok := value.(map[string]any)
	require.True(t, ok)

	items, ok := decoded["items"].([]any)
	require.True(t, ok)
	require.Equal(t, "alpha", items[0])
	require.Equal(t, int64(2), items[1])

	numbers, ok := decoded["numbers"].([]any)
	require.True(t, ok)
	require.Equal(t, int64(1), numbers[0])
	require.Equal(t, 2.5, numbers[1])

	binaries, ok := decoded["binaries"].([][]byte)
	require.True(t, ok)
	require.Equal(t, []byte("a"), binaries[0])
	require.Equal(t, []byte("ok"), decoded["bytes"])
	require.Nil(t, decoded["nil"])

	jsonBytes, err := attributeValueToJSONBytes(av)
	require.NoError(t, err)
	require.JSONEq(t, `{"binaries":["YQ==","Yg=="],"bytes":"b2s=","items":["alpha",2],"nil":null,"numbers":[1,2.5]}`, string(jsonBytes))

	_, err = attributeValueToJSONBytes(nil)
	require.Error(t, err)
}

func TestDecodeAndAssignmentHelpers(t *testing.T) {
	t.Run("dynamic targets decode generic values", func(t *testing.T) {
		var dest map[string]any
		err := decodeJSONTextIntoValue([]byte(`{"count":2}`), reflect.ValueOf(&dest).Elem())
		require.NoError(t, err)
		require.Equal(t, int64(2), dest["count"])
	})

	t.Run("typed targets decode directly", func(t *testing.T) {
		var dest struct {
			Count int `json:"count"`
		}
		err := decodeJSONTextIntoValue([]byte(`{"count":2}`), reflect.ValueOf(&dest).Elem())
		require.NoError(t, err)
		require.Equal(t, 2, dest.Count)
	})

	t.Run("typed targets reject trailing data", func(t *testing.T) {
		var dest struct {
			Count int `json:"count"`
		}
		err := decodeJSONTextIntoValue([]byte(`{"count":2} trailing`), reflect.ValueOf(&dest).Elem())
		require.Error(t, err)
	})

	t.Run("empty json text is rejected", func(t *testing.T) {
		var dest map[string]any
		err := decodeJSONTextIntoValue(nil, reflect.ValueOf(&dest).Elem())
		require.Error(t, err)
	})

	t.Run("dynamic assignment resets and rejects incompatible shapes", func(t *testing.T) {
		dest := map[string]any{"stale": true}
		err := assignDynamicJSONValue(reflect.ValueOf(&dest).Elem(), nil)
		require.NoError(t, err)
		require.Nil(t, dest)

		var list []any
		err = assignDynamicJSONValue(reflect.ValueOf(&list).Elem(), map[string]any{"bad": true})
		require.Error(t, err)
	})
}

func TestNormalizeDecodedJSONNumbers_CoversAdditionalNumericForms(t *testing.T) {
	require.Equal(t, uint64(18446744073709551615), normalizeDecodedJSONNumbers(json.Number("18446744073709551615")))
	require.Equal(t, 100.0, normalizeDecodedJSONNumbers(json.Number("1e2")))
	require.Equal(t, "not-a-number", normalizeDecodedJSONNumbers(json.Number("not-a-number")))
}

func TestJSONTypeHelpers(t *testing.T) {
	require.True(t, HasJSONTag(map[string]string{"json": ""}))
	require.False(t, HasJSONTag(nil))

	require.True(t, HasJSONModifier("pk,json,omitempty"))
	require.False(t, HasJSONModifier("pk,omitempty"))
	require.True(t, HasModifier("attr:value, omitempty", "omitempty"))
	require.False(t, HasModifier("attr:value,notomitempty", "omitempty"))
	require.False(t, HasModifier("attr:value,omitemptysuffix", "omitempty"))
	require.False(t, HasModifier("attr:value", ""))

	require.True(t, isJSONStringCarrierType(reflect.TypeOf("")))
	require.True(t, isJSONStringCarrierType(reflect.TypeOf([]byte{})))
	require.False(t, isJSONStringCarrierType(reflect.TypeOf(map[string]any{})))

	require.True(t, isDynamicJSONType(reflect.TypeOf(map[string]any{})))
	require.True(t, isDynamicJSONType(reflect.TypeOf([]any{})))
	require.True(t, isDynamicJSONType(reflect.TypeOf((*any)(nil))))
	require.False(t, isDynamicJSONType(reflect.TypeOf(struct{}{})))

	typ := derefType(reflect.TypeOf((**string)(nil)))
	require.Equal(t, reflect.TypeOf(""), typ)

	var nilSlice []string
	require.True(t, isNilLike(nilSlice))
	require.False(t, isNilLike("value"))

	require.True(t, isNullAttributeValue(nil))
	require.True(t, isNullAttributeValue(&types.AttributeValueMemberNULL{Value: true}))
	require.False(t, isNullAttributeValue(&types.AttributeValueMemberNULL{Value: false}))
	require.False(t, isNullAttributeValue(&types.AttributeValueMemberS{Value: "ok"}))
}

func TestSetJSONStringCarrierValue_CoversRemainingBranches(t *testing.T) {
	t.Run("string carrier pointers reset on null values", func(t *testing.T) {
		dest := new(string)
		*dest = "stale"
		err := setJSONStringCarrierValue(&types.AttributeValueMemberNULL{Value: true}, reflect.ValueOf(&dest).Elem())
		require.NoError(t, err)
		require.Nil(t, dest)
	})

	t.Run("binary carriers accept direct strings", func(t *testing.T) {
		dest := []byte("stale")
		err := setJSONStringCarrierValue(&types.AttributeValueMemberS{Value: `{"ok":true}`}, reflect.ValueOf(&dest).Elem())
		require.NoError(t, err)
		require.Equal(t, []byte(`{"ok":true}`), dest)
	})

	t.Run("unsupported destinations return an error", func(t *testing.T) {
		var dest int
		err := setJSONStringCarrierValue(&types.AttributeValueMemberS{Value: "1"}, reflect.ValueOf(&dest).Elem())
		require.Error(t, err)
	})
}
