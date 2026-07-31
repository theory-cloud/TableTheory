package query

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v3/pkg/core"
)

type fixedArrayFallbackItem struct {
	PK     string    `theorydb:"pk" json:"PK"`
	Values [2]string `json:"values"`
}

type fixedArrayFallbackExecutor struct{}

func (fixedArrayFallbackExecutor) ExecuteQuery(*core.CompiledQuery, any) error { return nil }
func (fixedArrayFallbackExecutor) ExecuteScan(*core.CompiledQuery, any) error  { return nil }

func (fixedArrayFallbackExecutor) ExecuteBatchGet(input *CompiledBatchGet, _ *core.BatchGetOptions) ([]map[string]types.AttributeValue, error) {
	return []map[string]types.AttributeValue{
		{
			"PK": input.Keys[0]["PK"],
			"values": &types.AttributeValueMemberL{Value: []types.AttributeValue{
				&types.AttributeValueMemberS{Value: "first"},
				&types.AttributeValueMemberS{Value: "second"},
			}},
		},
	}, nil
}

func (fixedArrayFallbackExecutor) ExecuteBatchWrite(*CompiledBatchWrite) error { return nil }

func (fixedArrayFallbackExecutor) ExecuteUpdateItem(*core.CompiledQuery, map[string]types.AttributeValue) error {
	return nil
}

func (fixedArrayFallbackExecutor) ExecuteUpdateItemWithResult(*core.CompiledQuery, map[string]types.AttributeValue) (*core.UpdateResult, error) {
	return &core.UpdateResult{Attributes: map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: "p1"},
		"values": &types.AttributeValueMemberL{Value: []types.AttributeValue{
			&types.AttributeValueMemberS{Value: "first"},
			&types.AttributeValueMemberS{Value: "second"},
		}},
	}}, nil
}

func TestBatchGet_NoMetadataFallbackUnmarshalsFixedArray(t *testing.T) {
	q := New(&struct{}{}, cov5Metadata{
		table:      "tbl",
		primaryKey: core.KeySchema{PartitionKey: "PK"},
	}, fixedArrayFallbackExecutor{})

	var got []fixedArrayFallbackItem
	require.NoError(t, q.BatchGet([]any{core.NewKeyPair("p1")}, &got))
	require.Equal(t, []fixedArrayFallbackItem{{
		PK:     "p1",
		Values: [2]string{"first", "second"},
	}}, got)
}

func TestUpdateBuilder_ALLNEWNoMetadataFallbackUnmarshalsFixedArray(t *testing.T) {
	q := New(&struct{}{}, cov5Metadata{
		table:      "tbl",
		primaryKey: core.KeySchema{PartitionKey: "PK"},
	}, fixedArrayFallbackExecutor{})
	q.Where("PK", "=", "p1")

	var got fixedArrayFallbackItem
	require.NoError(t, NewUpdateBuilder(q).Set("status", "ok").ExecuteWithResult(&got))
	require.Equal(t, fixedArrayFallbackItem{
		PK:     "p1",
		Values: [2]string{"first", "second"},
	}, got)
}
