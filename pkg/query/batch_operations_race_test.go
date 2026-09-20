package query

import (
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v3/pkg/core"
)

// batchUpdateEchoExecutor accepts every update without touching shared state,
// so it is safe for concurrent batch workers.
type batchUpdateEchoExecutor struct{}

func (e *batchUpdateEchoExecutor) ExecuteQuery(_ *core.CompiledQuery, _ any) error { return nil }
func (e *batchUpdateEchoExecutor) ExecuteScan(_ *core.CompiledQuery, _ any) error  { return nil }
func (e *batchUpdateEchoExecutor) ExecuteUpdateItem(_ *core.CompiledQuery, _ map[string]types.AttributeValue) error {
	return nil
}

type batchUpdateRaceItem struct {
	PK     string `theorydb:"pk"`
	Status string `theorydb:"attr:status"`
}

// TestBatchUpdateParallelProgressCallbackSerialized guards against the library
// invoking the user-supplied ProgressCallback concurrently when batches run in
// parallel. The callback writes shared state with no synchronization, so this
// test fails under -race if a future change reintroduces concurrent delivery.
func TestBatchUpdateParallelProgressCallbackSerialized(t *testing.T) {
	exec := &batchUpdateEchoExecutor{}
	q := New(&batchUpdateRaceItem{}, cov5Metadata{
		table:      "tbl",
		primaryKey: core.KeySchema{PartitionKey: "pk"},
	}, exec)

	const totalItems = 90
	const maxBatchSize = 1
	items := make([]any, 0, totalItems)
	for i := 0; i < totalItems; i++ {
		items = append(items, batchUpdateRaceItem{
			PK:     "p000",
			Status: "active",
		})
	}

	var calls, lastProcessed int
	opts := &BatchUpdateOptions{
		MaxBatchSize:   maxBatchSize,
		Parallel:       true,
		MaxConcurrency: 8,
		ProgressCallback: func(processed, total int) {
			calls++
			lastProcessed = processed
			require.Equal(t, totalItems, total)
		},
	}

	err := q.BatchUpdateWithOptions(items, []string{"status"}, opts)
	require.NoError(t, err)
	require.Equal(t, totalItems/maxBatchSize, calls)
	require.Equal(t, totalItems, lastProcessed)
}

// batchUpdateFailingExecutor fails every update without touching shared state,
// so it is safe for concurrent batch workers.
type batchUpdateFailingExecutor struct{}

func (e *batchUpdateFailingExecutor) ExecuteQuery(_ *core.CompiledQuery, _ any) error { return nil }
func (e *batchUpdateFailingExecutor) ExecuteScan(_ *core.CompiledQuery, _ any) error  { return nil }
func (e *batchUpdateFailingExecutor) ExecuteUpdateItem(_ *core.CompiledQuery, _ map[string]types.AttributeValue) error {
	return errors.New("update failed")
}

// TestBatchUpdateParallelErrorHandlerSerialized guards against the library
// invoking the user-supplied ErrorHandler concurrently when batches run in
// parallel. The handler writes shared state with no synchronization, so this
// test fails under -race if a future change reintroduces concurrent delivery.
func TestBatchUpdateParallelErrorHandlerSerialized(t *testing.T) {
	exec := &batchUpdateFailingExecutor{}
	q := New(&batchUpdateRaceItem{}, cov5Metadata{
		table:      "tbl",
		primaryKey: core.KeySchema{PartitionKey: "pk"},
	}, exec)

	const totalItems = 90
	const maxBatchSize = 1
	items := make([]any, 0, totalItems)
	for i := 0; i < totalItems; i++ {
		items = append(items, batchUpdateRaceItem{
			PK:     "p000",
			Status: "active",
		})
	}

	var calls int
	opts := &BatchUpdateOptions{
		MaxBatchSize:   maxBatchSize,
		Parallel:       true,
		MaxConcurrency: 8,
		ErrorHandler: func(item any, err error) error {
			calls++
			require.Error(t, err)
			return err
		},
	}

	err := q.BatchUpdateWithOptions(items, []string{"status"}, opts)
	require.Error(t, err)
	require.Equal(t, totalItems/maxBatchSize, calls)
}
