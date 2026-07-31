package query

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v3/pkg/core"
)

type visibleFieldHelperConverter struct{}

func (visibleFieldHelperConverter) HasCustomConverter(reflect.Type) bool { return false }

func (visibleFieldHelperConverter) ToAttributeValue(value any) (types.AttributeValue, error) {
	return &types.AttributeValueMemberS{Value: fmt.Sprintf("converted:%v", value)}, nil
}

func (visibleFieldHelperConverter) FromAttributeValue(types.AttributeValue, any) error { return nil }

func (visibleFieldHelperConverter) ConvertToSet(any, bool) (types.AttributeValue, error) {
	return nil, nil
}

func TestFindVisibleFieldByNames_UsesPromotedMetadataAndTags(t *testing.T) {
	type BaseObject struct {
		Status string `dynamodb:"status_db" theorydb:"attr:status_attr"`
		Secret string `theorydb:"encrypted,attr:secret"`
	}

	//nolint:govet // Field order mirrors the anonymous-embed contract fixture under test.
	type activity struct {
		BaseObject
		Actor string
	}

	q := New(&activity{}, &cov4Metadata{
		table: "tbl",
		pk:    core.KeySchema{PartitionKey: "pk"},
		attrs: map[string]string{"Status": "status_meta"},
	}, &cov4Executor{})

	modelValue := reflect.ValueOf(activity{
		BaseObject: BaseObject{Status: "ready", Secret: "plaintext"},
		Actor:      "acct:actor",
	})

	cases := []struct {
		lookup string
	}{
		{lookup: "Status"},
		{lookup: "status_meta"},
		{lookup: "status_db"},
		{lookup: "attr:status_attr"},
		{lookup: "status_attr"},
	}

	for _, tc := range cases {
		t.Run(tc.lookup, func(t *testing.T) {
			fieldValue, fieldStruct, ok := q.findVisibleFieldByNames(modelValue, tc.lookup)
			require.True(t, ok)
			require.Equal(t, "Status", fieldStruct.Name)
			require.Equal(t, "ready", fieldValue.String())
		})
	}

	for _, lookup := range []string{"attr:secret", "secret"} {
		t.Run(lookup, func(t *testing.T) {
			fieldValue, fieldStruct, ok := q.findVisibleFieldByNames(modelValue, lookup)
			require.True(t, ok)
			require.Equal(t, "Secret", fieldStruct.Name)
			require.Equal(t, "plaintext", fieldValue.String())
		})
	}

	_, _, ok := q.findVisibleFieldByNames(modelValue, "encrypted")
	require.False(t, ok)

	_, _, ok = q.findVisibleFieldByNames(reflect.ValueOf("not-a-struct"), "Status")
	require.False(t, ok)
}

func TestMarshalTaggedFieldAttributeValue_UsesJSONAndConverterPaths(t *testing.T) {
	type model struct {
		Payload map[string]any `theorydb:"json"`
		Status  string         `theorydb:"attr:status"`
	}

	q := &Query{converter: visibleFieldHelperConverter{}}
	modelType := reflect.TypeOf(model{})
	modelValue := reflect.ValueOf(model{
		Payload: map[string]any{"ok": true},
		Status:  "ready",
	})

	payloadField, ok := modelType.FieldByName("Payload")
	require.True(t, ok)
	payloadValue := modelValue.FieldByName("Payload")

	payloadAV, err := q.marshalTaggedFieldAttributeValue(payloadField, payloadValue)
	require.NoError(t, err)
	payloadMap, ok := payloadAV.(*types.AttributeValueMemberM)
	require.True(t, ok)
	require.Contains(t, payloadMap.Value, "ok")

	statusField, ok := modelType.FieldByName("Status")
	require.True(t, ok)
	statusValue := modelValue.FieldByName("Status")

	statusAV, err := q.marshalTaggedFieldAttributeValue(statusField, statusValue)
	require.NoError(t, err)
	statusString, ok := statusAV.(*types.AttributeValueMemberS)
	require.True(t, ok)
	require.Equal(t, "converted:ready", statusString.Value)
}

func TestVisibleFieldHelpers_CoverDefaultAndMissPaths(t *testing.T) {
	type model struct {
		Payload any    `theorydb:"json"`
		Status  string `theorydb:"attr:status"`
	}

	modelType := reflect.TypeOf(model{})
	modelValue := reflect.ValueOf(model{
		Payload: make(chan int),
		Status:  "ready",
	})

	statusField, ok := modelType.FieldByName("Status")
	require.True(t, ok)
	statusValue := modelValue.FieldByName("Status")

	statusAV, err := (*Query)(nil).marshalTaggedFieldAttributeValue(statusField, statusValue)
	require.NoError(t, err)
	statusString, ok := statusAV.(*types.AttributeValueMemberS)
	require.True(t, ok)
	require.Equal(t, "ready", statusString.Value)

	payloadField, ok := modelType.FieldByName("Payload")
	require.True(t, ok)
	payloadValue := modelValue.FieldByName("Payload")

	_, err = (&Query{}).marshalTaggedFieldAttributeValue(payloadField, payloadValue)
	require.Error(t, err)

	fieldValue, fieldStruct, ok := (&Query{}).findVisibleFieldByNames(modelValue, "Missing")
	require.False(t, ok)
	require.False(t, fieldValue.IsValid())
	require.Equal(t, reflect.StructField{}, fieldStruct)

	require.False(t, queryFieldMatchesNames(&Query{}, reflect.StructField{}))
	require.Empty(t, appendQueryTagLookupNames(nil, "-"))
	require.Empty(t, appendQueryTheorydbLookupNames(nil, "-"))
	require.Equal(t, []string{"Status"}, appendQueryLookupName([]string{"Status"}, ""))
	require.Equal(t, []string{"Status"}, appendQueryLookupName([]string{"Status"}, "Status"))
}
