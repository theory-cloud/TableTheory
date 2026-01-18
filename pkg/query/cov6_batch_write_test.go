package query

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/core"
)

type cov6BatchWriteSeqExecutor struct {
	results []*core.BatchWriteResult
	errs    []error

	calls int
}

func (e *cov6BatchWriteSeqExecutor) ExecuteBatchWriteItem(_ string, _ []types.WriteRequest) (*core.BatchWriteResult, error) {
	call := e.calls
	e.calls++

	if call < len(e.errs) && e.errs[call] != nil {
		return nil, e.errs[call]
	}

	if call < len(e.results) && e.results[call] != nil {
		return e.results[call], nil
	}

	return &core.BatchWriteResult{UnprocessedItems: map[string][]types.WriteRequest{}}, nil
}

func (e *cov6BatchWriteSeqExecutor) ExecuteQuery(*core.CompiledQuery, any) error { return nil }
func (e *cov6BatchWriteSeqExecutor) ExecuteScan(*core.CompiledQuery, any) error  { return nil }

func TestQuery_executeBatchWriteWithRetries_CoversRetriesAndCallbacks_COV6(t *testing.T) {
	makeReq := func() types.WriteRequest {
		return types.WriteRequest{PutRequest: &types.PutRequest{Item: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: "x"},
		}}}
	}

	t.Run("no requests", func(t *testing.T) {
		q := New(&cov6BatchCreateItem{}, cov6Metadata{table: "tbl"}, &cov6BatchWriteSeqExecutor{})
		require.NoError(t, q.executeBatchWriteWithRetries("tbl", nil, nil))
	})

	t.Run("unsupported executor", func(t *testing.T) {
		q := New(&cov6BatchCreateItem{}, cov6Metadata{table: "tbl"}, &struct{ QueryExecutor }{})
		require.ErrorContains(t, q.executeBatchWriteWithRetries("tbl", []types.WriteRequest{makeReq()}, nil), "does not support batch write operations")
	})

	t.Run("executor returns error", func(t *testing.T) {
		exec := &cov6BatchWriteSeqExecutor{errs: []error{errors.New("boom")}}
		q := New(&cov6BatchCreateItem{}, cov6Metadata{table: "tbl"}, exec)
		require.ErrorContains(t, q.executeBatchWriteWithRetries("tbl", []types.WriteRequest{makeReq()}, nil), "batch write failed")
	})

	t.Run("unprocessed items empty", func(t *testing.T) {
		exec := &cov6BatchWriteSeqExecutor{results: []*core.BatchWriteResult{{UnprocessedItems: map[string][]types.WriteRequest{}}}}
		q := New(&cov6BatchCreateItem{}, cov6Metadata{table: "tbl"}, exec)
		require.NoError(t, q.executeBatchWriteWithRetries("tbl", []types.WriteRequest{makeReq()}, nil))
	})

	t.Run("unprocessed map present but empty slice", func(t *testing.T) {
		exec := &cov6BatchWriteSeqExecutor{results: []*core.BatchWriteResult{{
			UnprocessedItems: map[string][]types.WriteRequest{
				"tbl": {},
			},
		}}}
		q := New(&cov6BatchCreateItem{}, cov6Metadata{table: "tbl"}, exec)
		require.NoError(t, q.executeBatchWriteWithRetries("tbl", []types.WriteRequest{makeReq()}, nil))
	})

	t.Run("retries and progress callback", func(t *testing.T) {
		exec := &cov6BatchWriteSeqExecutor{results: []*core.BatchWriteResult{
			{UnprocessedItems: map[string][]types.WriteRequest{"tbl": {makeReq()}}},
			{UnprocessedItems: map[string][]types.WriteRequest{}},
		}}
		q := New(&cov6BatchCreateItem{}, cov6Metadata{table: "tbl"}, exec)

		var progressCalls int32
		opts := &BatchUpdateOptions{
			ProgressCallback: func(processed, total int) {
				_ = processed
				_ = total
				atomic.AddInt32(&progressCalls, 1)
			},
		}

		start := time.Now()
		require.NoError(t, q.executeBatchWriteWithRetries("tbl", []types.WriteRequest{makeReq(), makeReq()}, opts))
		require.GreaterOrEqual(t, atomic.LoadInt32(&progressCalls), int32(1))
		require.Less(t, time.Since(start), 2*time.Second)
	})
}

func TestQuery_BatchWriteWithOptions_HandlesErrorsAndProgress_COV6(t *testing.T) {
	t.Run("swallows execute errors via ErrorHandler", func(t *testing.T) {
		q := New(&cov6BatchCreateItem{}, cov5Metadata{
			table:      "tbl",
			primaryKey: core.KeySchema{PartitionKey: "ID"},
		}, &struct{ QueryExecutor }{})

		var errorsHandled int32
		var progressCalls int32

		opts := &BatchUpdateOptions{
			MaxBatchSize: 0,
			ErrorHandler: func(_ any, _ error) error {
				atomic.AddInt32(&errorsHandled, 1)
				return nil
			},
			ProgressCallback: func(_, _ int) {
				atomic.AddInt32(&progressCalls, 1)
			},
		}

		require.NoError(t, q.BatchWriteWithOptions([]any{cov6BatchCreateItem{ID: "1"}}, nil, opts))
		require.GreaterOrEqual(t, atomic.LoadInt32(&errorsHandled), int32(1))
		require.GreaterOrEqual(t, atomic.LoadInt32(&progressCalls), int32(1))
	})

	t.Run("skips bad inputs when ErrorHandler returns nil", func(t *testing.T) {
		exec := &cov6BatchWriteSeqExecutor{results: []*core.BatchWriteResult{{UnprocessedItems: map[string][]types.WriteRequest{}}}}
		q := New(&cov6BatchCreateItem{}, cov5Metadata{
			table:      "tbl",
			primaryKey: core.KeySchema{PartitionKey: "ID"},
		}, exec)

		var errorsHandled int32
		opts := &BatchUpdateOptions{
			MaxBatchSize: 25,
			ErrorHandler: func(_ any, _ error) error {
				atomic.AddInt32(&errorsHandled, 1)
				return nil
			},
		}

		putItems := []any{cov6BatchCreateItem{ID: "1"}, 123}
		deleteKeys := []any{cov6BatchCreateItem{ID: "2"}, struct{ Name string }{Name: "missing-id"}}

		require.NoError(t, q.BatchWriteWithOptions(putItems, deleteKeys, opts))
		require.GreaterOrEqual(t, atomic.LoadInt32(&errorsHandled), int32(1))
	})
}
