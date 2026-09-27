package protection

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// timeoutBody blocks inside Read until Close is called. It records entry,
// closure, and exit so a test can tell whether SecureBodyReader abandoned the
// reader goroutine on the timeout path. A non-nil closeErr is reported by Close.
type timeoutBody struct {
	entered  chan struct{}
	closed   chan struct{}
	exited   chan struct{}
	closeErr error

	enterOnce sync.Once
	closeOnce sync.Once
	exitOnce  sync.Once
}

func newTimeoutBody() *timeoutBody {
	return &timeoutBody{
		entered: make(chan struct{}),
		closed:  make(chan struct{}),
		exited:  make(chan struct{}),
	}
}

func (b *timeoutBody) Read(_ []byte) (int, error) {
	b.enterOnce.Do(func() { close(b.entered) })
	<-b.closed
	b.exitOnce.Do(func() { close(b.exited) })
	return 0, io.EOF
}

func (b *timeoutBody) Close() error {
	b.closeOnce.Do(func() { close(b.closed) })
	return b.closeErr
}

// TestSecureBodyReaderTimeoutJoinsReaderGoroutine covers the timeout path of the
// request-body guard: it must close the body and wait for the reader goroutine
// before returning. Lambda freezes the execution environment as soon as the
// handler returns, so a reader left mid-read would be frozen and could resume
// against an invocation that is already over.
func TestSecureBodyReaderTimeoutJoinsReaderGoroutine(t *testing.T) {
	protector := NewResourceProtector(ResourceLimits{
		MaxRequestBodySize: 1024,
		MaxRequestTimeout:  20 * time.Millisecond,
		MaxConcurrentReq:   4,
		RequestsPerSecond:  1000,
		BurstSize:          10,
	})

	body := newTimeoutBody()
	t.Cleanup(func() { require.NoError(t, body.Close()) })

	req := (&http.Request{Body: body}).WithContext(context.Background())

	_, err := protector.SecureBodyReader(req)
	require.Error(t, err)
	require.True(t, IsResourceProtectionError(err))
	require.Equal(t, "RequestTimeout", GetResourceProtectionType(err))

	select {
	case <-body.entered:
	case <-time.After(time.Second):
		t.Fatal("test body never entered Read")
	}

	// The timeout path must close the body so the reader unblocks...
	select {
	case <-body.closed:
	default:
		t.Fatal("SecureBodyReader returned a timeout without closing the request body; the reader goroutine is abandoned mid-read")
	}

	// ...and it must not return until that reader has finished.
	select {
	case <-body.exited:
	case <-time.After(2 * time.Second):
		t.Fatal("SecureBodyReader returned while the body reader was still running")
	}
}

// TestSecureBodyReaderTimeoutReportsCloseFailure covers the close-error branch of
// the timeout path: the body is still unblocked and joined, and the close failure
// is reported rather than silently dropped.
func TestSecureBodyReaderTimeoutReportsCloseFailure(t *testing.T) {
	protector := NewResourceProtector(ResourceLimits{
		MaxRequestBodySize: 1024,
		MaxRequestTimeout:  20 * time.Millisecond,
		MaxConcurrentReq:   4,
		RequestsPerSecond:  1000,
		BurstSize:          10,
	})

	body := newTimeoutBody()
	body.closeErr = errors.New("body already gone")
	defer func() { require.Error(t, body.Close()) }()

	req := (&http.Request{Body: body}).WithContext(context.Background())

	_, err := protector.SecureBodyReader(req)
	require.Error(t, err)
	require.True(t, IsResourceProtectionError(err))
	require.Equal(t, "RequestTimeout", GetResourceProtectionType(err))
	require.Contains(t, err.Error(), "body already gone")

	select {
	case <-body.exited:
	default:
		t.Fatal("the reader must have been joined even when closing the body failed")
	}
}
