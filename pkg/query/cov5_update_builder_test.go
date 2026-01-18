package query

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/core"
)

type cov5UpdateExecutor struct {
	compiled *core.CompiledQuery
	key      map[string]types.AttributeValue

	updateCalls       int
	updateResultCalls int
}

func (e *cov5UpdateExecutor) ExecuteQuery(_ *core.CompiledQuery, _ any) error { return nil }
func (e *cov5UpdateExecutor) ExecuteScan(_ *core.CompiledQuery, _ any) error  { return nil }

func (e *cov5UpdateExecutor) ExecuteUpdateItem(compiled *core.CompiledQuery, key map[string]types.AttributeValue) error {
	e.updateCalls++
	e.compiled = compiled
	e.key = key
	return nil
}

func (e *cov5UpdateExecutor) ExecuteUpdateItemWithResult(compiled *core.CompiledQuery, key map[string]types.AttributeValue) (*core.UpdateResult, error) {
	e.updateResultCalls++
	e.compiled = compiled
	e.key = key
	return &core.UpdateResult{
		Attributes: map[string]types.AttributeValue{
			"pk":     &types.AttributeValueMemberS{Value: "p1"},
			"status": &types.AttributeValueMemberS{Value: "ok"},
		},
	}, nil
}

func TestUpdateBuilder_Execute_BuildsKeyExpressionsAndConditions(t *testing.T) {
	exec := &cov5UpdateExecutor{}
	q := New(&struct{}{}, cov5Metadata{
		table:      "tbl",
		primaryKey: core.KeySchema{PartitionKey: "pk"},
	}, exec)

	q.Where("pk", "=", "p1")

	ubAny := NewUpdateBuilder(q)
	ub, ok := ubAny.(*UpdateBuilder)
	require.True(t, ok)

	ub.Set("status", "ok").
		SetIfNotExists("missing", nil, "default").
		Add("counter", 2).
		Increment("inc").
		Decrement("dec").
		Remove("old").
		Delete("tags", "a").
		AppendToList("list", []string{"x"}).
		PrependToList("list", []string{"y"}).
		RemoveFromListAt("list", 0).
		SetListElement("list", 1, "z").
		Condition("status", "=", "ok").
		OrCondition("status", "=", "other").
		ConditionExists("pk").
		ConditionNotExists("missing").
		ReturnValues("NONE")

	require.NoError(t, ub.Execute())
	require.Equal(t, 1, exec.updateCalls)
	require.NotNil(t, exec.compiled)
	require.NotEmpty(t, exec.compiled.UpdateExpression)
	require.NotNil(t, exec.key)
	require.Contains(t, exec.key, "pk")
}

func TestUpdateBuilder_ExecuteWithResult_UnmarshalsAttributes(t *testing.T) {
	exec := &cov5UpdateExecutor{}
	q := New(&struct{}{}, cov5Metadata{
		table:      "tbl",
		primaryKey: core.KeySchema{PartitionKey: "pk"},
		attributes: map[string]*core.AttributeMetadata{
			"pk": {Name: "PK", DynamoDBName: "pk"},
		},
	}, exec)
	q.Where("pk", "=", "p1")

	ubAny := NewUpdateBuilder(q)
	ub, ok := ubAny.(*UpdateBuilder)
	require.True(t, ok)

	type result struct {
		PK     string `theorydb:"pk"`
		Status string `theorydb:"status"`
	}

	var out result
	require.NoError(t, ub.Set("status", "ok").ExecuteWithResult(&out))
	require.Equal(t, 1, exec.updateResultCalls)
	require.Equal(t, "p1", out.PK)
	require.Equal(t, "ok", out.Status)
}

func TestUpdateBuilder_ExecuteWithResult_ValidatesPointer(t *testing.T) {
	exec := &cov5UpdateExecutor{}
	q := New(&struct{}{}, cov5Metadata{
		table:      "tbl",
		primaryKey: core.KeySchema{PartitionKey: "pk"},
	}, exec)
	q.Where("pk", "=", "p1")

	ubAny := NewUpdateBuilder(q)
	ub, ok := ubAny.(*UpdateBuilder)
	require.True(t, ok)

	require.Error(t, ub.ExecuteWithResult(nil))
	require.Error(t, ub.ExecuteWithResult(struct{}{}))
}

func TestUpdateBuilder_populateKeyValues_RejectsMissingOrNonEqualityKeys(t *testing.T) {
	exec := &cov5UpdateExecutor{}

	q := New(&struct{}{}, cov5Metadata{
		table:      "tbl",
		primaryKey: core.KeySchema{PartitionKey: "pk"},
	}, exec)
	ubAny := NewUpdateBuilder(q)
	ub, ok := ubAny.(*UpdateBuilder)
	require.True(t, ok)
	require.Error(t, ub.Execute())

	q = New(&struct{}{}, cov5Metadata{
		table:      "tbl",
		primaryKey: core.KeySchema{PartitionKey: "pk"},
	}, exec)
	q.Where("pk", "<", "p1")
	ubAny = NewUpdateBuilder(q)
	ub, ok = ubAny.(*UpdateBuilder)
	require.True(t, ok)
	require.Error(t, ub.Execute())
}
