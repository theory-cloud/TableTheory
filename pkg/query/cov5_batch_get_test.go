package query

import (
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/core"
)

type cov5BatchExecutor struct {
	calls     int
	failFirst bool
}

func (e *cov5BatchExecutor) ExecuteQuery(_ *core.CompiledQuery, _ any) error { return nil }
func (e *cov5BatchExecutor) ExecuteScan(_ *core.CompiledQuery, _ any) error  { return nil }
func (e *cov5BatchExecutor) ExecuteBatchWrite(_ *CompiledBatchWrite) error   { return nil }

func (e *cov5BatchExecutor) ExecuteBatchGet(input *CompiledBatchGet, _ *core.BatchGetOptions) ([]map[string]types.AttributeValue, error) {
	e.calls++
	if e.failFirst && e.calls == 1 {
		return nil, errors.New("boom")
	}

	items := make([]map[string]types.AttributeValue, 0, len(input.Keys))
	for i := len(input.Keys) - 1; i >= 0; i-- {
		key := input.Keys[i]
		item := make(map[string]types.AttributeValue, len(key))
		for k, v := range key {
			item[k] = v
		}
		items = append(items, item)
	}

	return items, nil
}

func TestBatchGetBuilder_SelectAndOrdering(t *testing.T) {
	exec := &cov5BatchExecutor{}
	q := New(&struct{}{}, cov5Metadata{
		table:      "tbl",
		primaryKey: core.KeySchema{PartitionKey: "pk"},
	}, exec)

	keys := []any{
		core.NewKeyPair("p1"),
		core.NewKeyPair("p2"),
	}

	var out []map[string]types.AttributeValue
	err := q.BatchGetBuilder().
		Keys(keys).
		ChunkSize(2).
		Parallel(2).
		Select("pk").
		Execute(&out)
	require.NoError(t, err)
	require.Len(t, out, 2)

	pk0, ok := out[0]["pk"].(*types.AttributeValueMemberS)
	require.True(t, ok)
	pk1, ok := out[1]["pk"].(*types.AttributeValueMemberS)
	require.True(t, ok)
	require.Equal(t, "p1", pk0.Value)
	require.Equal(t, "p2", pk1.Value)
}

func TestBatchGetBuilder_OnErrorCanSwallowChunkErrors(t *testing.T) {
	exec := &cov5BatchExecutor{failFirst: true}
	q := New(&struct{}{}, cov5Metadata{
		table:      "tbl",
		primaryKey: core.KeySchema{PartitionKey: "pk"},
	}, exec)

	keys := []any{
		core.NewKeyPair("p1"),
		core.NewKeyPair("p2"),
	}

	var out []map[string]types.AttributeValue
	err := q.BatchGetBuilder().
		Keys(keys).
		ChunkSize(1).
		OnError(func(_ []any, _ error) error { return nil }).
		Execute(&out)
	require.NoError(t, err)
	require.Len(t, out, 1)

	pk, ok := out[0]["pk"].(*types.AttributeValueMemberS)
	require.True(t, ok)
	require.Equal(t, "p2", pk.Value)
}
