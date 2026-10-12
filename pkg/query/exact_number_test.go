package query

import (
	"encoding/json"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	pkgtypes "github.com/theory-cloud/tabletheory/v4/pkg/types"
)

//nolint:govet // Field order mirrors the production fixture under test.
type exactNumberRecord struct {
	ID      string         `json:"id"`
	Payload map[string]any `json:"payload"`
}

func requireExactNumberText(t *testing.T, av types.AttributeValue) string {
	t.Helper()
	number, ok := av.(*types.AttributeValueMemberN)
	require.Truef(t, ok, "expected number attribute value, got %T", av)
	return number.Value
}

func requireExactMessage(t *testing.T, av types.AttributeValue) map[string]types.AttributeValue {
	t.Helper()
	message, ok := av.(*types.AttributeValueMemberM)
	require.Truef(t, ok, "expected map attribute value, got %T", av)
	return message.Value
}

func TestUnmarshalItemPreservesExactNumbers(t *testing.T) {
	item := map[string]types.AttributeValue{
		"id": &types.AttributeValueMemberS{Value: "1"},
		"payload": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
			"bigN": &types.AttributeValueMemberN{Value: "9223372036854775809"},
			"ns":   &types.AttributeValueMemberNS{Value: []string{"9007199254740993", "0.123456789012345678"}},
		}},
	}

	var out exactNumberRecord
	require.NoError(t, UnmarshalItem(item, &out))

	require.Equal(t, json.Number("9223372036854775809"), out.Payload["bigN"])
	require.Equal(t, []any{int64(9007199254740993), json.Number("0.123456789012345678")}, out.Payload["ns"])

	// Rewrite proof: re-encoding the decoded payload preserves the exact N text
	// instead of substituting the float64-rounded value.
	av, err := pkgtypes.NewConverter().ToAttributeValue(out.Payload)
	require.NoError(t, err)
	require.Equal(t, "9223372036854775809", requireExactNumberText(t, requireExactMessage(t, av)["bigN"]))
}
