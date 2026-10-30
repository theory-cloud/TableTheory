package query

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v6/pkg/core"
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

// TestQueryTimeout_ThenWithCancellationPreservesDeadlineAndCancel covers the
// other ordinary order. The returned canceler must cancel the query's current
// context AND the deadline configured before WithCancellation must survive: the
// composition regression replaced the effective context with a cancel-only layer,
// silently dropping the timeout so the query could run unbounded.
func TestQueryTimeout_ThenWithCancellationPreservesDeadlineAndCancel(t *testing.T) {
	q := New(&cov6BatchCreateItem{}, cov6Metadata{table: "tbl"}, &contextCapturingScanExecutor{})

	q.QueryTimeout(time.Hour)
	_, canceler := q.WithCancellation()

	require.NoError(t, q.ctx.Err())
	deadline, ok := q.ctx.Deadline()
	require.True(t, ok, "WithCancellation must not discard the configured deadline")
	require.WithinDuration(t, time.Now().Add(time.Hour), deadline, time.Second)

	canceler.Cancel()
	require.ErrorIs(t, q.ctx.Err(), context.Canceled)
}

// TestQueryTimeout_ThenWithCancellationDoesNotExtendDeadline proves the retained
// deadline is the original instant: WithCancellation must not restart the clock
// and silently buy the query more time than QueryTimeout configured.
func TestQueryTimeout_ThenWithCancellationDoesNotExtendDeadline(t *testing.T) {
	q := New(&cov6BatchCreateItem{}, cov6Metadata{table: "tbl"}, &contextCapturingScanExecutor{})

	q.QueryTimeout(200 * time.Millisecond)
	before, ok := q.ctx.Deadline()
	require.True(t, ok)

	time.Sleep(30 * time.Millisecond)
	_, canceler := q.WithCancellation()
	defer canceler.Cancel()

	after, ok := q.ctx.Deadline()
	require.True(t, ok)
	require.WithinDuration(t, before, after, time.Millisecond,
		"WithCancellation must preserve the original deadline instant")
}

// TestQueryTimeout_ThenWithCancellation_DeadlineStillExpires is the negative
// control: after WithCancellation the retained deadline must still fire on its
// own, not only when Cancel is called.
func TestQueryTimeout_ThenWithCancellation_DeadlineStillExpires(t *testing.T) {
	q := New(&cov6BatchCreateItem{}, cov6Metadata{table: "tbl"}, &contextCapturingScanExecutor{})

	q.QueryTimeout(20 * time.Millisecond)
	_, canceler := q.WithCancellation()
	defer canceler.Cancel()

	select {
	case <-q.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("the configured deadline did not expire after WithCancellation")
	}
	require.ErrorIs(t, q.ctx.Err(), context.DeadlineExceeded)
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

// TestNewWithConditions_ConstructorDeadlineSurvivesQueryTimeout proves the
// constructor caller's context is a retained root rather than a value that only
// lives on q.ctx: a later QueryTimeout must compose with the constructor
// deadline instead of replacing it with a fresh one derived from Background.
func TestNewWithConditions_ConstructorDeadlineSurvivesQueryTimeout(t *testing.T) {
	constructorCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	q := NewWithConditions(
		&cov6BatchCreateItem{},
		cov6Metadata{table: "tbl"},
		&contextCapturingScanExecutor{},
		nil,
		constructorCtx,
	)
	q.QueryTimeout(time.Hour)

	require.NoError(t, q.ctx.Err())
	deadline, ok := q.ctx.Deadline()
	require.True(t, ok)
	require.WithinDuration(t, time.Now().Add(50*time.Millisecond), deadline, 100*time.Millisecond,
		"QueryTimeout must not discard the constructor caller's deadline")

	<-q.ctx.Done()
	require.ErrorIs(t, q.ctx.Err(), context.DeadlineExceeded)
}

// TestNewWithConditions_ConstructorCancellationSurvivesDerivations proves the
// constructor caller's cancellation still reaches the live context after both
// WithCancellation and QueryTimeout, in either order.
func TestNewWithConditions_ConstructorCancellationSurvivesDerivations(t *testing.T) {
	for _, order := range []string{"cancel-then-timeout", "timeout-after-cancellation"} {
		t.Run(order, func(t *testing.T) {
			constructorCtx, cancel := context.WithCancel(context.Background())

			q := NewWithConditions(
				&cov6BatchCreateItem{},
				cov6Metadata{table: "tbl"},
				&contextCapturingScanExecutor{},
				nil,
				constructorCtx,
			)
			if order == "cancel-then-timeout" {
				_, queryCanceler := q.WithCancellation()
				defer queryCanceler.Cancel()
				q.QueryTimeout(time.Hour)
			} else {
				q.QueryTimeout(time.Hour)
				_, queryCanceler := q.WithCancellation()
				defer queryCanceler.Cancel()
			}

			require.NoError(t, q.ctx.Err())
			cancel()
			require.ErrorIs(t, q.ctx.Err(), context.Canceled,
				"the constructor caller's cancellation must reach the live context")
		})
	}
}

// recordingScanExecutor records the context result observed by every segment
// worker and returns it, so a test can prove caller cancellation propagated to
// the scan workers rather than being replaced by a background root.
type recordingScanExecutor struct {
	ctx   context.Context
	mu    sync.Mutex
	seen  []error
	calls int
}

func (e *recordingScanExecutor) SetContext(ctx context.Context) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.ctx = ctx
}

func (e *recordingScanExecutor) ExecuteQuery(*core.CompiledQuery, any) error { return nil }

func (e *recordingScanExecutor) ExecuteScan(*core.CompiledQuery, any) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls++
	err := e.ctx.Err()
	e.seen = append(e.seen, err)
	return err
}

func (e *recordingScanExecutor) observed() (int, []error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.calls, append([]error(nil), e.seen...)
}

// TestNewWithConditions_ConstructorCancellationReachesEverySegment is the
// negative control for the constructor fix: canceling the caller context before
// a parallel scan must be visible to the segment workers, never replaced by a
// background root. The scan joins every worker before returning.
func TestNewWithConditions_ConstructorCancellationReachesEverySegment(t *testing.T) {
	constructorCtx, cancel := context.WithCancel(context.Background())
	exec := &recordingScanExecutor{}

	q := NewWithConditions(&cov6BatchCreateItem{}, cov6Metadata{table: "tbl"}, exec, nil, constructorCtx)
	q.QueryTimeout(time.Hour)
	cancel()

	var dest []cov6BatchCreateItem
	err := q.ScanAllSegments(&dest, 3)
	require.ErrorIs(t, err, context.Canceled,
		"the scan must observe the constructor caller's cancellation")

	calls, seen := exec.observed()
	require.Positive(t, calls, "at least one segment worker must have run")
	for i, observed := range seen {
		require.ErrorIs(t, observed, context.Canceled,
			"segment worker %d did not observe the constructor cancellation", i)
	}
}

// TestNewWithConditions_OrdinaryScanStillSucceeds is the positive control: a
// live constructor context keeps ordinary non-cancellation behavior unchanged.
func TestNewWithConditions_OrdinaryScanStillSucceeds(t *testing.T) {
	exec := &recordingScanExecutor{}
	q := NewWithConditions(
		&cov6BatchCreateItem{},
		cov6Metadata{table: "tbl"},
		exec,
		nil,
		context.Background(),
	)
	q.QueryTimeout(time.Hour)

	var dest []cov6BatchCreateItem
	require.NoError(t, q.ScanAllSegments(&dest, 2))

	calls, seen := exec.observed()
	require.Positive(t, calls)
	for i, observed := range seen {
		require.NoError(t, observed, "segment worker %d must not see a cancellation", i)
	}
}

// TestApplyContext_FallsBackToExistingContextWhenRootsAreNil pins the last
// non-nil fallback: when neither retained root is set, the existing q.ctx must
// be kept instead of silently replacing it with Background.
func TestApplyContext_FallsBackToExistingContextWhenRootsAreNil(t *testing.T) {
	type key struct{}
	existing := context.WithValue(context.Background(), key{}, "caller")

	q := &Query{ctx: existing}
	q.applyContext()

	require.True(t, existing == q.ctx, "an existing q.ctx must survive when both retained roots are nil")
}
