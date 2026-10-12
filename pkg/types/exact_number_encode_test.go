package types

import (
	"encoding/json"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"
)

func TestToAttributeValueEncodesExactJSONNumberAsN(t *testing.T) {
	av, err := NewConverter().ToAttributeValue(map[string]any{
		"big":   json.Number("9223372036854775809"),
		"small": json.Number("0.123456789012345678"),
		"plain": "text",
	})
	require.NoError(t, err)

	got := messageMap(t, av)
	require.Equal(t, "9223372036854775809", numberText(t, got["big"]))
	require.Equal(t, "0.123456789012345678", numberText(t, got["small"]))
	require.IsType(t, &types.AttributeValueMemberS{}, got["plain"])
}
