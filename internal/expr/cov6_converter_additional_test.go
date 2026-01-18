package expr

import (
	"reflect"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"
)

func TestFieldNameFromJSONTag_DefaultsWhenEmpty_COV6(t *testing.T) {
	require.Equal(t, "default", fieldNameFromJSONTag("default", ",omitempty"))
	require.Equal(t, "explicit", fieldNameFromJSONTag("default", "explicit,omitempty"))
}

func TestIsZeroValue_CoversPointersInterfacesAndStructs_COV6(t *testing.T) {
	var ptr *int
	require.True(t, isZeroValue(reflect.ValueOf(ptr)))

	val := 1
	ptr = &val
	require.False(t, isZeroValue(reflect.ValueOf(ptr)))

	var iface any
	require.True(t, isZeroValue(reflect.ValueOf(&iface).Elem()))

	iface = "x"
	require.False(t, isZeroValue(reflect.ValueOf(&iface).Elem()))

	require.False(t, isZeroValue(reflect.ValueOf(time.Time{})))
}

func TestAttributeValueToInterface_UnknownTypesAndErrorPropagation_COV6(t *testing.T) {
	type unsupportedAV struct{ types.AttributeValue }

	_, err := attributeValueToInterface(&unsupportedAV{})
	require.Error(t, err)

	_, err = attributeValueListToInterface([]types.AttributeValue{&unsupportedAV{}})
	require.Error(t, err)

	_, err = attributeValueMapToInterface(map[string]types.AttributeValue{"a": &unsupportedAV{}})
	require.Error(t, err)
}

func TestUnmarshalHelpers_ErrorBranches_COV6(t *testing.T) {
	var outString string
	require.Error(t, unmarshalBinary([]byte("x"), reflect.ValueOf(&outString).Elem()))
	require.Error(t, unmarshalBool(true, reflect.ValueOf(&outString).Elem()))
	require.Error(t, unmarshalList([]types.AttributeValue{}, reflect.ValueOf(&outString).Elem()))
	require.Error(t, unmarshalMap(map[string]types.AttributeValue{}, reflect.ValueOf(&outString).Elem()))

	var intKeyed map[int]string
	require.Error(t, unmarshalMapIntoMap(map[string]types.AttributeValue{}, reflect.ValueOf(&intKeyed).Elem()))

	require.Error(t, unmarshalStringSet([]string{"a"}, reflect.ValueOf(&outString).Elem()))
	require.Error(t, unmarshalNumberSet([]string{"1"}, reflect.ValueOf(&outString).Elem()))
	require.Error(t, unmarshalBinarySet([][]byte{[]byte("x")}, reflect.ValueOf(&outString).Elem()))
}

func TestUnmarshalNumberSet_WrapsParseErrors_COV6(t *testing.T) {
	var out []int
	err := unmarshalNumberSet([]string{"not-a-number"}, reflect.ValueOf(&out).Elem())
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to unmarshal number set item")
}
