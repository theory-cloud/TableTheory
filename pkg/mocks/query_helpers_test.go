package mocks

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v3/pkg/core"
)

type recordingTestingT struct {
	errors []string
	logs   []string
	failed bool
}

func (r *recordingTestingT) Errorf(format string, args ...interface{}) {
	r.errors = append(r.errors, fmt.Sprintf(format, args...))
}

func (r *recordingTestingT) Logf(format string, args ...interface{}) {
	r.logs = append(r.logs, fmt.Sprintf(format, args...))
}

func (r *recordingTestingT) FailNow() {
	r.failed = true
}

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

	require.NotPanics(t, func() { require.Nil(t, mustCoreQuery("bad")) })
	require.NotPanics(t, func() { require.Nil(t, mustCoreDB("bad")) })
	require.NotPanics(t, func() { require.Nil(t, mustPaginatedResult("bad")) })
	require.NotPanics(t, func() { require.Zero(t, mustInt64("bad")) })
	require.NotPanics(t, func() { require.Nil(t, mustUpdateBuilder("bad")) })
}

func TestMockQuery_BatchGetBuilder_Types(t *testing.T) {
	t.Run("returns nil for unexpected type", func(t *testing.T) {
		q := new(MockQuery)
		q.On("BatchGetBuilder").Return("bad").Once()
		require.Nil(t, q.BatchGetBuilder())

		recorder := &recordingTestingT{}
		require.False(t, q.AssertExpectations(recorder))
		require.Contains(t, strings.Join(recorder.errors, "\n"), "expected core.BatchGetBuilder")
	})

	t.Run("returns builder when correct type", func(t *testing.T) {
		q := new(MockQuery)
		builder := new(MockBatchGetBuilder)
		q.On("BatchGetBuilder").Return(builder).Once()
		require.Equal(t, builder, q.BatchGetBuilder())
		q.AssertExpectations(t)
	})
}

func TestMockQuery_WrongReturnTypeRecordsAssertionFailure(t *testing.T) {
	q := new(MockQuery)
	q.On("Where", "ID", "=", "123").Return("bad-query").Once()

	require.NotPanics(t, func() {
		require.Nil(t, q.Where("ID", "=", "123"))
	})

	recorder := &recordingTestingT{}
	require.False(t, q.AssertExpectations(recorder))
	require.Contains(t, strings.Join(recorder.errors, "\n"), "Where")
	require.Contains(t, strings.Join(recorder.errors, "\n"), "expected core.Query")
	require.False(t, recorder.failed)
}
