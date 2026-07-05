package query

import (
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"
)

func TestAttributeValueToInterface_CoversBinaryAndSetCases_COV6(t *testing.T) {
	out, err := attributeValueToInterface(&types.AttributeValueMemberSS{Value: []string{"a", "b"}})
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b"}, out)

	out, err = attributeValueToInterface(&types.AttributeValueMemberBS{Value: [][]byte{[]byte("x")}})
	require.NoError(t, err)
	require.Equal(t, [][]byte{[]byte("x")}, out)

	out, err = attributeValueToInterface(&types.AttributeValueMemberB{Value: []byte("raw")})
	require.NoError(t, err)
	require.Equal(t, []byte("raw"), out)
}

func TestUnmarshalAttributeValue_NullClearsScalarValue_COV6(t *testing.T) {
	v := 42
	dest := reflect.ValueOf(&v).Elem()

	err := unmarshalAttributeValue(&types.AttributeValueMemberNULL{Value: true}, dest)
	require.NoError(t, err)
	require.Zero(t, v)
}

func TestUnmarshalAnyAttributeValue_PropagatesUnsupportedType_COV6(t *testing.T) {
	type unsupportedAV struct{ types.AttributeValue }

	var out any
	dest := reflect.ValueOf(&out).Elem()

	err := unmarshalAttributeValue(&unsupportedAV{}, dest)
	require.Error(t, err)
}
