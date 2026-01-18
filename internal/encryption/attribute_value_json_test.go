package encryption

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"
)

func TestAVJSON_RoundTrip_Collections(t *testing.T) {
	cases := []struct {
		av   types.AttributeValue
		name string
	}{
		{
			name: "list",
			av: &types.AttributeValueMemberL{Value: []types.AttributeValue{
				&types.AttributeValueMemberS{Value: "a"},
				&types.AttributeValueMemberN{Value: "1"},
				&types.AttributeValueMemberBOOL{Value: true},
				&types.AttributeValueMemberNULL{Value: true},
				&types.AttributeValueMemberB{Value: []byte{0x01, 0x02}},
			}},
		},
		{
			name: "map",
			av: &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
				"k1":     &types.AttributeValueMemberS{Value: "v1"},
				"nested": &types.AttributeValueMemberL{Value: []types.AttributeValue{&types.AttributeValueMemberN{Value: "2"}}},
			}},
		},
		{
			name: "string_set",
			av:   &types.AttributeValueMemberSS{Value: []string{"a", "b"}},
		},
		{
			name: "number_set",
			av:   &types.AttributeValueMemberNS{Value: []string{"1", "2"}},
		},
		{
			name: "binary_set",
			av:   &types.AttributeValueMemberBS{Value: [][]byte{[]byte("a"), []byte("b")}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := marshalAVJSON(tc.av)
			require.NoError(t, err)

			decoded, err := unmarshalAVJSON(encoded)
			require.NoError(t, err)

			require.Equal(t, tc.av, decoded)
		})
	}
}

func TestAVJSON_UnmarshalScalars_DefaultsAndErrors(t *testing.T) {
	t.Run("scalar_defaults", func(t *testing.T) {
		got, err := unmarshalAVJSON(avJSON{Type: "S"})
		require.NoError(t, err)
		require.Equal(t, &types.AttributeValueMemberS{Value: ""}, got)

		got, err = unmarshalAVJSON(avJSON{Type: "N"})
		require.NoError(t, err)
		require.Equal(t, &types.AttributeValueMemberN{Value: "0"}, got)

		got, err = unmarshalAVJSON(avJSON{Type: "B"})
		require.NoError(t, err)
		require.Equal(t, &types.AttributeValueMemberB{Value: nil}, got)

		got, err = unmarshalAVJSON(avJSON{Type: "BOOL"})
		require.NoError(t, err)
		require.Equal(t, &types.AttributeValueMemberBOOL{Value: false}, got)

		got, err = unmarshalAVJSON(avJSON{Type: "NULL"})
		require.NoError(t, err)
		require.Equal(t, &types.AttributeValueMemberNULL{Value: true}, got)
	})

	t.Run("binary_decode_errors", func(t *testing.T) {
		invalid := "!!!"
		_, err := unmarshalAVJSON(avJSON{Type: "B", B: &invalid})
		require.Error(t, err)

		_, err = unmarshalAVJSON(avJSON{Type: "BS", BS: []string{invalid}})
		require.Error(t, err)
	})

	t.Run("unsupported_encoded_type_errors", func(t *testing.T) {
		_, err := unmarshalAVJSON(avJSON{Type: "UNKNOWN"})
		require.Error(t, err)
	})
}

func TestAVJSON_MarshalUnsupportedTypeErrors(t *testing.T) {
	_, err := marshalAVJSON(nil)
	require.Error(t, err)
}

func TestAVJSON_CollectionMarshalUnmarshal_PropagatesErrors(t *testing.T) {
	t.Run("marshal_list_with_unsupported_element", func(t *testing.T) {
		_, err := marshalAVJSON(&types.AttributeValueMemberL{Value: []types.AttributeValue{nil}})
		require.Error(t, err)
	})

	t.Run("marshal_map_with_unsupported_element", func(t *testing.T) {
		_, err := marshalAVJSON(&types.AttributeValueMemberM{Value: map[string]types.AttributeValue{"bad": nil}})
		require.Error(t, err)
	})

	t.Run("unmarshal_list_with_invalid_nested_element", func(t *testing.T) {
		invalid := "!!!"
		_, err := unmarshalAVJSON(avJSON{Type: "L", L: []avJSON{{Type: "B", B: &invalid}}})
		require.Error(t, err)
	})

	t.Run("unmarshal_map_with_invalid_nested_element", func(t *testing.T) {
		invalid := "!!!"
		_, err := unmarshalAVJSON(avJSON{Type: "M", M: map[string]avJSON{"bad": {Type: "B", B: &invalid}}})
		require.Error(t, err)
	})
}
