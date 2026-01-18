package query

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"
)

func TestCursor_JSONToAttributeValue_ErrorBranches_COV6(t *testing.T) {
	_, err := jsonToAttributeValue("not-a-map")
	require.Error(t, err)

	_, err = jsonToAttributeValue(map[string]any{"S": "a", "N": "1"})
	require.Error(t, err)

	_, err = jsonToAttributeValue(map[string]any{"unknown": "x"})
	require.Error(t, err)
}

func TestCursor_JSONHelpers_TypeValidation_COV6(t *testing.T) {
	_, err := jsonNumberToAttributeValue(1)
	require.Error(t, err)

	_, err = jsonBoolToAttributeValue("true")
	require.Error(t, err)

	_, err = jsonListToAttributeValue("not-a-slice")
	require.Error(t, err)

	_, err = jsonMapToAttributeValue("not-a-map")
	require.Error(t, err)

	_, err = jsonStringSetToAttributeValue("not-a-slice")
	require.Error(t, err)
	_, err = jsonStringSetToAttributeValue([]any{1})
	require.Error(t, err)

	_, err = jsonNumberSetToAttributeValue("not-a-slice")
	require.Error(t, err)
	_, err = jsonNumberSetToAttributeValue([]any{1})
	require.Error(t, err)

	_, err = jsonBinarySetToAttributeValue("not-a-slice")
	require.Error(t, err)
	_, err = jsonBinarySetToAttributeValue([]any{1})
	require.Error(t, err)
	_, err = jsonBinarySetToAttributeValue([]any{"not-base64"})
	require.Error(t, err)
}

func TestCursor_AttributeValueListAndMapToJSON_ErrorPropagation_COV6(t *testing.T) {
	type unsupportedAV struct{ types.AttributeValue }

	_, err := attributeValueListToJSON([]types.AttributeValue{&unsupportedAV{}})
	require.Error(t, err)

	_, err = attributeValueMapToJSON(map[string]types.AttributeValue{"a": &unsupportedAV{}})
	require.Error(t, err)

	_, err = EncodeCursor(map[string]types.AttributeValue{"a": &unsupportedAV{}}, "", "")
	require.Error(t, err)
}
