package query

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v4/pkg/core"
)

// contextCapturingScanExecutor records the context installed on it and reports a
// cancellation error from ExecuteScan when that context is already canceled,
// modeling a context-dependent scan.
type contextCapturingScanExecutor struct {
	ctx context.Context
}

func (e *contextCapturingScanExecutor) SetContext(ctx context.Context) {
	e.ctx = ctx
}

func (e *contextCapturingScanExecutor) ExecuteQuery(*core.CompiledQuery, any) error { return nil }

func (e *contextCapturingScanExecutor) ExecuteScan(*core.CompiledQuery, any) error {
	if e.ctx == nil {
		return nil
	}
	return e.ctx.Err()
}

// TestQueryTimeout_RepeatedReconfigurationKeepsBaseContextLive proves that a
// second QueryTimeout releases the previous derived context's timer without
// canceling the replacement, and that a subsequent ScanAllSegments runs on a
// live context.
func TestQueryTimeout_RepeatedReconfigurationKeepsBaseContextLive(t *testing.T) {
	exec := &contextCapturingScanExecutor{}
	q := New(&cov6BatchCreateItem{}, cov6Metadata{table: "tbl"}, exec)

	q.QueryTimeout(time.Hour)
	firstCtx := q.ctx
	require.NoError(t, firstCtx.Err())

	q.QueryTimeout(2 * time.Hour)

	require.ErrorIs(t, firstCtx.Err(), context.Canceled, "the previous timeout context is released")
	require.NoError(t, q.ctx.Err(), "the reconfigured context must stay live")

	deadline, ok := q.ctx.Deadline()
	require.True(t, ok)
	require.WithinDuration(t, time.Now().Add(2*time.Hour), deadline, time.Second)

	var dest []cov6BatchCreateItem
	require.NoError(t, q.ScanAllSegments(&dest, 2))
}

// TestQueryTimeout_SingleConfigurationIsLive proves the ordinary positive case:
// a single timeout configuration yields a context with the requested deadline.
func TestQueryTimeout_SingleConfigurationIsLive(t *testing.T) {
	q := New(&cov6BatchCreateItem{}, cov6Metadata{table: "tbl"}, &contextCapturingScanExecutor{})

	q.QueryTimeout(3 * time.Second)

	require.NoError(t, q.ctx.Err())
	deadline, ok := q.ctx.Deadline()
	require.True(t, ok)
	require.WithinDuration(t, time.Now().Add(3*time.Second), deadline, 100*time.Millisecond)
}
