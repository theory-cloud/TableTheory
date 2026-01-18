package query

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/core"
)

type cov5BatchWriteExecutor struct {
	batches [][]types.WriteRequest
	calls   int
}

func (e *cov5BatchWriteExecutor) ExecuteQuery(_ *core.CompiledQuery, _ any) error { return nil }
func (e *cov5BatchWriteExecutor) ExecuteScan(_ *core.CompiledQuery, _ any) error  { return nil }

func (e *cov5BatchWriteExecutor) ExecuteBatchWriteItem(_ string, writeRequests []types.WriteRequest) (*core.BatchWriteResult, error) {
	e.calls++
	e.batches = append(e.batches, append([]types.WriteRequest(nil), writeRequests...))
	return &core.BatchWriteResult{}, nil
}

func TestQuery_BatchWriteWithOptions_ExecutesPutAndDeleteRequests(t *testing.T) {
	exec := &cov5BatchWriteExecutor{}
	q := New(&struct{}{}, cov5Metadata{
		table:      "tbl",
		primaryKey: core.KeySchema{PartitionKey: "pk"},
	}, exec)

	type item struct {
		PK string `dynamodb:"pk"`
	}

	opts := DefaultBatchOptions()
	opts.MaxBatchSize = 25

	putItems := []any{&item{PK: "p1"}}
	deleteKeys := []any{&item{PK: "d1"}}

	require.NoError(t, q.BatchWriteWithOptions(putItems, deleteKeys, opts))
	require.Equal(t, 1, exec.calls)
	require.Len(t, exec.batches, 1)
	require.Len(t, exec.batches[0], 2)

	var seenPut, seenDelete bool
	for _, req := range exec.batches[0] {
		if req.PutRequest != nil {
			seenPut = true
		}
		if req.DeleteRequest != nil {
			seenDelete = true
		}
	}
	require.True(t, seenPut)
	require.True(t, seenDelete)
}

func TestQuery_BatchWrite_UsesDefaultOptions_COV5(t *testing.T) {
	exec := &cov5BatchWriteExecutor{}
	q := New(&struct{}{}, cov5Metadata{
		table:      "tbl",
		primaryKey: core.KeySchema{PartitionKey: "pk"},
	}, exec)

	type item struct {
		PK string `dynamodb:"pk"`
	}

	require.NoError(t, q.BatchWrite([]any{&item{PK: "p1"}}, []any{&item{PK: "d1"}}))
	require.Equal(t, 1, exec.calls)
}

func TestQuery_BatchWriteWithOptions_ErrorHandler_SkipsInvalidItems(t *testing.T) {
	exec := &cov5BatchWriteExecutor{}
	q := New(&struct{}{}, cov5Metadata{
		table:      "tbl",
		primaryKey: core.KeySchema{PartitionKey: "pk"},
	}, exec)

	type item struct {
		PK string `dynamodb:"pk"`
	}

	opts := DefaultBatchOptions()
	opts.ErrorHandler = func(_ any, _ error) error { return nil }

	putItems := []any{
		make(chan int), // cannot be marshaled
		&item{PK: "p1"},
	}
	deleteKeys := []any{
		&struct{ ID string }{ID: "missing-key"},
		&item{PK: "d1"},
	}

	require.NoError(t, q.BatchWriteWithOptions(putItems, deleteKeys, opts))
	require.Equal(t, 1, exec.calls)
	require.Len(t, exec.batches, 1)
	require.Len(t, exec.batches[0], 2)
}
