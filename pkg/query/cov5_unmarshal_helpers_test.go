package query

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"
)

func TestUnmarshalItem_UnmarshalBoolAttribute_COV5(t *testing.T) {
	type item struct {
		Flag bool `dynamodb:"flag"`
	}

	var out item
	require.NoError(t, UnmarshalItem(map[string]types.AttributeValue{
		"flag": &types.AttributeValueMemberBOOL{Value: true},
	}, &out))
	require.True(t, out.Flag)
}

func TestAttributeValueToInterface_NumberSet_COV5(t *testing.T) {
	val, err := attributeValueToInterface(&types.AttributeValueMemberNS{Value: []string{"1.5", "2"}})
	require.NoError(t, err)
	require.Equal(t, []float64{1.5, 2}, val)

	_, err = attributeValueToInterface(&types.AttributeValueMemberNS{Value: []string{"not-a-number"}})
	require.Error(t, err)
}
