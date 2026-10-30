package protection

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// readCountingBody records how many times Read is entered. It lets the
// fail-fast test prove that an unsupported transport is rejected before any
// request body work starts.
type readCountingBody struct {
	reads int
	mu    sync.Mutex
}

func (b *readCountingBody) Read(p []byte) (int, error) {
	b.mu.Lock()
	b.reads++
	b.mu.Unlock()
	return copy(p, "body"), io.EOF
}

func (b *readCountingBody) Close() error { return nil }

func (b *readCountingBody) readCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.reads
}

// TestSecureBodyReaderWithDeadlineBoundsStalledHTTP1Cleanup proves the
// transport-deadline shape: when the caller supplies the response writer, the
// helper installs a read deadline on the connection before reading, so a client
// that declares a body and withholds it cannot keep the call or its request
// slot past MaxRequestTimeout. The stalled connection stays open the whole
// time, so the recovery is attributable to the enforced deadline rather than to
// a client disconnect.
func TestSecureBodyReaderWithDeadlineBoundsStalledHTTP1Cleanup(t *testing.T) {
	const requestTimeout = 80 * time.Millisecond

	protector := NewResourceProtector(ResourceLimits{
		MaxRequestBodySize: 1 << 20,
		MaxRequestTimeout:  requestTimeout,
		MaxConcurrentReq:   1,
		RequestsPerSecond:  1000,
		BurstSize:          50,
	})

	handlerStarted := make(chan struct{})
	var handlerStartOnce sync.Once
	handlerErrs := make(chan error, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerStartOnce.Do(func() { close(handlerStarted) })
		if _, err := protector.SecureBodyReaderWithDeadline(w, r); err != nil {
			handlerErrs <- err
			http.Error(w, err.Error(), http.StatusRequestTimeout)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	addr := strings.TrimPrefix(srv.URL, "http://")
	conn, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", addr)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() }) //nolint:errcheck // best-effort test cleanup

	_, err = io.WriteString(conn,
		"POST /stalled HTTP/1.1\r\nHost: "+addr+"\r\nContent-Length: 10\r\nConnection: close\r\n\r\n")
	require.NoError(t, err)

	select {
	case <-handlerStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("server handler never started")
	}
	require.Eventually(t, func() bool {
		return len(protector.requestSemaphore) == 1
	}, requestTimeout+2*time.Second, 2*time.Millisecond,
		"stalled HTTP/1 request never acquired the request slot")

	// The client socket is deliberately left open: the enforced transport
	// deadline, not a disconnect, must end the read and the cleanup.
	select {
	case err := <-handlerErrs:
		require.Error(t, err)
		require.Equal(t, "RequestTimeout", GetResourceProtectionType(err))
	case <-time.After(2 * time.Second):
		t.Fatal("the enforced transport deadline did not unblock the stalled cleanup")
	}

	require.Eventually(t, func() bool {
		return len(protector.requestSemaphore) == 0
	}, requestTimeout+2*time.Second, 2*time.Millisecond,
		"the request slot was not released after the enforced transport deadline")

	// Positive control: an unrelated complete request succeeds while the
	// original stalled connection is still open.
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/ok", nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestSecureBodyReaderWithDeadlineRecoversAcrossStalledWindows repeats the
// stalled-connection pattern over several windows with all clients left open
// until the end. Every window must return its timeout at the enforced deadline
// and recover the full slot budget before the next window starts.
func TestSecureBodyReaderWithDeadlineRecoversAcrossStalledWindows(t *testing.T) {
	const (
		requestTimeout = 50 * time.Millisecond
		maxConcurrent  = 2
		windows        = 2
	)

	protector := NewResourceProtector(ResourceLimits{
		MaxRequestBodySize: 1 << 20,
		MaxRequestTimeout:  requestTimeout,
		MaxConcurrentReq:   maxConcurrent,
		RequestsPerSecond:  1000,
		BurstSize:          50,
	})

	handlerErrs := make(chan error, windows*maxConcurrent)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := protector.SecureBodyReaderWithDeadline(w, r); err != nil {
			handlerErrs <- err
			http.Error(w, err.Error(), http.StatusRequestTimeout)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	addr := strings.TrimPrefix(srv.URL, "http://")
	stalled := make([]net.Conn, 0, windows*maxConcurrent)
	t.Cleanup(func() {
		for _, conn := range stalled {
			_ = conn.Close() //nolint:errcheck // best-effort test cleanup
		}
	})

	for window := 0; window < windows; window++ {
		for i := 0; i < maxConcurrent; i++ {
			conn, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", addr)
			require.NoError(t, err)
			stalled = append(stalled, conn)
			_, err = io.WriteString(conn,
				"POST /stalled HTTP/1.1\r\nHost: "+addr+"\r\nContent-Length: 10\r\nConnection: close\r\n\r\n")
			require.NoError(t, err)
		}

		for i := 0; i < maxConcurrent; i++ {
			select {
			case err := <-handlerErrs:
				require.Equal(t, "RequestTimeout", GetResourceProtectionType(err))
			case <-time.After(2 * time.Second):
				t.Fatalf("window %d: stalled request %d did not return at the enforced deadline", window, i)
			}
		}

		require.Eventually(t, func() bool {
			return len(protector.requestSemaphore) == 0
		}, requestTimeout+2*time.Second, 2*time.Millisecond,
			"window %d: the slot budget did not recover", window)
	}

	stats := protector.GetStats()
	require.LessOrEqual(t, stats.MaxConcurrentReq, int64(maxConcurrent))
	require.Equal(t, int64(0), stats.ConcurrentRequests)
}

// TestSecureBodyReaderWithDeadlineRejectsUnsupportedTransport is the fail-fast
// configuration control: when the response writer cannot enforce a read
// deadline, the helper rejects the request before starting the body read
// instead of entering a cleanup path that cannot be bounded.
func TestSecureBodyReaderWithDeadlineRejectsUnsupportedTransport(t *testing.T) {
	protector := NewResourceProtector(ResourceLimits{
		MaxRequestBodySize: 1024,
		MaxRequestTimeout:  time.Second,
		MaxConcurrentReq:   1,
		RequestsPerSecond:  1000,
		BurstSize:          50,
	})

	body := &readCountingBody{}
	req := (&http.Request{Body: body}).WithContext(context.Background())

	// httptest.ResponseRecorder implements neither SetReadDeadline nor Unwrap,
	// so it models a wrapper that cannot enforce a transport deadline.
	_, err := protector.SecureBodyReaderWithDeadline(httptest.NewRecorder(), req)
	require.Error(t, err)
	require.Equal(t, "TransportDeadlineUnsupported", GetResourceProtectionType(err))
	require.Zero(t, body.readCount(), "the body must not be read without an enforceable deadline")
	require.Equal(t, 0, len(protector.requestSemaphore),
		"the rejected request must not retain its request slot")
}

// TestSecureBodyReaderWithDeadlineReadsCompleteHTTP1Body is the ordinary
// success control for the transport-deadline form: a complete body is read and
// returned normally with the deadline installed.
func TestSecureBodyReaderWithDeadlineReadsCompleteHTTP1Body(t *testing.T) {
	protector := NewResourceProtector(ResourceLimits{
		MaxRequestBodySize: 1 << 20,
		MaxRequestTimeout:  time.Second,
		MaxConcurrentReq:   4,
		RequestsPerSecond:  1000,
		BurstSize:          50,
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := protector.SecureBodyReaderWithDeadline(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(body); err != nil {
			return
		}
	}))
	t.Cleanup(srv.Close)

	payload := []byte("complete request body")
	req, err := http.NewRequestWithContext(
		t.Context(), http.MethodPost, srv.URL, bytes.NewReader(payload))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "text/plain")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, resp.Body.Close()) })
	require.Equal(t, http.StatusOK, resp.StatusCode)

	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, payload, got)
}
