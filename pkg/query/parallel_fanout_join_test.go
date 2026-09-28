package query

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v4/pkg/core"
)

// The pkg/query parallel fan-outs (segment scans, parallel batch get, parallel
// batch update) must join every worker before returning, on success and on every
// error path. The repository invariant is that no fan-out worker may outlive the
// call that started it: Lambda freezes the execution environment as soon as the
// handler returns, so a worker left running past its call would be frozen
// mid-flight and could resume against an invocation that is already over. These
// tests cover the success path, the first-segment-error path, the
// caller-cancellation path, and the parallel batch error paths.

// scanLeakGoroutineFrame matches the segment worker frame in a goroutine stack.
const scanLeakGoroutineFrame = "pkg/query.(*scanJoinExecutor).ExecuteScan"

var scanLeakGoroutineHeader = regexp.MustCompile(`(?m)^goroutine (\d+) \[`)

// scanLeakGoroutineStacksByID returns the stack of every live goroutine keyed by
// goroutine ID. Go goroutine IDs are never reused, so an ID absent before the
// call under test always identifies a goroutine created by it.
func scanLeakGoroutineStacksByID() map[int64]string {
	buf := make([]byte, 1<<20)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		buf = make([]byte, len(buf)*2)
	}

	matches := scanLeakGoroutineHeader.FindAllSubmatchIndex(buf, -1)
	stacks := make(map[int64]string, len(matches))
	for i, match := range matches {
		id, err := strconv.ParseInt(string(buf[match[2]:match[3]]), 10, 64)
		if err != nil {
			continue
		}
		end := len(buf)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		stacks[id] = string(buf[match[0]:end])
	}

	return stacks
}

// requireNoScanLeak fails when a segment worker is still running after
// ScanAllSegments returned, naming the abandoned worker stacks.
func requireNoScanLeak(t *testing.T, before map[int64]string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for {
		var leaked []string
		for id, stack := range scanLeakGoroutineStacksByID() {
			if _, existed := before[id]; existed {
				continue
			}
			if strings.Contains(stack, scanLeakGoroutineFrame) {
				leaked = append(leaked, stack)
			}
		}
		if len(leaked) == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("segment workers outlived ScanAllSegments:\n%s", strings.Join(leaked, "\n"))
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// requireNoFanoutWorkerRunning fails when any fan-out worker is still inside its
// executor after the fan-out returned. Workers report completion as they return,
// so a missing report means the call abandoned them.
func requireNoFanoutWorkerRunning(t *testing.T, finished <-chan int32, total int32) {
	t.Helper()

	deadline := time.Now().Add(100 * time.Millisecond)
	for remaining := total; remaining > 0; remaining-- {
		wait := time.Until(deadline)
		if wait <= 0 {
			t.Fatalf("fan-out returned with %d of %d workers still running", remaining, total)
		}
		select {
		case <-finished:
		case <-time.After(wait):
			t.Fatalf("fan-out returned with %d of %d workers still running", remaining, total)
		}
	}
}

// scanJoinExecutor is a segment executor whose workers only return when the
// fan-out cancels the shared segment context, when the test releases them, or
// after a configured tail delay. failSegment returns an error immediately, which
// makes the remaining workers observable: the early-return bug abandons them
// inside ExecuteScan, and the fixed behavior cancels the shared segment context
// and then joins them.
type scanJoinExecutor struct {
	ctx      context.Context
	started  chan int32
	finished chan int32
	release  chan struct{}

	mu        sync.Mutex
	seen      int
	slowDelay time.Duration

	failSegment   int32
	fastSegment   int32
	blockOnCancel bool
}

func (e *scanJoinExecutor) SetContext(ctx context.Context) {
	e.mu.Lock()
	e.ctx = ctx
	e.mu.Unlock()
}

func (e *scanJoinExecutor) context() context.Context {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.ctx == nil {
		return context.Background()
	}
	return e.ctx
}

func (e *scanJoinExecutor) scansStarted() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.seen
}

// ExecuteQuery satisfies QueryExecutor; these tests only exercise scans.
func (e *scanJoinExecutor) ExecuteQuery(_ *core.CompiledQuery, _ any) error {
	return errors.New("ExecuteQuery is not used by these tests")
}

func (e *scanJoinExecutor) ExecuteScan(input *core.CompiledQuery, dest any) error {
	segment := int32(-1)
	if input != nil && input.Segment != nil {
		segment = *input.Segment
	}

	e.mu.Lock()
	e.seen++
	e.mu.Unlock()

	if e.started != nil {
		e.started <- segment
	}
	if e.finished != nil {
		defer func() { e.finished <- segment }()
	}

	if segment == e.failSegment {
		return errors.New("segment scan failed")
	}

	if !e.blockOnCancel {
		return appendSegmentResult(dest, segment)
	}

	select {
	case <-e.context().Done():
		if segment != e.fastSegment && e.slowDelay > 0 {
			time.Sleep(e.slowDelay)
		}
		return e.context().Err()
	case <-e.release:
		return errors.New("segment worker released")
	}
}

func appendSegmentResult(dest any, segment int32) error {
	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr || destValue.Elem().Kind() != reflect.Slice {
		return errors.New("dest must be pointer to slice")
	}

	slice := destValue.Elem()
	elem := reflect.New(slice.Type().Elem()).Elem()
	if id := elem.FieldByName("ID"); id.IsValid() && id.CanSet() && id.Kind() == reflect.String {
		id.SetString(fmt.Sprintf("seg-%d", segment))
	}
	slice.Set(reflect.Append(slice, elem))
	return nil
}

func scanJoinMetadata() *cov4Metadata {
	return &cov4Metadata{
		table: "tbl",
		pk:    core.KeySchema{PartitionKey: "ID"},
		attrs: map[string]string{"ID": "id"},
	}
}

// TestScanAllSegments_SuccessPathLeavesNoSegmentWorkers covers the happy path:
// ScanAllSegments may only return once every segment worker has finished.
func TestScanAllSegments_SuccessPathLeavesNoSegmentWorkers(t *testing.T) {
	type scanItem struct {
		ID string
	}

	executor := &scanJoinExecutor{
		failSegment: -1,
		finished:    make(chan int32, 6),
	}
	q := New(&scanItem{}, scanJoinMetadata(), executor)

	var out []scanItem
	require.NoError(t, q.ScanAllSegments(&out, 3))
	require.Len(t, out, 3)

	requireNoFanoutWorkerRunning(t, executor.finished, 3)
}

// TestScanAllSegments_JoinsWorkersOnSegmentError is the error-path goroutine-leak
// check: a failing segment must cancel the remaining work and wait for it rather
// than return while the other segment workers are still inside ExecuteScan.
func TestScanAllSegments_JoinsWorkersOnSegmentError(t *testing.T) {
	type scanItem struct {
		ID string
	}

	executor := &scanJoinExecutor{
		failSegment:   0,
		blockOnCancel: true,
		release:       make(chan struct{}),
	}
	t.Cleanup(func() { close(executor.release) })
	q := New(&scanItem{}, scanJoinMetadata(), executor)

	before := scanLeakGoroutineStacksByID()

	var out []scanItem
	err := q.ScanAllSegments(&out, 3)
	require.EqualError(t, err, "segment scan failed", "ScanAllSegments must return the first segment error")

	requireNoScanLeak(t, before)
}

// TestScanAllSegments_JoinsWorkersOnContextCancel is the cancellation-path
// goroutine-leak check: canceling the caller context must not let
// ScanAllSegments return while a segment worker is still running. Segment 2
// unwinds as soon as the context is canceled; segments 0 and 1 keep working for
// a while afterwards, which is the window an unjoined implementation returns in.
func TestScanAllSegments_JoinsWorkersOnContextCancel(t *testing.T) {
	type scanItem struct {
		ID string
	}

	executor := &scanJoinExecutor{
		started:       make(chan int32, 3),
		finished:      make(chan int32, 6),
		failSegment:   -1,
		blockOnCancel: true,
		fastSegment:   2,
		slowDelay:     300 * time.Millisecond,
	}
	q := New(&scanItem{}, scanJoinMetadata(), executor)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q.ctx = ctx
	q.setExecutorContext(ctx)

	errCh := make(chan error, 1)
	var out []scanItem
	go func() { errCh <- q.ScanAllSegments(&out, 3) }()

	for i := 0; i < 3; i++ {
		select {
		case <-executor.started:
		case <-time.After(5 * time.Second):
			t.Fatal("segment workers did not start")
		}
	}

	cancel()
	require.Error(t, <-errCh)

	requireNoFanoutWorkerRunning(t, executor.finished, 3)
}

// TestScanAllSegments_RejectsNonSliceDestination keeps the validation path from
// launching any segment worker at all.
func TestScanAllSegments_RejectsNonSliceDestination(t *testing.T) {
	executor := &scanJoinExecutor{failSegment: -1}
	q := New(&struct{ ID string }{}, scanJoinMetadata(), executor)

	var notASlice struct{ ID string }
	require.Error(t, q.ScanAllSegments(&notASlice, 2))
	require.Zero(t, executor.scansStarted(), "no segment work may start for an invalid destination")
}

// TestScanAllSegments_WithoutContextStillJoins covers the default-context path: a
// Query that carries no context still joins every segment worker.
func TestScanAllSegments_WithoutContextStillJoins(t *testing.T) {
	type scanItem struct {
		ID string
	}

	executor := &scanJoinExecutor{failSegment: -1}
	q := New(&scanItem{}, scanJoinMetadata(), executor)
	q.ctx = nil

	var out []scanItem
	require.NoError(t, q.ScanAllSegments(&out, 2))
	require.Len(t, out, 2)
}

// batchGetJoinExecutor fails the first chunk it sees and holds every other chunk
// inside ExecuteBatchGet until the test releases it, which is the window an
// unjoined parallel batch get would return in.
type batchGetJoinExecutor struct {
	release  chan struct{}
	finished chan int32

	mu      sync.Mutex
	started int
}

func (e *batchGetJoinExecutor) ExecuteQuery(_ *core.CompiledQuery, _ any) error { return nil }
func (e *batchGetJoinExecutor) ExecuteScan(_ *core.CompiledQuery, _ any) error  { return nil }
func (e *batchGetJoinExecutor) ExecuteBatchWrite(_ *CompiledBatchWrite) error   { return nil }

func (e *batchGetJoinExecutor) ExecuteBatchGet(_ *CompiledBatchGet, _ *core.BatchGetOptions) ([]map[string]types.AttributeValue, error) {
	e.mu.Lock()
	e.started++
	first := e.started == 1
	e.mu.Unlock()

	if e.finished != nil {
		defer func() { e.finished <- 1 }()
	}
	if first {
		return nil, errors.New("chunk failed")
	}

	<-e.release
	return nil, nil
}

// TestBatchGetParallelJoinsWorkersOnChunkError locks in that the parallel batch
// get fan-out waits for every chunk worker before returning the first error, the
// same all-paths join ScanAllSegments guarantees.
func TestBatchGetParallelJoinsWorkersOnChunkError(t *testing.T) {
	executor := &batchGetJoinExecutor{
		release:  make(chan struct{}),
		finished: make(chan int32, 4),
	}
	q := New(&struct{ ID string }{}, scanJoinMetadata(), executor)

	// Release the held chunk after the fan-out has had time to observe the
	// failure; an unjoined implementation would return well before this fires.
	time.AfterFunc(150*time.Millisecond, func() { close(executor.release) })

	keys := []any{core.NewKeyPair("p1"), core.NewKeyPair("p2")}
	opts := core.DefaultBatchGetOptions()
	opts.ChunkSize = 1
	opts.Parallel = true
	opts.MaxConcurrency = 2

	var out []struct{ ID string }
	require.Error(t, q.BatchGetWithOptions(keys, &out, opts))

	requireNoFanoutWorkerRunning(t, executor.finished, 2)
}

// batchUpdateJoinExecutor fails the first batch it sees and holds every other
// batch inside ExecuteUpdateItem until the test releases it.
type batchUpdateJoinExecutor struct {
	release  chan struct{}
	finished chan int32

	mu      sync.Mutex
	started int
}

func (e *batchUpdateJoinExecutor) ExecuteQuery(_ *core.CompiledQuery, _ any) error { return nil }
func (e *batchUpdateJoinExecutor) ExecuteScan(_ *core.CompiledQuery, _ any) error  { return nil }

func (e *batchUpdateJoinExecutor) ExecuteUpdateItem(_ *core.CompiledQuery, _ map[string]types.AttributeValue) error {
	e.mu.Lock()
	e.started++
	first := e.started == 1
	e.mu.Unlock()

	if e.finished != nil {
		defer func() { e.finished <- 1 }()
	}
	if first {
		return errors.New("update failed")
	}

	<-e.release
	return nil
}

// TestBatchUpdateParallelJoinsWorkersOnBatchError locks in that the parallel
// batch update fan-out waits for every batch worker before returning the first
// error.
func TestBatchUpdateParallelJoinsWorkersOnBatchError(t *testing.T) {
	executor := &batchUpdateJoinExecutor{
		release:  make(chan struct{}),
		finished: make(chan int32, 4),
	}
	q := New(&batchUpdateRaceItem{}, cov5Metadata{
		table:      "tbl",
		primaryKey: core.KeySchema{PartitionKey: "pk"},
	}, executor)

	time.AfterFunc(150*time.Millisecond, func() { close(executor.release) })

	items := []any{
		batchUpdateRaceItem{PK: "p1", Status: "active"},
		batchUpdateRaceItem{PK: "p2", Status: "active"},
	}
	opts := &BatchUpdateOptions{
		MaxBatchSize:   1,
		Parallel:       true,
		MaxConcurrency: 2,
	}

	require.Error(t, q.BatchUpdateWithOptions(items, []string{"status"}, opts))

	requireNoFanoutWorkerRunning(t, executor.finished, 2)
}
