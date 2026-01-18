package mocks

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/core"
)

func TestMockBatchGetBuilder_FluentMethods(t *testing.T) {
	builder := new(MockBatchGetBuilder)

	keys := []any{"k1"}
	policy := &core.RetryPolicy{MaxRetries: 1}

	builder.On("Keys", keys).Return(builder).Once()
	builder.On("ChunkSize", 2).Return(builder).Once()
	builder.On("ConsistentRead").Return(builder).Once()
	builder.On("Parallel", 3).Return(builder).Once()
	builder.On("WithRetry", policy).Return(builder).Once()
	builder.On("Select", []string{"a", "b"}).Return(builder).Once()
	builder.On("OnProgress", mock.Anything).Return(builder).Once()
	builder.On("OnError", mock.Anything).Return(builder).Once()
	builder.On("Execute", mock.Anything).Return(nil).Once()

	require.Same(t, builder, builder.Keys(keys))
	require.Same(t, builder, builder.ChunkSize(2))
	require.Same(t, builder, builder.ConsistentRead())
	require.Same(t, builder, builder.Parallel(3))
	require.Same(t, builder, builder.WithRetry(policy))
	require.Same(t, builder, builder.Select("a", "b"))
	require.Same(t, builder, builder.OnProgress(func(int, int) {}))
	require.Same(t, builder, builder.OnError(func([]any, error) error { return nil }))
	require.NoError(t, builder.Execute(&[]any{}))

	builder.AssertExpectations(t)
}

func TestMockUpdateBuilder_FluentMethods(t *testing.T) {
	builder := new(MockUpdateBuilder)

	builder.On("SetIfNotExists", "field", 1, 0).Return(builder).Once()
	builder.On("Add", "count", 1).Return(builder).Once()
	builder.On("Increment", "count").Return(builder).Once()
	builder.On("Decrement", "count").Return(builder).Once()
	builder.On("Remove", "deprecated").Return(builder).Once()
	builder.On("Delete", "setField", "v").Return(builder).Once()
	builder.On("AppendToList", "list", []string{"a"}).Return(builder).Once()
	builder.On("PrependToList", "list", []string{"a"}).Return(builder).Once()
	builder.On("RemoveFromListAt", "list", 2).Return(builder).Once()
	builder.On("SetListElement", "list", 0, "x").Return(builder).Once()
	builder.On("Condition", "field", "=", 1).Return().Once()
	builder.On("OrCondition", "field", "=", 1).Return().Once()
	builder.On("ConditionExists", "field").Return(builder).Once()
	builder.On("ConditionNotExists", "field").Return(builder).Once()
	builder.On("ConditionVersion", int64(2)).Return(builder).Once()
	builder.On("ReturnValues", "ALL_NEW").Return(builder).Once()
	builder.On("ExecuteWithResult", mock.Anything).Return(nil).Once()

	require.Same(t, builder, builder.SetIfNotExists("field", 1, 0))
	require.Same(t, builder, builder.Add("count", 1))
	require.Same(t, builder, builder.Increment("count"))
	require.Same(t, builder, builder.Decrement("count"))
	require.Same(t, builder, builder.Remove("deprecated"))
	require.Same(t, builder, builder.Delete("setField", "v"))
	require.Same(t, builder, builder.AppendToList("list", []string{"a"}))
	require.Same(t, builder, builder.PrependToList("list", []string{"a"}))
	require.Same(t, builder, builder.RemoveFromListAt("list", 2))
	require.Same(t, builder, builder.SetListElement("list", 0, "x"))
	require.Same(t, builder, builder.Condition("field", "=", 1))
	require.Same(t, builder, builder.OrCondition("field", "=", 1))
	require.Same(t, builder, builder.ConditionExists("field"))
	require.Same(t, builder, builder.ConditionNotExists("field"))
	require.Same(t, builder, builder.ConditionVersion(2))
	require.Same(t, builder, builder.ReturnValues("ALL_NEW"))
	require.NoError(t, builder.ExecuteWithResult(&struct{}{}))

	builder.AssertExpectations(t)
}

func TestMockDB_MigrateAndAutoMigrate(t *testing.T) {
	db := new(MockDB)

	db.On("Migrate").Return(nil).Once()
	db.On("AutoMigrate", mock.Anything).Return(nil).Once()

	require.NoError(t, db.Migrate())
	require.NoError(t, db.AutoMigrate(&struct{}{}))

	db.AssertExpectations(t)
}
