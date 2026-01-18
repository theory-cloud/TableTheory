package query

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/internal/expr"
	"github.com/theory-cloud/tabletheory/pkg/core"
)

type cov6QueryOnlyExecutor struct{}

func (cov6QueryOnlyExecutor) ExecuteQuery(*core.CompiledQuery, any) error { return nil }
func (cov6QueryOnlyExecutor) ExecuteScan(*core.CompiledQuery, any) error  { return nil }

type cov6UpdateOnlyExecutor struct {
	calls    int
	compiled *core.CompiledQuery
	key      map[string]types.AttributeValue
}

func (e *cov6UpdateOnlyExecutor) ExecuteQuery(*core.CompiledQuery, any) error { return nil }
func (e *cov6UpdateOnlyExecutor) ExecuteScan(*core.CompiledQuery, any) error  { return nil }

func (e *cov6UpdateOnlyExecutor) ExecuteUpdateItem(compiled *core.CompiledQuery, key map[string]types.AttributeValue) error {
	e.calls++
	e.compiled = compiled
	e.key = key
	return nil
}

type cov6UpdateResultEmptyExecutor struct {
	calls int
}

func (e *cov6UpdateResultEmptyExecutor) ExecuteQuery(*core.CompiledQuery, any) error { return nil }
func (e *cov6UpdateResultEmptyExecutor) ExecuteScan(*core.CompiledQuery, any) error  { return nil }

func (e *cov6UpdateResultEmptyExecutor) ExecuteUpdateItem(*core.CompiledQuery, map[string]types.AttributeValue) error {
	return nil
}

func (e *cov6UpdateResultEmptyExecutor) ExecuteUpdateItemWithResult(*core.CompiledQuery, map[string]types.AttributeValue) (*core.UpdateResult, error) {
	e.calls++
	return &core.UpdateResult{Attributes: map[string]types.AttributeValue{}}, nil
}

func TestUpdateBuilder_Delete_HandlesValueShapes_COV6(t *testing.T) {
	ubAny := NewUpdateBuilder(&Query{})
	ub, ok := ubAny.(*UpdateBuilder)
	require.True(t, ok)

	ub.Delete("tags", "a")
	ub.Delete("nums", 1)
	ub.Delete("bin", []byte("x"))
	ub.Delete("strings", []string{"a", "b"})
	ub.Delete("otherSlice", []int32{1, 2})
	ub.Delete("wrapped", struct{ A string }{A: "x"})

	components := ub.expr.Build()
	require.NotEmpty(t, components.UpdateExpression)
	require.Contains(t, components.UpdateExpression, "DELETE")
}

func TestUpdateBuilder_Execute_RejectsNilQueryOrMetadata_COV6(t *testing.T) {
	ub := &UpdateBuilder{
		query:        nil,
		expr:         expr.NewBuilder(),
		keyValues:    make(map[string]any),
		returnValues: "NONE",
	}
	require.Error(t, ub.Execute())

	ub.query = &Query{metadata: nil}
	require.Error(t, ub.Execute())
}

func TestUpdateBuilder_Execute_FailsOnKeyConversion_COV6(t *testing.T) {
	exec := &cov6UpdateOnlyExecutor{}
	q := New(&struct{}{}, cov5Metadata{
		table:      "tbl",
		primaryKey: core.KeySchema{PartitionKey: "pk"},
	}, exec)

	q.Where("pk", "=", make(chan int))

	ubAny := NewUpdateBuilder(q)
	ub := ubAny.(*UpdateBuilder)

	require.Error(t, ub.Set("status", "ok").Execute())
	require.Equal(t, 0, exec.calls, "fails before executor is invoked")
}

func TestUpdateBuilder_Execute_ReturnsErrorWhenExecutorUnsupported_COV6(t *testing.T) {
	q := New(&struct{}{}, cov5Metadata{
		table:      "tbl",
		primaryKey: core.KeySchema{PartitionKey: "pk"},
	}, cov6QueryOnlyExecutor{})
	q.Where("pk", "=", "p1")

	ub := NewUpdateBuilder(q).(*UpdateBuilder)
	require.Error(t, ub.Set("status", "ok").Execute())
}

func TestUpdateBuilder_Execute_PropagatesQueryConditionBuildError_COV6(t *testing.T) {
	exec := &cov6UpdateOnlyExecutor{}
	q := New(&struct{}{}, cov5Metadata{
		table:      "tbl",
		primaryKey: core.KeySchema{PartitionKey: "pk"},
	}, exec)

	q.Where("pk", "=", "p1")
	q.writeConditions = append(q.writeConditions, Condition{Field: "", Operator: "=", Value: "x"})

	ub := NewUpdateBuilder(q).(*UpdateBuilder)
	require.Error(t, ub.Set("status", "ok").Execute())
}

func TestUpdateBuilder_ExecuteWithResult_FallsBackToUpdateOnlyExecutor_COV6(t *testing.T) {
	exec := &cov6UpdateOnlyExecutor{}
	q := New(&struct{}{}, cov5Metadata{
		table:      "tbl",
		primaryKey: core.KeySchema{PartitionKey: "pk"},
	}, exec)
	q.Where("pk", "=", "p1")

	ub := NewUpdateBuilder(q).(*UpdateBuilder)

	var out struct{}
	require.NoError(t, ub.Set("status", "ok").ExecuteWithResult(&out))
	require.Equal(t, 1, exec.calls)
	require.NotNil(t, exec.compiled)
	require.Equal(t, "ALL_NEW", exec.compiled.ReturnValues)
}

func TestUpdateBuilder_ExecuteWithResult_ReturnsNilOnEmptyUpdateResult_COV6(t *testing.T) {
	exec := &cov6UpdateResultEmptyExecutor{}
	q := New(&struct{}{}, cov5Metadata{
		table:      "tbl",
		primaryKey: core.KeySchema{PartitionKey: "pk"},
	}, exec)
	q.Where("pk", "=", "p1")

	ub := NewUpdateBuilder(q).(*UpdateBuilder)

	var out struct{}
	require.NoError(t, ub.Set("status", "ok").ExecuteWithResult(&out))
	require.Equal(t, 1, exec.calls)
}

func TestUpdateBuilder_Execute_RequiresSortKeyWhenConfigured_COV6(t *testing.T) {
	exec := &cov6UpdateOnlyExecutor{}
	q := New(&struct{}{}, cov5Metadata{
		table: "tbl",
		primaryKey: core.KeySchema{
			PartitionKey: "pk",
			SortKey:      "sk",
		},
	}, exec)

	q.Where("pk", "=", "p1")

	ub := NewUpdateBuilder(q).(*UpdateBuilder)
	require.Error(t, ub.Set("status", "ok").Execute())
}

func TestUpdateBuilder_Execute_FailsOnInvalidConditionOperator_COV6(t *testing.T) {
	exec := &cov6UpdateOnlyExecutor{}
	q := New(&struct{}{}, cov5Metadata{
		table:      "tbl",
		primaryKey: core.KeySchema{PartitionKey: "pk"},
	}, exec)
	q.Where("pk", "=", "p1")

	ub := NewUpdateBuilder(q).(*UpdateBuilder)
	require.Error(t, ub.Condition("pk", "INVALID", nil).Execute())
}
