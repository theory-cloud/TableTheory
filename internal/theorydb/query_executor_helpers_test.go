package theorydb

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"
)

func TestAttributeValueStringAndType(t *testing.T) {
	require.Equal(t, "text", attributeValueString(&types.AttributeValueMemberS{Value: "text"}))
	require.Equal(t, "42", attributeValueString(&types.AttributeValueMemberN{Value: "42"}))
	require.Empty(t, attributeValueString(&types.AttributeValueMemberBOOL{Value: true}))
	require.Empty(t, attributeValueString(nil))

	require.Equal(t, "S", attributeValueType(&types.AttributeValueMemberS{Value: "text"}))
	require.Equal(t, "N", attributeValueType(&types.AttributeValueMemberN{Value: "42"}))
	require.Equal(t, "B", attributeValueType(&types.AttributeValueMemberB{Value: []byte("bin")}))
	require.Equal(t, "BOOL", attributeValueType(&types.AttributeValueMemberBOOL{Value: true}))
	require.Equal(t, "NULL", attributeValueType(&types.AttributeValueMemberNULL{Value: true}))
	require.Equal(t, "L", attributeValueType(&types.AttributeValueMemberL{}))
	require.Equal(t, "M", attributeValueType(&types.AttributeValueMemberM{}))
	require.Equal(t, "SS", attributeValueType(&types.AttributeValueMemberSS{}))
	require.Equal(t, "NS", attributeValueType(&types.AttributeValueMemberNS{}))
	require.Equal(t, "BS", attributeValueType(&types.AttributeValueMemberBS{}))
	require.Equal(t, "<nil>", attributeValueType(nil))
}
