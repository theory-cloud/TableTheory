package query

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/core"
)

type cov5PaginatedExecutor struct {
	paginatedQueryCalls int
	paginatedScanCalls  int
	queryCalls          int
	scanCalls           int
}

func (e *cov5PaginatedExecutor) ExecuteQuery(_ *core.CompiledQuery, _ any) error {
	e.queryCalls++
	return nil
}

func (e *cov5PaginatedExecutor) ExecuteScan(_ *core.CompiledQuery, _ any) error {
	e.scanCalls++
	return nil
}

func (e *cov5PaginatedExecutor) ExecuteQueryWithPagination(_ *core.CompiledQuery, _ any) (*QueryResult, error) {
	e.paginatedQueryCalls++
	return &QueryResult{
		Count:        1,
		ScannedCount: 2,
		LastEvaluatedKey: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: "p1"},
		},
	}, nil
}

func (e *cov5PaginatedExecutor) ExecuteScanWithPagination(_ *core.CompiledQuery, _ any) (*ScanResult, error) {
	e.paginatedScanCalls++
	return &ScanResult{
		Count:        3,
		ScannedCount: 4,
		LastEvaluatedKey: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: "s1"},
		},
	}, nil
}

type cov5BasicExecutor struct {
	queryCalls int
	scanCalls  int
}

func (e *cov5BasicExecutor) ExecuteQuery(_ *core.CompiledQuery, _ any) error {
	e.queryCalls++
	return nil
}

func (e *cov5BasicExecutor) ExecuteScan(_ *core.CompiledQuery, _ any) error {
	e.scanCalls++
	return nil
}

func TestQuery_AllPaginated_UsesPaginatedExecutorAndEncodesCursor(t *testing.T) {
	exec := &cov5PaginatedExecutor{}
	q := New(&struct{}{}, cov5Metadata{
		table:      "tbl",
		primaryKey: core.KeySchema{PartitionKey: "pk"},
	}, exec)
	q.Index("gsi1")
	q.OrderBy("sk", "desc")
	q.Where("pk", "=", "p1")

	var out []map[string]types.AttributeValue
	res, err := q.AllPaginated(&out)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.True(t, res.HasMore)
	require.NotEmpty(t, res.NextCursor)
	require.Equal(t, 1, res.Count)
	require.Equal(t, 2, res.ScannedCount)
	require.NotEmpty(t, res.LastEvaluatedKey)

	require.Equal(t, 1, exec.paginatedQueryCalls)
	require.Zero(t, exec.queryCalls)
	require.Zero(t, exec.paginatedScanCalls)
	require.Zero(t, exec.scanCalls)
}

func TestQuery_AllPaginated_FallsBackWithoutPaginationInfo(t *testing.T) {
	exec := &cov5BasicExecutor{}
	q := New(&struct{}{}, cov5Metadata{
		table:      "tbl",
		primaryKey: core.KeySchema{PartitionKey: "pk"},
	}, exec)
	q.Where("pk", "=", "p1")

	var out []map[string]types.AttributeValue
	res, err := q.AllPaginated(&out)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.False(t, res.HasMore)
	require.Empty(t, res.NextCursor)
	require.Equal(t, 0, res.Count)
	require.Equal(t, 0, res.ScannedCount)

	require.Equal(t, 1, exec.queryCalls)
	require.Zero(t, exec.scanCalls)
}

func TestQuery_AllPaginated_ScanPath(t *testing.T) {
	exec := &cov5PaginatedExecutor{}
	q := New(&struct{}{}, cov5Metadata{
		table:      "tbl",
		primaryKey: core.KeySchema{PartitionKey: "pk"},
	}, exec)
	q.Where("pk", "<", "p1")

	var out []map[string]types.AttributeValue
	res, err := q.AllPaginated(&out)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, 3, res.Count)
	require.Equal(t, 4, res.ScannedCount)

	require.Equal(t, 1, exec.paginatedScanCalls)
	require.Zero(t, exec.paginatedQueryCalls)
}

func TestQuery_SetCursor_DecodesAndAppliesToCompile(t *testing.T) {
	exec := &cov5BasicExecutor{}
	q := New(&struct{}{}, cov5Metadata{
		table:      "tbl",
		primaryKey: core.KeySchema{PartitionKey: "pk"},
	}, exec)
	q.Where("pk", "=", "p1")

	cursor, err := EncodeCursor(map[string]types.AttributeValue{
		"pk": &types.AttributeValueMemberS{Value: "p1"},
	}, "", "")
	require.NoError(t, err)

	require.NoError(t, q.SetCursor(cursor))

	compiled, err := q.Compile()
	require.NoError(t, err)
	require.NotEmpty(t, compiled.ExclusiveStartKey)
}

func TestQuery_Cursor_InvalidInputRecordsError(t *testing.T) {
	exec := &cov5BasicExecutor{}
	q := New(&struct{}{}, cov5Metadata{
		table:      "tbl",
		primaryKey: core.KeySchema{PartitionKey: "pk"},
	}, exec)
	q.Where("pk", "=", "p1")
	q.Cursor("not-base64!!")

	var out []map[string]types.AttributeValue
	_, err := q.AllPaginated(&out)
	require.Error(t, err)
}

func TestQuery_encodeCursor_AdditionalBranches(t *testing.T) {
	exec := &cov5BasicExecutor{}
	q := New(&struct{}{}, cov5Metadata{
		table:      "tbl",
		primaryKey: core.KeySchema{PartitionKey: "pk"},
	}, exec)

	encoded := q.encodeCursor(map[string]any{
		"LastEvaluatedKey": map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: "p1"},
		},
	})
	require.NotEmpty(t, encoded)

	require.Empty(t, q.encodeCursor("not-a-key-map"))
	require.Empty(t, q.encodeCursor(map[string]types.AttributeValue{}))
}
