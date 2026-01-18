package expr

import (
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"
)

func TestAttributeValueToInterface_CoversAdditionalBranches_COV7(t *testing.T) {
	t.Run("ScalarTypes", func(t *testing.T) {
		out, err := attributeValueToInterface(&types.AttributeValueMemberB{Value: []byte("x")})
		require.NoError(t, err)
		require.Equal(t, []byte("x"), out)

		out, err = attributeValueToInterface(&types.AttributeValueMemberBOOL{Value: true})
		require.NoError(t, err)
		require.Equal(t, true, out)

		out, err = attributeValueToInterface(&types.AttributeValueMemberNULL{Value: true})
		require.NoError(t, err)
		require.Nil(t, out)

		out, err = attributeValueToInterface(&types.AttributeValueMemberSS{Value: []string{"a", "b"}})
		require.NoError(t, err)
		require.Equal(t, []string{"a", "b"}, out)

		out, err = attributeValueToInterface(&types.AttributeValueMemberBS{Value: [][]byte{[]byte("a"), []byte("b")}})
		require.NoError(t, err)
		require.Equal(t, [][]byte{[]byte("a"), []byte("b")}, out)
	})

	t.Run("UnknownTypesReturnErrors", func(t *testing.T) {
		var unknown types.AttributeValue
		_, err := attributeValueToInterface(unknown)
		require.Error(t, err)

		_, err = attributeValueListToInterface([]types.AttributeValue{
			&types.AttributeValueMemberS{Value: "ok"},
			unknown,
		})
		require.Error(t, err)

		_, err = attributeValueMapToInterface(map[string]types.AttributeValue{
			"ok":  &types.AttributeValueMemberS{Value: "v"},
			"bad": unknown,
		})
		require.Error(t, err)

		_, err = attributeValueNumberSetToInterface([]string{"not-a-number"})
		require.Error(t, err)
	})
}

func TestIsZeroValue_CoversAllKinds_COV7(t *testing.T) {
	var iface any
	var nilPtr *int
	nonNilPtr := new(int)

	tests := []struct {
		name  string
		value reflect.Value
		want  bool
	}{
		{name: "ArrayLenZero", value: reflect.ValueOf([0]int{}), want: true},
		{name: "ArrayLenNonZero", value: reflect.ValueOf([1]int{}), want: false},
		{name: "MapLenZero", value: reflect.ValueOf(map[string]int{}), want: true},
		{name: "MapLenNonZero", value: reflect.ValueOf(map[string]int{"a": 1}), want: false},
		{name: "SliceLenZero", value: reflect.ValueOf([]int{}), want: true},
		{name: "SliceLenNonZero", value: reflect.ValueOf([]int{1}), want: false},
		{name: "StringLenZero", value: reflect.ValueOf(""), want: true},
		{name: "StringLenNonZero", value: reflect.ValueOf("x"), want: false},
		{name: "BoolFalse", value: reflect.ValueOf(false), want: true},
		{name: "BoolTrue", value: reflect.ValueOf(true), want: false},
		{name: "IntZero", value: reflect.ValueOf(int64(0)), want: true},
		{name: "IntNonZero", value: reflect.ValueOf(int64(1)), want: false},
		{name: "UintZero", value: reflect.ValueOf(uint64(0)), want: true},
		{name: "UintNonZero", value: reflect.ValueOf(uint64(1)), want: false},
		{name: "FloatZero", value: reflect.ValueOf(float64(0)), want: true},
		{name: "FloatNonZero", value: reflect.ValueOf(float64(1)), want: false},
		{name: "InterfaceNil", value: reflect.ValueOf(&iface).Elem(), want: true},
		{name: "PointerNil", value: reflect.ValueOf(nilPtr), want: true},
		{name: "PointerNonNil", value: reflect.ValueOf(nonNilPtr), want: false},
		{name: "OtherKindsDefaultFalse", value: reflect.ValueOf(struct{}{}), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isZeroValue(tt.value))
		})
	}
}
