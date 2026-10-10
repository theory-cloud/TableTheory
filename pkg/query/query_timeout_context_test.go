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

// TestWithCancellation_ThenQueryTimeoutStillCancellable pins the regression where
// a timeout configured after WithCancellation used to be re-derived from the root
// base and orphan the returned canceler, so Cancel no longer stopped the query.
func TestWithCancellation_ThenQueryTimeoutStillCancellable(t *testing.T) {
	q := New(&cov6BatchCreateItem{}, cov6Metadata{table: "tbl"}, &contextCapturingScanExecutor{})

	_, canceler := q.WithCancellation()
	q.QueryTimeout(time.Hour)

	require.NoError(t, q.ctx.Err(), "the timeout configured after WithCancellation must stay live")
	deadline, ok := q.ctx.Deadline()
	require.True(t, ok)
	require.WithinDuration(t, time.Now().Add(time.Hour), deadline, time.Second)

	canceler.Cancel()
	require.ErrorIs(t, q.ctx.Err(), context.Canceled,
		"Cancel must reach the live context even after a later QueryTimeout")
}

// TestQueryTimeout_ThenWithCancellationIsCancellable covers the other ordinary
// order: the returned canceler cancels the query's current context.
func TestQueryTimeout_ThenWithCancellationIsCancellable(t *testing.T) {
	q := New(&cov6BatchCreateItem{}, cov6Metadata{table: "tbl"}, &contextCapturingScanExecutor{})

	q.QueryTimeout(time.Hour)
	_, canceler := q.WithCancellation()

	require.NoError(t, q.ctx.Err())
	canceler.Cancel()
	require.ErrorIs(t, q.ctx.Err(), context.Canceled)
}

// TestQueryTimeoutAfterCancellation_RepeatedTimeoutStaysLive proves the two
// guarantees hold together: replacing a timeout after WithCancellation releases
// the old timer without poisoning the replacement, and the original canceler
// still reaches the newest context.
func TestQueryTimeoutAfterCancellation_RepeatedTimeoutStaysLive(t *testing.T) {
	q := New(&cov6BatchCreateItem{}, cov6Metadata{table: "tbl"}, &contextCapturingScanExecutor{})

	_, canceler := q.WithCancellation()
	q.QueryTimeout(time.Hour)
	first := q.ctx
	require.NoError(t, first.Err())

	q.QueryTimeout(2 * time.Hour)

	require.ErrorIs(t, first.Err(), context.Canceled, "the previous timeout context is released")
	require.NoError(t, q.ctx.Err(), "the replacement timeout must stay live")
	deadline, ok := q.ctx.Deadline()
	require.True(t, ok)
	require.WithinDuration(t, time.Now().Add(2*time.Hour), deadline, time.Second)

	canceler.Cancel()
	require.ErrorIs(t, q.ctx.Err(), context.Canceled,
		"Cancel must still reach the newest timeout context")
}

// TestWithContext_ReplacesCancellationLayer proves an explicit context clears the
// cancellation layer rather than leaving QueryTimeout deriving from a stale one.
func TestWithContext_ReplacesCancellationLayer(t *testing.T) {
	q := New(&cov6BatchCreateItem{}, cov6Metadata{table: "tbl"}, &contextCapturingScanExecutor{})

	_, canceler := q.WithCancellation()
	q.WithContext(context.Background())
	q.QueryTimeout(time.Hour)

	require.NoError(t, q.ctx.Err())
	canceler.Cancel()
	require.NoError(t, q.ctx.Err(),
		"a canceler from a replaced cancellation layer must not affect the live context")
}
