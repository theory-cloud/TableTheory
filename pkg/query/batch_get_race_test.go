package query

import (
	"errors"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v3/pkg/core"
)

// batchGetEchoExecutor returns every requested key as a matching item without
// touching shared state, so it is safe for concurrent chunk workers.
type batchGetEchoExecutor struct{}

func (e *batchGetEchoExecutor) ExecuteQuery(_ *core.CompiledQuery, _ any) error { return nil }
func (e *batchGetEchoExecutor) ExecuteScan(_ *core.CompiledQuery, _ any) error  { return nil }
func (e *batchGetEchoExecutor) ExecuteBatchWrite(_ *CompiledBatchWrite) error   { return nil }

func (e *batchGetEchoExecutor) ExecuteBatchGet(input *CompiledBatchGet, _ *core.BatchGetOptions) ([]map[string]types.AttributeValue, error) {
	items := make([]map[string]types.AttributeValue, 0, len(input.Keys))
	for _, key := range input.Keys {
		item := make(map[string]types.AttributeValue, len(key))
		for attr, value := range key {
			item[attr] = value
		}
		items = append(items, item)
	}
	return items, nil
}

// TestBatchGetParallelProgressCallbackSerialized guards against the library
// invoking the user-supplied ProgressCallback concurrently when chunks run in
// parallel. The callback writes shared state with no synchronization, so this
// test fails under -race if a future change reintroduces concurrent delivery.
func TestBatchGetParallelProgressCallbackSerialized(t *testing.T) {
	exec := &batchGetEchoExecutor{}
	q := New(&struct{}{}, cov5Metadata{
		table:      "tbl",
		primaryKey: core.KeySchema{PartitionKey: "pk"},
	}, exec)

	const totalKeys = 90
	const chunkSize = 10
	keys := make([]any, 0, totalKeys)
	for i := 0; i < totalKeys; i++ {
		keys = append(keys, core.NewKeyPair(fmt.Sprintf("p%03d", i)))
	}

	var calls, lastRetrieved int
	opts := core.DefaultBatchGetOptions()
	opts.ChunkSize = chunkSize
	opts.Parallel = true
	opts.MaxConcurrency = 8
	opts.ProgressCallback = func(retrieved, total int) {
		calls++
		lastRetrieved = retrieved
		require.Equal(t, totalKeys, total)
	}

	var out []map[string]types.AttributeValue
	err := q.BatchGetWithOptions(keys, &out, opts)
	require.NoError(t, err)
	require.Len(t, out, totalKeys)
	require.Equal(t, totalKeys/chunkSize, calls)
	require.Equal(t, totalKeys, lastRetrieved)
}

// batchGetFailingExecutor fails every requested chunk without touching shared
// state, so it is safe for concurrent chunk workers.
type batchGetFailingExecutor struct{}

func (e *batchGetFailingExecutor) ExecuteQuery(_ *core.CompiledQuery, _ any) error { return nil }
func (e *batchGetFailingExecutor) ExecuteScan(_ *core.CompiledQuery, _ any) error  { return nil }
func (e *batchGetFailingExecutor) ExecuteBatchWrite(_ *CompiledBatchWrite) error   { return nil }

func (e *batchGetFailingExecutor) ExecuteBatchGet(_ *CompiledBatchGet, _ *core.BatchGetOptions) ([]map[string]types.AttributeValue, error) {
	return nil, errors.New("chunk failed")
}

// TestBatchGetParallelOnChunkErrorSerialized guards against the library invoking
// the user-supplied OnChunkError handler concurrently when chunks run in
// parallel. The handler writes shared state with no synchronization, so this
// test fails under -race if a future change reintroduces concurrent delivery.
func TestBatchGetParallelOnChunkErrorSerialized(t *testing.T) {
	exec := &batchGetFailingExecutor{}
	q := New(&struct{}{}, cov5Metadata{
		table:      "tbl",
		primaryKey: core.KeySchema{PartitionKey: "pk"},
	}, exec)

	const totalKeys = 90
	const chunkSize = 10
	keys := make([]any, 0, totalKeys)
	for i := 0; i < totalKeys; i++ {
		keys = append(keys, core.NewKeyPair(fmt.Sprintf("p%03d", i)))
	}

	var calls int
	opts := core.DefaultBatchGetOptions()
	opts.ChunkSize = chunkSize
	opts.Parallel = true
	opts.MaxConcurrency = 8
	opts.OnChunkError = func(chunk []any, err error) error {
		calls++
		require.Error(t, err)
		return err
	}

	var out []map[string]types.AttributeValue
	err := q.BatchGetWithOptions(keys, &out, opts)
	require.Error(t, err)
	require.Equal(t, totalKeys/chunkSize, calls)
}
