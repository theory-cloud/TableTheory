package mocks

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/core"
)

func TestMustHelpers_NilReturnsNil(t *testing.T) {
	require.Nil(t, mustCoreQuery(nil))
	require.Nil(t, mustCoreDB(nil))
	require.Nil(t, mustPaginatedResult(nil))
	require.Nil(t, mustUpdateBuilder(nil))
}

func TestMustHelpers_SuccessAndPanicBranches(t *testing.T) {
	require.NotNil(t, mustCoreQuery(new(MockQuery)))
	require.NotNil(t, mustCoreDB(new(MockDB)))
	require.NotNil(t, mustPaginatedResult(&core.PaginatedResult{}))
	require.Equal(t, int64(42), mustInt64(int64(42)))
	require.NotNil(t, mustUpdateBuilder(new(MockUpdateBuilder)))

	require.Panics(t, func() { _ = mustCoreQuery("bad") })
	require.Panics(t, func() { _ = mustCoreDB("bad") })
	require.Panics(t, func() { _ = mustPaginatedResult("bad") })
	require.Panics(t, func() { _ = mustInt64("bad") })
	require.Panics(t, func() { _ = mustUpdateBuilder("bad") })
}

func TestMockQuery_BatchGetBuilder_Types(t *testing.T) {
	t.Run("returns nil for unexpected type", func(t *testing.T) {
		q := new(MockQuery)
		q.On("BatchGetBuilder").Return("bad").Once()
		require.Nil(t, q.BatchGetBuilder())
		q.AssertExpectations(t)
	})

	t.Run("returns builder when correct type", func(t *testing.T) {
		q := new(MockQuery)
		builder := new(MockBatchGetBuilder)
		q.On("BatchGetBuilder").Return(builder).Once()
		require.Equal(t, builder, q.BatchGetBuilder())
		q.AssertExpectations(t)
	})
}
