package query

import (
	"errors"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/internal/expr"
	"github.com/theory-cloud/tabletheory/pkg/core"
)

type cov6NoopExecutor struct{}

func (cov6NoopExecutor) ExecuteQuery(*core.CompiledQuery, any) error { return nil }
func (cov6NoopExecutor) ExecuteScan(*core.CompiledQuery, any) error  { return nil }

type cov6PutExecutor struct {
	compiled *core.CompiledQuery
	item     map[string]types.AttributeValue
}

func (e *cov6PutExecutor) ExecuteQuery(*core.CompiledQuery, any) error { return nil }
func (e *cov6PutExecutor) ExecuteScan(*core.CompiledQuery, any) error  { return nil }
func (e *cov6PutExecutor) ExecutePutItem(input *core.CompiledQuery, item map[string]types.AttributeValue) error {
	e.compiled = input
	e.item = item
	return nil
}

type cov6CountExecutor struct {
	queries int
	scans   int
}

func (e *cov6CountExecutor) ExecuteQuery(_ *core.CompiledQuery, dest any) error {
	e.queries++
	setStructFieldInt(dest, "Count", 7)
	return nil
}

func (e *cov6CountExecutor) ExecuteScan(_ *core.CompiledQuery, dest any) error {
	e.scans++
	setStructFieldInt(dest, "Count", 3)
	return nil
}

func setStructFieldInt(dest any, name string, value int64) {
	v := reflect.ValueOf(dest)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return
	}
	elem := v.Elem()
	if elem.Kind() != reflect.Struct {
		return
	}
	field := elem.FieldByName(name)
	if !field.IsValid() || !field.CanSet() || field.Kind() != reflect.Int64 {
		return
	}
	field.SetInt(value)
}

func TestQuery_normalizeCondition_UsesMetaNameWhenDynamoDBNameEmpty_COV6(t *testing.T) {
	q := &Query{metadata: cov5Metadata{
		attributes: map[string]*core.AttributeMetadata{
			"Field": {Name: "GoName", DynamoDBName: ""},
		},
	}}

	normalized, goField, attrName := q.normalizeCondition(Condition{Field: "Field"})
	require.Equal(t, "GoName", normalized.Field)
	require.Equal(t, "GoName", goField)
	require.Equal(t, "GoName", attrName)
}

func TestCloneConditionValues_EmptyMapReturnsNil_COV6(t *testing.T) {
	require.Nil(t, cloneConditionValues(nil))
	require.Nil(t, cloneConditionValues(map[string]any{}))
}

func TestMergeConditionExpressions_Branches_COV6(t *testing.T) {
	t.Run("skips empty expressions and merges into nil map", func(t *testing.T) {
		mergedExpr, mergedValues, err := mergeConditionExpressions("", nil, []conditionExpression{
			{Expression: "a = :x", Values: map[string]any{":x": "1"}},
			{Expression: "", Values: map[string]any{":y": "2"}},
		}, nil)
		require.NoError(t, err)
		require.Equal(t, "a = :x", mergedExpr)
		require.Contains(t, mergedValues, ":x")
	})

	t.Run("detects duplicate placeholders", func(t *testing.T) {
		_, _, err := mergeConditionExpressions("a = :x", map[string]types.AttributeValue{
			":x": &types.AttributeValueMemberS{Value: "1"},
		}, []conditionExpression{
			{Expression: "b = :x", Values: map[string]any{":x": "dup"}},
		}, nil)
		require.ErrorContains(t, err, "duplicate placeholder")
	})

	t.Run("fails when conversion fails", func(t *testing.T) {
		_, _, err := mergeConditionExpressions("", nil, []conditionExpression{
			{Expression: "a = :x", Values: map[string]any{":x": make(chan int)}},
		}, nil)
		require.Error(t, err)
	})
}

func TestQuery_buildConditionExpression_ErrorBranches_COV6(t *testing.T) {
	t.Run("where conditions propagate errors", func(t *testing.T) {
		q := &Query{}
		_, _, _, err := q.buildConditionExpression(expr.NewBuilder(), true, false, false)
		require.Error(t, err)
	})

	t.Run("default condition errors propagate", func(t *testing.T) {
		q := &Query{metadata: cov5Metadata{primaryKey: core.KeySchema{PartitionKey: ""}}}
		_, _, _, err := q.buildConditionExpression(expr.NewBuilder(), false, false, true)
		require.Error(t, err)
	})

	t.Run("merge errors propagate", func(t *testing.T) {
		q := &Query{
			writeConditions: []Condition{{Field: "status", Operator: "=", Value: "ok"}},
			rawConditionExpressions: []conditionExpression{
				{Expression: "status = :v1", Values: map[string]any{":v1": "dup"}},
			},
		}

		_, _, _, err := q.buildConditionExpression(expr.NewBuilder(), false, false, false)
		require.ErrorContains(t, err, "duplicate placeholder")
	})
}

func TestQuery_addWriteConditions_And_addWhereConditions_ErrorBranches_COV6(t *testing.T) {
	t.Run("addWriteConditions wraps builder errors", func(t *testing.T) {
		q := &Query{writeConditions: []Condition{
			{Field: "status", Operator: "NOPE", Value: "x"},
		}}

		_, err := q.addWriteConditions(expr.NewBuilder())
		require.Error(t, err)
	})

	t.Run("addWhereConditions wraps builder errors", func(t *testing.T) {
		q := &Query{
			metadata: cov5Metadata{primaryKey: core.KeySchema{PartitionKey: "ID"}},
			conditions: []Condition{
				{Field: "Other", Operator: "NOPE", Value: "x"},
			},
		}

		_, err := q.addWhereConditions(expr.NewBuilder(), false)
		require.Error(t, err)
	})
}

func TestQuery_Create_ErrorBranches_COV6(t *testing.T) {
	t.Run("returns builder errors", func(t *testing.T) {
		q := &Query{builderErr: errors.New("boom")}
		require.ErrorContains(t, q.Create(), "boom")
	})

	t.Run("wraps marshal errors", func(t *testing.T) {
		q := &Query{model: 42}
		require.ErrorContains(t, q.Create(), "failed to marshal item")
	})

	t.Run("bubbles condition build errors", func(t *testing.T) {
		q := &Query{
			model:           &struct{}{},
			metadata:        cov5Metadata{table: "tbl"},
			executor:        cov6NoopExecutor{},
			writeConditions: []Condition{{Field: "status", Operator: "NOPE", Value: "x"}},
		}
		require.Error(t, q.Create())
	})

	t.Run("errors when executor lacks PutItem support", func(t *testing.T) {
		q := &Query{
			model:    &struct{}{},
			metadata: cov5Metadata{table: "tbl"},
			executor: cov6NoopExecutor{},
		}
		require.ErrorContains(t, q.Create(), "does not support PutItem")
	})
}

type cov6UpsertModelOK struct {
	ID         string   `theorydb:"pk"`
	Ignored    string   `theorydb:"-"`
	Optional   string   `theorydb:"attr:optional,omitempty"`
	When       struct{} `theorydb:"attr:when,omitempty"`
	Bad        chan int `theorydb:"attr:bad,omitempty"`
	unexported string   // should be skipped
}

type cov6UpsertModelBad struct {
	BadHard chan int `theorydb:"attr:bad_hard"`
	ID      string   `theorydb:"pk"`
}

func TestQuery_CreateOrUpdate_Branches_COV6(t *testing.T) {
	meta := cov5Metadata{table: "tbl", primaryKey: core.KeySchema{PartitionKey: "ID"}}

	t.Run("skips unexported and ignored fields", func(t *testing.T) {
		exec := &cov6PutExecutor{}
		item := &cov6UpsertModelOK{ID: "id-1", unexported: "secret"}
		require.Equal(t, "secret", item.unexported)

		q := New(item, meta, exec)
		require.NoError(t, q.CreateOrUpdate())

		require.NotNil(t, exec.compiled)
		require.Equal(t, "PutItem", exec.compiled.Operation)
		require.Contains(t, exec.item, "ID")
		require.NotContains(t, exec.item, "Ignored")
		require.NotContains(t, exec.item, "unexported")
		require.NotContains(t, exec.item, "Optional")
		require.NotContains(t, exec.item, "When")
	})

	t.Run("returns conversion errors for unsupported non-omitempty fields", func(t *testing.T) {
		exec := &cov6PutExecutor{}
		item := &cov6UpsertModelBad{ID: "id-1", BadHard: make(chan int)}

		q := New(item, meta, exec)
		require.Error(t, q.CreateOrUpdate())
	})

	t.Run("errors when executor lacks PutItem support", func(t *testing.T) {
		item := &cov6UpsertModelOK{ID: "id-1"}
		q := New(item, meta, cov6NoopExecutor{})
		require.ErrorContains(t, q.CreateOrUpdate(), "does not support PutItem")
	})
}

func TestQuery_UpdateAndDelete_UnsupportedExecutors_COV6(t *testing.T) {
	type item struct {
		ID     string `theorydb:"pk"`
		Status string `theorydb:"attr:status"`
	}

	meta := cov5Metadata{table: "tbl", primaryKey: core.KeySchema{PartitionKey: "ID"}}

	t.Run("Update requires UpdateItem executor", func(t *testing.T) {
		q := New(&item{ID: "id-1", Status: "ok"}, meta, cov6NoopExecutor{}).
			Where("ID", "=", "id-1")
		require.ErrorContains(t, q.Update(), "does not support UpdateItem")
	})

	t.Run("Delete requires DeleteItem executor", func(t *testing.T) {
		q := New(&item{ID: "id-1"}, meta, cov6NoopExecutor{}).
			Where("ID", "=", "id-1")
		require.ErrorContains(t, q.Delete(), "does not support DeleteItem")
	})
}

func TestQuery_Count_QueryAndScanBranches_COV6(t *testing.T) {
	meta := cov5Metadata{table: "tbl", primaryKey: core.KeySchema{PartitionKey: "ID"}}
	exec := &cov6CountExecutor{}

	t.Run("query path", func(t *testing.T) {
		q := New(&struct{ ID string }{}, meta, exec).Where("ID", "=", "id-1")
		count, err := q.Count()
		require.NoError(t, err)
		require.Equal(t, int64(7), count)
	})

	t.Run("scan path", func(t *testing.T) {
		q := New(&struct{ ID string }{}, meta, exec).Where("Other", "=", "x")
		count, err := q.Count()
		require.NoError(t, err)
		require.Equal(t, int64(3), count)
	})

	require.GreaterOrEqual(t, exec.queries, 1)
	require.GreaterOrEqual(t, exec.scans, 1)
}
