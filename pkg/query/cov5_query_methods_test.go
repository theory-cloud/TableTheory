package query

import (
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/core"
)

type cov5QueryExecutor struct {
	lastQuery  *core.CompiledQuery
	lastScan   *core.CompiledQuery
	queryCalls int
	scanCalls  int
}

func appendZeroElementToSlice(dest any) {
	rv := reflect.ValueOf(dest)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return
	}
	slice := rv.Elem()
	if slice.Kind() != reflect.Slice {
		return
	}

	elemType := slice.Type().Elem()
	if elemType.Kind() == reflect.Ptr {
		slice.Set(reflect.Append(slice, reflect.New(elemType.Elem())))
		return
	}
	slice.Set(reflect.Append(slice, reflect.New(elemType).Elem()))
}

func (e *cov5QueryExecutor) ExecuteQuery(input *core.CompiledQuery, dest any) error {
	e.queryCalls++
	e.lastQuery = input

	rv := reflect.ValueOf(dest)
	appendZeroElementToSlice(dest)
	if rv.Kind() == reflect.Ptr && rv.Elem().Kind() == reflect.Struct {
		count := rv.Elem().FieldByName("Count")
		if count.IsValid() && count.CanSet() && count.Kind() == reflect.Int64 {
			count.SetInt(7)
		}
		scanned := rv.Elem().FieldByName("ScannedCount")
		if scanned.IsValid() && scanned.CanSet() && scanned.Kind() == reflect.Int64 {
			scanned.SetInt(9)
		}
	}

	return nil
}

func (e *cov5QueryExecutor) ExecuteScan(input *core.CompiledQuery, dest any) error {
	e.scanCalls++
	e.lastScan = input
	appendZeroElementToSlice(dest)
	return nil
}

func TestQuery_First_UsesQueryOrScan(t *testing.T) {
	exec := &cov5QueryExecutor{}

	q := New(&struct{}{}, cov5Metadata{
		table:      "tbl",
		primaryKey: core.KeySchema{PartitionKey: "pk"},
	}, exec)
	q.Where("pk", "=", "p1")

	var out struct{}
	require.NoError(t, q.First(&out))
	require.Equal(t, 1, exec.queryCalls)
	require.NotNil(t, exec.lastQuery)
	require.NotNil(t, exec.lastQuery.Limit)
	require.Equal(t, int32(1), *exec.lastQuery.Limit)

	exec = &cov5QueryExecutor{}
	q = New(&struct{}{}, cov5Metadata{
		table:      "tbl",
		primaryKey: core.KeySchema{PartitionKey: "pk"},
	}, exec)
	q.Where("pk", "<", "p1")
	require.NoError(t, q.First(&out))
	require.Equal(t, 1, exec.scanCalls)
	require.NotNil(t, exec.lastScan)
}

func TestQuery_Count_SetsSelectAndReturnsCount(t *testing.T) {
	exec := &cov5QueryExecutor{}

	q := New(&struct{}{}, cov5Metadata{
		table:      "tbl",
		primaryKey: core.KeySchema{PartitionKey: "pk"},
	}, exec)
	q.Where("pk", "=", "p1")

	count, err := q.Count()
	require.NoError(t, err)
	require.Equal(t, int64(7), count)
	require.NotNil(t, exec.lastQuery)
	require.Equal(t, "COUNT", exec.lastQuery.Select)
}

func TestQuery_FilterGroups_RecordBuilderError(t *testing.T) {
	exec := &cov5QueryExecutor{}

	q := New(&struct{}{}, cov5Metadata{
		table:      "tbl",
		primaryKey: core.KeySchema{PartitionKey: "pk"},
	}, exec)

	q.OrFilter("field", "=", "v")
	q.FilterGroup(func(sub core.Query) {
		sub.Filter("bad", "not-an-op", "x")
	})

	var out []struct{}
	require.Error(t, q.All(&out))
}

func TestQuery_OrFilterGroup_CoversGroupedOrFilters_COV5(t *testing.T) {
	exec := &cov5QueryExecutor{}

	q := New(&struct{}{}, cov5Metadata{
		table:      "tbl",
		primaryKey: core.KeySchema{PartitionKey: "pk"},
	}, exec)

	q.OrFilterGroup(func(sub core.Query) {
		sub.Filter("field", "=", "v")
	})

	var out []struct{}
	require.NoError(t, q.All(&out))
}

func TestQuery_FilterGroup_PreservesEarlierFilterPlaceholders_COV5(t *testing.T) {
	exec := &cov5QueryExecutor{}

	q := New(&struct{}{}, cov5Metadata{
		table:      "tbl",
		primaryKey: core.KeySchema{PartitionKey: "pk"},
	}, exec)

	q.Filter("tenant_id", "=", "tenant-a")
	q.FilterGroup(func(sub core.Query) {
		sub.Filter("status", "=", "public")
	})

	var out []struct{}
	require.NoError(t, q.All(&out))
	require.NotNil(t, exec.lastScan)
	require.Contains(t, exec.lastScan.FilterExpression, "#n1 = :v1")
	require.Contains(t, exec.lastScan.FilterExpression, "(#STATUS = :v2)")

	require.Equal(t, "tenant_id", exec.lastScan.ExpressionAttributeNames["#n1"])
	require.Equal(t, "status", exec.lastScan.ExpressionAttributeNames["#STATUS"])

	tenantValue, ok := exec.lastScan.ExpressionAttributeValues[":v1"].(*types.AttributeValueMemberS)
	require.True(t, ok)
	require.Equal(t, "tenant-a", tenantValue.Value)

	statusValue, ok := exec.lastScan.ExpressionAttributeValues[":v2"].(*types.AttributeValueMemberS)
	require.True(t, ok)
	require.Equal(t, "public", statusValue.Value)
}
