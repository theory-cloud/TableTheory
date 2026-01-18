package testing_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/core"
	theorydberrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"github.com/theory-cloud/tabletheory/pkg/session"
	theorydbtesting "github.com/theory-cloud/tabletheory/pkg/testing"
)

func TestNewTestDB_CommonSetup(t *testing.T) {
	testDB := theorydbtesting.NewTestDB()
	require.NotNil(t, testDB)
	require.NotNil(t, testDB.MockDB)
	require.NotNil(t, testDB.MockQuery)

	q := testDB.MockDB.Model(struct{}{})
	require.IsType(t, (*mocks.MockQuery)(nil), q)
	mockQuery, ok := q.(*mocks.MockQuery)
	require.True(t, ok)
	require.Same(t, testDB.MockQuery, mockQuery)

	dbWithCtx := testDB.MockDB.WithContext(context.Background())
	require.IsType(t, (*mocks.MockDB)(nil), dbWithCtx)
	mockDB, ok := dbWithCtx.(*mocks.MockDB)
	require.True(t, ok)
	require.Same(t, testDB.MockDB, mockDB)
}

func TestTestDB_ExpectFindCopiesValue(t *testing.T) {
	type user struct {
		ID string
	}

	testDB := theorydbtesting.NewTestDB()

	expected := user{ID: "u1"}
	testDB.ExpectFind(&expected)

	var got user
	err := testDB.MockQuery.First(&got)
	require.NoError(t, err)
	require.Equal(t, expected, got)

	testDB.AssertExpectations(t)
}

func TestTestDB_ExpectAllCopiesValues(t *testing.T) {
	testDB := theorydbtesting.NewTestDB()

	expected := []int{1, 2, 3}
	testDB.ExpectAll(&expected)

	var got []int
	err := testDB.MockQuery.All(&got)
	require.NoError(t, err)
	require.Equal(t, expected, got)

	testDB.AssertExpectations(t)
}

func TestQueryChain_ExpectFirst(t *testing.T) {
	type user struct {
		ID string
	}

	testDB := theorydbtesting.NewTestDB()

	expected := user{ID: "u1"}
	testDB.NewQueryChain().
		Where("id", "=", "u1").
		Limit(10).
		OrderBy("id", "ASC").
		ExpectFirst(&expected)

	var got user
	err := testDB.MockQuery.
		Where("id", "=", "u1").
		Limit(10).
		OrderBy("id", "ASC").
		First(&got)
	require.NoError(t, err)
	require.Equal(t, expected, got)

	testDB.AssertExpectations(t)
}

func TestTestDB_ExpectationHelpers(t *testing.T) {
	type user struct {
		ID string
	}

	u := &user{ID: "u1"}
	ctx := context.Background()

	testDB := theorydbtesting.NewTestDB()
	testDB.Reset()

	t.Run("create and create error", func(t *testing.T) {
		testDB.ExpectModel(u).ExpectCreate()
		require.NoError(t, testDB.MockDB.Model(u).Create())

		expectedErr := errors.New("boom")
		testDB.ExpectModel(u).ExpectCreateError(expectedErr)
		require.ErrorIs(t, testDB.MockDB.Model(u).Create(), expectedErr)
	})

	t.Run("where, first, and not found", func(t *testing.T) {
		testDB.ExpectModel(u).
			ExpectWhere("id", "=", "u1").
			ExpectFind(u)

		var got user
		require.NoError(t, testDB.MockDB.Model(u).Where("id", "=", "u1").First(&got))
		require.Equal(t, *u, got)

		testDB.ExpectModel(u).ExpectNotFound()
		require.ErrorIs(t, testDB.MockDB.Model(u).First(&got), theorydberrors.ErrItemNotFound)
	})

	t.Run("update and delete", func(t *testing.T) {
		testDB.ExpectModel(u).ExpectUpdate("Name")
		require.NoError(t, testDB.MockDB.Model(u).Update("Name"))

		expectedErr := errors.New("update failed")
		testDB.ExpectModel(u).ExpectUpdateError(expectedErr, "Name")
		require.ErrorIs(t, testDB.MockDB.Model(u).Update("Name"), expectedErr)

		testDB.ExpectModel(u).ExpectDelete()
		require.NoError(t, testDB.MockDB.Model(u).Delete())

		deleteErr := errors.New("delete failed")
		testDB.ExpectModel(u).ExpectDeleteError(deleteErr)
		require.ErrorIs(t, testDB.MockDB.Model(u).Delete(), deleteErr)
	})

	t.Run("count and query modifiers", func(t *testing.T) {
		testDB.ExpectModel(u).ExpectCount(3)
		got, err := testDB.MockDB.Model(u).Count()
		require.NoError(t, err)
		require.Equal(t, int64(3), got)

		testDB.ExpectModel(u).
			ExpectLimit(10).
			ExpectOffset(20).
			ExpectOrderBy("id", "ASC").
			ExpectIndex("by-id")

		_ = testDB.MockDB.Model(u).
			Limit(10).
			Offset(20).
			OrderBy("id", "ASC").
			Index("by-id")
	})

	t.Run("transaction helpers", func(t *testing.T) {
		testDB.ExpectTransaction(func(_ *core.Tx) {})
		require.NoError(t, testDB.MockDB.Transaction(func(_ *core.Tx) error { return nil }))

		expectedErr := errors.New("tx failed")
		testDB.ExpectTransactionError(expectedErr)
		require.ErrorIs(t, testDB.MockDB.Transaction(func(_ *core.Tx) error { return nil }), expectedErr)
	})

	t.Run("batch helpers", func(t *testing.T) {
		keys := []interface{}{"u1", "u2"}
		expected := []user{{ID: "u1"}, {ID: "u2"}}

		testDB.ExpectModel(u).ExpectBatchGet(keys, &expected)

		var got []user
		require.NoError(t, testDB.MockDB.Model(u).BatchGet(keys, &got))
		require.Equal(t, expected, got)

		testDB.ExpectModel(u).ExpectBatchCreate([]user{{ID: "u3"}})
		require.NoError(t, testDB.MockDB.Model(u).BatchCreate([]user{{ID: "u3"}}))

		testDB.ExpectModel(u).ExpectBatchDelete(keys)
		require.NoError(t, testDB.MockDB.Model(u).BatchDelete(keys))
	})

	t.Run("with context passthrough", func(t *testing.T) {
		testDB.MockDB.On("WithContext", ctx).Return(testDB.MockDB).Once()
		dbWithCtx := testDB.MockDB.WithContext(ctx)
		require.Equal(t, testDB.MockDB, dbWithCtx)
	})

	testDB.AssertExpectations(t)

	// Ensure Reset clears expectations and doesn't panic.
	testDB.Reset()
	testDB.MockDB.ExpectedCalls = nil
	testDB.MockQuery.ExpectedCalls = nil

	// Touch DefaultDBFactory.CreateDB for coverage; it's a placeholder.
	var factory theorydbtesting.DefaultDBFactory
	db, err := factory.CreateDB(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	require.Nil(t, db)
}

func TestQueryChain_ExpectAll(t *testing.T) {
	testDB := theorydbtesting.NewTestDB()

	expected := []int{1, 2, 3}
	testDB.NewQueryChain().
		Where("id", "=", "u1").
		Limit(10).
		OrderBy("id", "ASC").
		ExpectAll(&expected)

	var got []int
	err := testDB.MockQuery.
		Where("id", "=", "u1").
		Limit(10).
		OrderBy("id", "ASC").
		All(&got)
	require.NoError(t, err)
	require.Equal(t, expected, got)

	testDB.AssertExpectations(t)
}
