package query

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"
)

func TestCursor_EncodeDecode_RoundTripAllTypes(t *testing.T) {
	lastKey := map[string]types.AttributeValue{
		"s":    &types.AttributeValueMemberS{Value: "x"},
		"n":    &types.AttributeValueMemberN{Value: "42"},
		"b":    &types.AttributeValueMemberB{Value: []byte("bin")},
		"bool": &types.AttributeValueMemberBOOL{Value: true},
		"null": &types.AttributeValueMemberNULL{Value: true},
		"ss":   &types.AttributeValueMemberSS{Value: []string{"a", "b"}},
		"ns":   &types.AttributeValueMemberNS{Value: []string{"1", "2.5"}},
		"bs":   &types.AttributeValueMemberBS{Value: [][]byte{[]byte("a"), []byte("b")}},
		"l": &types.AttributeValueMemberL{Value: []types.AttributeValue{
			&types.AttributeValueMemberS{Value: "x"},
			&types.AttributeValueMemberN{Value: "1"},
			&types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
				"k": &types.AttributeValueMemberS{Value: "v"},
			}},
		}},
		"m": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
			"nested": &types.AttributeValueMemberBOOL{Value: false},
		}},
	}

	encoded, err := EncodeCursor(lastKey, "gsi1", "desc")
	require.NoError(t, err)
	require.NotEmpty(t, encoded)

	cursor, err := DecodeCursor(encoded)
	require.NoError(t, err)
	require.NotNil(t, cursor)
	require.Equal(t, "gsi1", cursor.IndexName)
	require.Equal(t, "desc", cursor.SortDirection)

	roundTripped, err := cursor.ToAttributeValues()
	require.NoError(t, err)
	require.Equal(t, lastKey, roundTripped)
}

func TestCursor_EncodeCursor_EmptyKeyReturnsEmptyString(t *testing.T) {
	encoded, err := EncodeCursor(nil, "", "")
	require.NoError(t, err)
	require.Empty(t, encoded)
}

func TestCursor_DecodeCursor_InvalidBase64ReturnsError(t *testing.T) {
	_, err := DecodeCursor("not-base64!!")
	require.Error(t, err)
}

func TestCursor_ToAttributeValues_NilAndEmpty(t *testing.T) {
	var cursor *Cursor
	values, err := cursor.ToAttributeValues()
	require.NoError(t, err)
	require.Nil(t, values)

	empty := &Cursor{LastEvaluatedKey: map[string]any{}}
	values, err = empty.ToAttributeValues()
	require.NoError(t, err)
	require.Nil(t, values)
}

func TestCursor_jsonToAttributeValue_ErrorCases(t *testing.T) {
	_, err := jsonToAttributeValue("not-a-map")
	require.Error(t, err)

	_, err = jsonToAttributeValue(map[string]any{"S": "x", "N": "1"})
	require.Error(t, err)

	_, err = jsonToAttributeValue(map[string]any{"UNKNOWN": "x"})
	require.Error(t, err)

	_, err = jsonToAttributeValue(map[string]any{"B": "!!!"})
	require.Error(t, err)

	_, err = jsonToAttributeValue(map[string]any{"L": "not-a-list"})
	require.Error(t, err)

	_, err = jsonToAttributeValue(map[string]any{"SS": "not-a-list"})
	require.Error(t, err)

	_, err = jsonToAttributeValue(map[string]any{"NS": []any{1}})
	require.Error(t, err)

	_, err = jsonToAttributeValue(map[string]any{"BS": []any{"!!!"}})
	require.Error(t, err)
}
