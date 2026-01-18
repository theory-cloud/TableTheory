package consistency

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/core"
	queryPkg "github.com/theory-cloud/tabletheory/pkg/query"
)

type cov5ConsistencyMetadata struct{}

func (cov5ConsistencyMetadata) TableName() string                                { return "tbl" }
func (cov5ConsistencyMetadata) PrimaryKey() core.KeySchema                       { return core.KeySchema{} }
func (cov5ConsistencyMetadata) Indexes() []core.IndexSchema                      { return nil }
func (cov5ConsistencyMetadata) AttributeMetadata(string) *core.AttributeMetadata { return nil }
func (cov5ConsistencyMetadata) VersionFieldName() string                         { return "" }

type cov5ConsistencyExecutor struct {
	last *core.CompiledQuery
}

func (e *cov5ConsistencyExecutor) ExecuteQuery(input *core.CompiledQuery, dest any) error {
	e.last = input
	if dest != nil {
		destValue := reflect.ValueOf(dest)
		if destValue.Kind() == reflect.Ptr && destValue.Elem().Kind() == reflect.Slice && destValue.Elem().Len() == 0 {
			zero := reflect.Zero(destValue.Elem().Type().Elem())
			destValue.Elem().Set(reflect.Append(destValue.Elem(), zero))
		}
	}
	return nil
}

func (e *cov5ConsistencyExecutor) ExecuteScan(input *core.CompiledQuery, dest any) error {
	e.last = input
	if dest != nil {
		destValue := reflect.ValueOf(dest)
		if destValue.Kind() == reflect.Ptr && destValue.Elem().Kind() == reflect.Slice && destValue.Elem().Len() == 0 {
			zero := reflect.Zero(destValue.Elem().Type().Elem())
			destValue.Elem().Set(reflect.Append(destValue.Elem(), zero))
		}
	}
	return nil
}

type cov5DB struct {
	query core.Query
}

func (d cov5DB) Model(any) core.Query                      { return d.query }
func (d cov5DB) Transaction(func(tx *core.Tx) error) error { return nil }
func (d cov5DB) Migrate() error                            { return nil }
func (d cov5DB) AutoMigrate(...any) error                  { return nil }
func (d cov5DB) Close() error                              { return nil }
func (d cov5DB) WithContext(context.Context) core.DB       { return d }

func TestConsistentQueryBuilder_First_UseMainTableUsesConsistentRead(t *testing.T) {
	exec := &cov5ConsistencyExecutor{}
	baseQuery := queryPkg.New(&struct{}{}, cov5ConsistencyMetadata{}, exec)

	helper := NewReadAfterWriteHelper(cov5DB{query: baseQuery})
	builder := helper.QueryAfterWrite(&struct{}{}, &QueryAfterWriteOptions{
		UseMainTable: true,
	})

	var out struct{}
	require.NoError(t, builder.First(&out))
	require.NotNil(t, exec.last)
	require.NotNil(t, exec.last.ConsistentRead)
	require.True(t, *exec.last.ConsistentRead)
}
