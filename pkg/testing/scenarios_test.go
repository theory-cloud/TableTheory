package testing_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/core"
	theorydbtesting "github.com/theory-cloud/tabletheory/pkg/testing"
)

func TestCommonScenarios_SetupTransactionScenario(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		testDB := theorydbtesting.NewTestDB()
		scenarios := theorydbtesting.NewCommonScenarios(testDB)

		scenarios.SetupTransactionScenario(true)

		called := false
		err := testDB.MockDB.Transaction(func(_ *core.Tx) error {
			called = true
			return nil
		})
		require.NoError(t, err)
		require.True(t, called)
	})

	t.Run("failure", func(t *testing.T) {
		testDB := theorydbtesting.NewTestDB()
		scenarios := theorydbtesting.NewCommonScenarios(testDB)

		scenarios.SetupTransactionScenario(false)

		err := testDB.MockDB.Transaction(func(_ *core.Tx) error { return nil })
		require.Error(t, err)
	})
}

func TestCommonScenarios_OtherSetups(t *testing.T) {
	type user struct {
		ID string
	}

	u := &user{ID: "u1"}

	t.Run("SetupCRUD", func(t *testing.T) {
		testDB := theorydbtesting.NewTestDB()
		scenarios := theorydbtesting.NewCommonScenarios(testDB)

		scenarios.SetupCRUD(u)

		q := testDB.MockDB.Model(u)
		require.NoError(t, q.Create())
		require.NoError(t, q.First(u))
		require.NoError(t, q.Update("Name"))
		require.NoError(t, q.Delete())
	})

	t.Run("SetupPagination", func(t *testing.T) {
		testDB := theorydbtesting.NewTestDB()
		scenarios := theorydbtesting.NewCommonScenarios(testDB)

		scenarios.SetupPagination(10)

		var got []user
		res, err := testDB.MockQuery.Limit(10).Cursor("c").AllPaginated(&got)
		require.NoError(t, err)
		require.True(t, res.HasMore)
		require.Equal(t, "next-cursor", res.NextCursor)
	})

	t.Run("SetupMultiTenant", func(t *testing.T) {
		testDB := theorydbtesting.NewTestDB()
		scenarios := theorydbtesting.NewCommonScenarios(testDB)

		scenarios.SetupMultiTenant("tenant1")

		_ = testDB.MockQuery.Where("tenant_id", "=", "tenant1")
		_ = testDB.MockQuery.Filter("tenant_id", "=", "tenant1")
	})

	t.Run("SetupBatchOperations", func(t *testing.T) {
		testDB := theorydbtesting.NewTestDB()
		scenarios := theorydbtesting.NewCommonScenarios(testDB)

		scenarios.SetupBatchOperations()

		require.NoError(t, testDB.MockQuery.BatchCreate([]user{{ID: "u1"}}))

		var got []user
		require.NoError(t, testDB.MockQuery.BatchGet([]any{"u1"}, &got))
		require.NoError(t, testDB.MockQuery.BatchDelete([]any{"u1"}))
		require.NoError(t, testDB.MockQuery.BatchWrite([]any{u}, []any{"u1"}))
	})

	t.Run("SetupComplexQuery", func(t *testing.T) {
		testDB := theorydbtesting.NewTestDB()
		scenarios := theorydbtesting.NewCommonScenarios(testDB)

		scenarios.SetupComplexQuery()

		_ = testDB.MockQuery.
			Where("id", "=", "u1").
			Filter("name", "=", "alice").
			OrderBy("id", "ASC").
			Limit(5).
			Index("by-id")
	})

	t.Run("SetupErrorScenarios", func(t *testing.T) {
		testDB := theorydbtesting.NewTestDB()
		scenarios := theorydbtesting.NewCommonScenarios(testDB)

		expectedErr := errors.New("boom")
		scenarios.SetupErrorScenarios(map[string]error{
			"create": expectedErr,
			"find":   expectedErr,
			"update": expectedErr,
			"delete": expectedErr,
			"all":    expectedErr,
			"count":  expectedErr,
		})

		require.ErrorIs(t, testDB.MockQuery.Create(), expectedErr)
		require.ErrorIs(t, testDB.MockQuery.First(u), expectedErr)
		require.ErrorIs(t, testDB.MockQuery.Update("Name"), expectedErr)
		require.ErrorIs(t, testDB.MockQuery.Delete(), expectedErr)

		var got []user
		require.ErrorIs(t, testDB.MockQuery.All(&got), expectedErr)
		_, err := testDB.MockQuery.Count()
		require.ErrorIs(t, err, expectedErr)
	})

	t.Run("SetupScanScenario", func(t *testing.T) {
		testDB := theorydbtesting.NewTestDB()
		scenarios := theorydbtesting.NewCommonScenarios(testDB)

		scenarios.SetupScanScenario()

		var got []user
		require.NoError(t, testDB.MockQuery.Scan(&got))

		q := testDB.MockQuery.ParallelScan(0, 2)
		require.NotNil(t, q)

		require.NoError(t, testDB.MockQuery.ScanAllSegments(&got, 2))
	})

	t.Run("SetupIndexQuery", func(t *testing.T) {
		testDB := theorydbtesting.NewTestDB()
		scenarios := theorydbtesting.NewCommonScenarios(testDB)

		scenarios.SetupIndexQuery("by-id")

		var got []user
		require.NoError(t, testDB.MockQuery.Index("by-id").Where("id", "=", "u1").All(&got))
	})
}

func TestCommonScenarios_SetupUpdateBuilder(t *testing.T) {
	testDB := theorydbtesting.NewTestDB()
	scenarios := theorydbtesting.NewCommonScenarios(testDB)
	scenarios.SetupUpdateBuilder()

	builder := testDB.MockQuery.UpdateBuilder()
	require.NotNil(t, builder)

	require.Same(t, builder, builder.Set("name", "alice"))
	require.Same(t, builder, builder.Add("count", 1))
	require.Same(t, builder, builder.Remove("deprecated"))
	require.NoError(t, builder.Execute())
}

func TestMockUpdateBuilder_ReturnTypesAndPanics(t *testing.T) {
	builder := new(theorydbtesting.MockUpdateBuilder)

	t.Run("returns self", func(t *testing.T) {
		builder.On("Set", "field", 1).Return(builder).Once()

		got := builder.Set("field", 1)
		require.Same(t, builder, got)

		builder.AssertExpectations(t)
	})

	t.Run("returns nil", func(t *testing.T) {
		builder.On("Remove", "field").Return(nil).Once()
		require.Nil(t, builder.Remove("field"))
		builder.AssertExpectations(t)
	})

	t.Run("panics on wrong type", func(t *testing.T) {
		builder.On("Add", "field", 1).Return("not a builder").Once()

		require.Panics(t, func() {
			_ = builder.Add("field", 1)
		})

		builder.AssertExpectations(t)
	})

	t.Run("propagates error", func(t *testing.T) {
		expectedErr := errors.New("boom")
		builder.On("ExecuteWithResult", mock.Anything).Return(expectedErr).Once()

		err := builder.ExecuteWithResult(&struct{}{})
		require.ErrorIs(t, err, expectedErr)

		builder.AssertExpectations(t)
	})

	t.Run("covers fluent methods", func(t *testing.T) {
		b := new(theorydbtesting.MockUpdateBuilder)

		b.On("SetIfNotExists", "field", 1, 0).Return(b).Once()
		b.On("Increment", "count").Return(b).Once()
		b.On("Decrement", "count").Return(b).Once()
		b.On("Delete", "setField", "v").Return(b).Once()
		b.On("AppendToList", "list", []string{"a"}).Return(b).Once()
		b.On("PrependToList", "list", []string{"a"}).Return(b).Once()
		b.On("RemoveFromListAt", "list", 2).Return(b).Once()
		b.On("SetListElement", "list", 0, "x").Return(b).Once()
		b.On("Condition", "field", "=", 1).Return(b).Once()
		b.On("OrCondition", "field", "=", 1).Return(b).Once()
		b.On("ConditionExists", "field").Return(b).Once()
		b.On("ConditionNotExists", "field").Return(b).Once()
		b.On("ConditionVersion", int64(2)).Return(b).Once()
		b.On("ReturnValues", "ALL_NEW").Return(b).Once()

		require.Same(t, b, b.SetIfNotExists("field", 1, 0))
		require.Same(t, b, b.Increment("count"))
		require.Same(t, b, b.Decrement("count"))
		require.Same(t, b, b.Delete("setField", "v"))
		require.Same(t, b, b.AppendToList("list", []string{"a"}))
		require.Same(t, b, b.PrependToList("list", []string{"a"}))
		require.Same(t, b, b.RemoveFromListAt("list", 2))
		require.Same(t, b, b.SetListElement("list", 0, "x"))
		require.Same(t, b, b.Condition("field", "=", 1))
		require.Same(t, b, b.OrCondition("field", "=", 1))
		require.Same(t, b, b.ConditionExists("field"))
		require.Same(t, b, b.ConditionNotExists("field"))
		require.Same(t, b, b.ConditionVersion(2))
		require.Same(t, b, b.ReturnValues("ALL_NEW"))

		b.AssertExpectations(t)
	})
}
