package query

import (
	"testing"

	ddbTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"
)

func TestUnmarshalItems_CoversPointerSetsAndMaps(t *testing.T) {
	type nested struct {
		Field string
	}
	type target struct {
		Any       any      `dynamodb:"any"`
		Binary    []byte   `dynamodb:"bin"`
		BinarySet [][]byte `dynamodb:"bset"`
		ID        string   `dynamodb:"id"`
		Nested    nested   `dynamodb:"nested"`
		Numbers   []int    `dynamodb:"nums"`
		Ptr       *string  `dynamodb:"ptr"`
		Tags      []string `dynamodb:"tags"`
	}

	items := []map[string]ddbTypes.AttributeValue{
		{
			"id":   &ddbTypes.AttributeValueMemberS{Value: "1"},
			"ptr":  &ddbTypes.AttributeValueMemberS{Value: "x"},
			"tags": &ddbTypes.AttributeValueMemberSS{Value: []string{"a", "b"}},
			"nums": &ddbTypes.AttributeValueMemberNS{Value: []string{"1", "2"}},
			"bin":  &ddbTypes.AttributeValueMemberB{Value: []byte("data")},
			"bset": &ddbTypes.AttributeValueMemberBS{Value: [][]byte{[]byte("a")}},
			"nested": &ddbTypes.AttributeValueMemberM{Value: map[string]ddbTypes.AttributeValue{
				"Field": &ddbTypes.AttributeValueMemberS{Value: "v"},
			}},
			"any": &ddbTypes.AttributeValueMemberL{Value: []ddbTypes.AttributeValue{
				&ddbTypes.AttributeValueMemberN{Value: "1"},
				&ddbTypes.AttributeValueMemberM{Value: map[string]ddbTypes.AttributeValue{
					"k": &ddbTypes.AttributeValueMemberS{Value: "v"},
				}},
			}},
		},
	}

	var out []target
	require.NoError(t, UnmarshalItems(items, &out))
	require.Len(t, out, 1)
	require.NotNil(t, out[0].Ptr)
	require.Equal(t, "x", *out[0].Ptr)
	require.ElementsMatch(t, []string{"a", "b"}, out[0].Tags)
	require.Equal(t, []int{1, 2}, out[0].Numbers)
	require.Equal(t, "v", out[0].Nested.Field)
	require.NotNil(t, out[0].Any)

	var single target
	require.NoError(t, UnmarshalItems(items, &single))
	require.Equal(t, "1", single.ID)

	var empty target
	require.Error(t, UnmarshalItems(nil, &empty))
}
