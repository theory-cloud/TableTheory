package query

import (
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
