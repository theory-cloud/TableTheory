package query

import (
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/core"
)

type cov6Metadata struct {
	table string
}

func (m cov6Metadata) TableName() string                                { return m.table }
func (m cov6Metadata) PrimaryKey() core.KeySchema                       { return core.KeySchema{} }
func (m cov6Metadata) Indexes() []core.IndexSchema                      { return nil }
func (m cov6Metadata) AttributeMetadata(string) *core.AttributeMetadata { return nil }
func (m cov6Metadata) VersionFieldName() string                         { return "" }

type cov6BatchWriteExecutor struct {
	err    error
	result *core.BatchWriteResult

	gotTableName string
	gotRequests  []types.WriteRequest

	calls int
}

func (e *cov6BatchWriteExecutor) ExecuteBatchWriteItem(tableName string, writeRequests []types.WriteRequest) (*core.BatchWriteResult, error) {
	e.calls++
	e.gotTableName = tableName
	e.gotRequests = append([]types.WriteRequest(nil), writeRequests...)
	return e.result, e.err
}

func (e *cov6BatchWriteExecutor) ExecuteQuery(*core.CompiledQuery, any) error { return nil }
func (e *cov6BatchWriteExecutor) ExecuteScan(*core.CompiledQuery, any) error  { return nil }

type cov6LegacyBatchExecutor struct {
	err error

	got *CompiledBatchWrite
}

func (e *cov6LegacyBatchExecutor) ExecuteBatchGet(*CompiledBatchGet, *core.BatchGetOptions) ([]map[string]types.AttributeValue, error) {
	return nil, errors.New("not implemented")
}

func (e *cov6LegacyBatchExecutor) ExecuteBatchWrite(input *CompiledBatchWrite) error {
	e.got = input
	return e.err
}

func (e *cov6LegacyBatchExecutor) ExecuteQuery(*core.CompiledQuery, any) error { return nil }
func (e *cov6LegacyBatchExecutor) ExecuteScan(*core.CompiledQuery, any) error  { return nil }

type cov6BatchCreateItem struct {
	ID string
}

func TestQuery_BatchCreate_ValidatesInputAndExecutorSupport_COV6(t *testing.T) {
	t.Run("items must be a slice", func(t *testing.T) {
		q := New(&cov6BatchCreateItem{}, cov6Metadata{table: "tbl"}, &cov6BatchWriteExecutor{})
		require.ErrorContains(t, q.BatchCreate(123), "items must be a slice")
	})

	t.Run("empty slice", func(t *testing.T) {
		exec := &cov6BatchWriteExecutor{result: &core.BatchWriteResult{UnprocessedItems: map[string][]types.WriteRequest{}}}
		q := New(&cov6BatchCreateItem{}, cov6Metadata{table: "tbl"}, exec)
		require.NoError(t, q.BatchCreate([]cov6BatchCreateItem{}))
		require.Equal(t, 0, exec.calls)
	})

	t.Run("too many items", func(t *testing.T) {
		items := make([]cov6BatchCreateItem, 26)
		exec := &cov6BatchWriteExecutor{result: &core.BatchWriteResult{UnprocessedItems: map[string][]types.WriteRequest{}}}
		q := New(&cov6BatchCreateItem{}, cov6Metadata{table: "tbl"}, exec)

		require.NoError(t, q.BatchCreate(items))
		require.Equal(t, 2, exec.calls)
		require.Equal(t, "tbl", exec.gotTableName)
		require.Len(t, exec.gotRequests, 1)
	})

	t.Run("batch write executor success", func(t *testing.T) {
		exec := &cov6BatchWriteExecutor{
			result: &core.BatchWriteResult{UnprocessedItems: map[string][]types.WriteRequest{}},
		}
		q := New(&cov6BatchCreateItem{}, cov6Metadata{table: "tbl"}, exec)

		require.NoError(t, q.BatchCreate([]cov6BatchCreateItem{{ID: "1"}, {ID: "2"}}))
		require.Equal(t, "tbl", exec.gotTableName)
		require.Len(t, exec.gotRequests, 2)
	})

	t.Run("batch write executor returns unprocessed items", func(t *testing.T) {
		exec := &cov6BatchWriteExecutor{
			result: &core.BatchWriteResult{
				UnprocessedItems: map[string][]types.WriteRequest{
					"tbl": {{PutRequest: &types.PutRequest{Item: map[string]types.AttributeValue{}}}},
				},
			},
		}
		q := New(&cov6BatchCreateItem{}, cov6Metadata{table: "tbl"}, exec)
		require.ErrorContains(t, q.BatchCreate([]cov6BatchCreateItem{{ID: "1"}}), "failed to process")
	})

	t.Run("item conversion error (non-struct)", func(t *testing.T) {
		exec := &cov6BatchWriteExecutor{result: &core.BatchWriteResult{}}
		q := New(&cov6BatchCreateItem{}, cov6Metadata{table: "tbl"}, exec)
		require.ErrorContains(t, q.BatchCreate([]any{123}), "failed to marshal item 0")
	})

	t.Run("legacy batch executor fallback", func(t *testing.T) {
		exec := &cov6LegacyBatchExecutor{}
		q := New(&cov6BatchCreateItem{}, cov6Metadata{table: "tbl"}, exec)

		require.NoError(t, q.BatchCreate([]cov6BatchCreateItem{{ID: "1"}}))
		require.NotNil(t, exec.got)
		require.Equal(t, "tbl", exec.got.TableName)
		require.Len(t, exec.got.Items, 1)
	})

	t.Run("no supported batch executor", func(t *testing.T) {
		q := New(&cov6BatchCreateItem{}, cov6Metadata{table: "tbl"}, &struct{ QueryExecutor }{})
		require.ErrorContains(t, q.BatchCreate([]cov6BatchCreateItem{{ID: "1"}}), "does not support batch operations")
	})
}
