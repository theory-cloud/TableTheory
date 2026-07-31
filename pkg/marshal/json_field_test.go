package marshal

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v3/pkg/model"
	"github.com/theory-cloud/tabletheory/v3/pkg/naming"
)

type jsonMarshalRecord struct {
	Payload  map[string]any `json:"payload"`
	Response string         `json:"response,omitempty"`
}

func withTags(tags map[string]string) func(*model.FieldMetadata) {
	return func(fm *model.FieldMetadata) {
		fm.Tags = make(map[string]string, len(tags))
		for key, value := range tags {
			fm.Tags[key] = value
		}
	}
}

func TestMarshalers_JSONTaggedFieldsUseCompatibleShapes(t *testing.T) {
	metadata := createMetadata(
		createFieldMetadata(reflect.TypeOf(jsonMarshalRecord{}), "Payload", "payload", reflect.TypeOf(map[string]any{}), withTags(map[string]string{"json": ""})),
		createFieldMetadata(reflect.TypeOf(jsonMarshalRecord{}), "Response", "response", reflect.TypeOf(""), withTags(map[string]string{"json": ""}), withOmitEmpty()),
	)

	record := jsonMarshalRecord{
		Payload: map[string]any{
			"count": 2,
			"mode":  "sync",
		},
		Response: `{"accepted":true}`,
	}

	t.Run("unsafe marshaler keeps structured values native", func(t *testing.T) {
		item, err := New(nil).MarshalItem(record, metadata)
		require.NoError(t, err)

		payloadAV := requireAVM(t, item["payload"])
		require.Equal(t, "sync", requireAVS(t, payloadAV.Value["mode"]).Value)
		require.Equal(t, "2", requireAVN(t, payloadAV.Value["count"]).Value)
		require.Equal(t, `{"accepted":true}`, requireAVS(t, item["response"]).Value)
	})

	t.Run("unsafe marshaler respects omitempty for json fields", func(t *testing.T) {
		item, err := New(nil).MarshalItem(jsonMarshalRecord{
			Payload: map[string]any{"enabled": true},
		}, metadata)
		require.NoError(t, err)

		_, ok := item["response"]
		require.False(t, ok)
	})

	t.Run("safe marshaler matches json field behavior and omitempty", func(t *testing.T) {
		item, err := NewSafeMarshaler().MarshalItem(jsonMarshalRecord{
			Payload: map[string]any{"enabled": true},
		}, metadata)
		require.NoError(t, err)

		payloadAV := requireAVM(t, item["payload"])
		require.Equal(t, true, requireAVBOOL(t, payloadAV.Value["enabled"]).Value)
		_, ok := item["response"]
		require.False(t, ok)
	})

	t.Run("invalid json field values return marshal errors", func(t *testing.T) {
		_, err := New(nil).MarshalItem(jsonMarshalRecord{
			Payload: map[string]any{"bad": make(chan int)},
		}, metadata)
		require.Error(t, err)
	})
}

func TestResolveNestedFieldName_UsesJSONTagFallback(t *testing.T) {
	type nested struct {
		JSONOnly   string `json:"payload,omitempty"`
		Ignored    string `json:"-"`
		TheoryOnly string `theorydb:"attr:theory_payload"`
	}

	nestedType := reflect.TypeOf(nested{})

	jsonField := nestedType.Field(0)
	attrName, skip := resolveNestedFieldName(jsonField, naming.CamelCase)
	require.False(t, skip)
	require.Equal(t, "payload", attrName)
	require.Equal(t, "payload", safeJSONTagName(jsonField.Tag.Get("json")))
	require.Equal(t, "payload", safeJSONTagName("payload"))

	ignoredField := nestedType.Field(1)
	attrName, skip = resolveNestedFieldName(ignoredField, naming.CamelCase)
	require.True(t, skip)
	require.Empty(t, attrName)

	theoryField := nestedType.Field(2)
	attrName, skip = resolveNestedFieldName(theoryField, naming.CamelCase)
	require.False(t, skip)
	require.Equal(t, "theory_payload", attrName)
}
