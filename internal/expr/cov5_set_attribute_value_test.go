package expr

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"
)

func TestBuilder_convertToSetAttributeValue_CoversSetsAndErrors_COV5(t *testing.T) {
	b := NewBuilder()

	av, err := b.convertToSetAttributeValue([]string{"a", "b"})
	require.NoError(t, err)
	ss, ok := av.(*types.AttributeValueMemberSS)
	require.True(t, ok)
	require.Equal(t, []string{"a", "b"}, ss.Value)

	av, err = b.convertToSetAttributeValue([]int{1, 2})
	require.NoError(t, err)
	ns, ok := av.(*types.AttributeValueMemberNS)
	require.True(t, ok)
	require.Equal(t, []string{"1", "2"}, ns.Value)

	_, err = b.convertToSetAttributeValue("nope")
	require.Error(t, err)
}
