package transaction

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMergeRawConditionsFailsClosedOnARepeatedValuePlaceholder is the Go control
// for the chained-collision class: two conditions that reuse one value
// placeholder are refused rather than collapsed onto a single value. A
// composition that renamed the second placement would silently retarget the
// first condition's predicate, which is what the TypeScript fix prevents.
func TestMergeRawConditionsFailsClosedOnARepeatedValuePlaceholder(t *testing.T) {
	b := &Builder{}
	first := rawCondition{expression: "#a = :v", values: map[string]types.AttributeValue{":v": &types.AttributeValueMemberS{Value: "first"}}}
	second := rawCondition{expression: "#b = :v", values: map[string]types.AttributeValue{":v": &types.AttributeValueMemberS{Value: "second"}}}

	_, _, _, err := b.mergeRawConditions("", nil, nil, []rawCondition{first, second})
	require.ErrorContains(t, err, "duplicate condition value placeholder")
}

// TestMergeRawConditionsKeepsDistinctPredicates is the positive control: with no
// collision, every placeholder and value survives into the merged condition.
func TestMergeRawConditionsKeepsDistinctPredicates(t *testing.T) {
	b := &Builder{}
	first := rawCondition{expression: "#a = :first", values: map[string]types.AttributeValue{":first": &types.AttributeValueMemberS{Value: "one"}}}
	second := rawCondition{expression: "#b = :second", values: map[string]types.AttributeValue{":second": &types.AttributeValueMemberS{Value: "two"}}}

	expr, _, values, err := b.mergeRawConditions("", nil, nil, []rawCondition{first, second})
	require.NoError(t, err)
	assert.Equal(t, "(#a = :first) AND (#b = :second)", expr)
	assert.Len(t, values, 2)
}
