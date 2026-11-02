package protection

import (
	"bufio"
	"bytes"
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

// blockingBody blocks inside both Read and Close until the test releases it. It
// models the worst case of a stalled Go HTTP/1 server request body: a client
// declares a body and withholds it, so neither Read nor Close returns until the
// connection ends. No real network is involved, so the test can assert the
// documented slot bound without depending on socket timing.
type blockingBody struct {
	release     chan struct{}
	entered     chan struct{}
	enterOnce   sync.Once
	releaseOnce sync.Once
}

func newBlockingBody() *blockingBody {
	return &blockingBody{
		release: make(chan struct{}),
		entered: make(chan struct{}),
	}
}

func (b *blockingBody) Read(_ []byte) (int, error) {
	b.enterOnce.Do(func() { close(b.entered) })
	<-b.release
	return 0, io.EOF
}

func (b *blockingBody) Close() error {
	<-b.release
	return nil
}

func (b *blockingBody) unblock() {
	b.releaseOnce.Do(func() { close(b.release) })
}

// TestSecureBodyReaderTimeoutHoldsSlotUntilCleanup pins the accounting
// guarantee: the request slot is held until the timeout cleanup has finished.
// A body that blocks both Read and Close cannot free the slot while its reader
// and close are still live, so live calls stay bounded by MaxConcurrentReq
// instead of accumulating outside the semaphore after each timeout window.
func TestSecureBodyReaderTimeoutHoldsSlotUntilCleanup(t *testing.T) {
	const requestTimeout = 30 * time.Millisecond

	protector := NewResourceProtector(ResourceLimits{
		MaxRequestBodySize: 1024,
		MaxRequestTimeout:  requestTimeout,
		MaxConcurrentReq:   1,
		RequestsPerSecond:  1000,
		BurstSize:          50,
	})

	body := newBlockingBody()
	defer body.unblock()

	req := (&http.Request{Body: body}).WithContext(t.Context())

	result := make(chan error, 1)
	go func() {
		_, err := protector.SecureBodyReader(req)
		result <- err
	}()

	select {
	case <-body.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("test body never entered Read")
	}

	// Past the timeout the cleanup is still blocked, so the slot must still be
	// accounted. Releasing it here is the defect this test pins: the call would
	// keep running while no longer counting against the bound.
	time.Sleep(requestTimeout + 50*time.Millisecond)
	require.Equal(t, 1, len(protector.requestSemaphore),
		"the request slot must stay accounted while timeout cleanup is still pending")

	select {
	case err := <-result:
		t.Fatalf("SecureBodyReader returned (%v) before the stalled body was unblocked", err)
	default:
	}

	// While the one slot is held, a further request fails fast at admission
	// instead of starting work the bound does not cover.
	_, err := protector.SecureBodyReader(req)
	require.Error(t, err)
	require.Equal(t, "ConcurrencyLimitExceeded", GetResourceProtectionType(err))

	// Unblocking the body ends the cleanup and the call, still reporting the
	// timeout and releasing the slot only now.
	body.unblock()
	select {
	case err := <-result:
		require.Error(t, err)
		require.Equal(t, "RequestTimeout", GetResourceProtectionType(err))
	case <-time.After(2 * time.Second):
		t.Fatal("SecureBodyReader did not return after the body was unblocked")
	}
	require.Equal(t, 0, len(protector.requestSemaphore))
}

// TestSecureBodyReaderStalledHTTP1BodyHoldsSlotUntilCleanup reproduces the
// reported attack over a real HTTP/1 connection: a client declares a body and
// withholds it. The request slot must stay accounted while the timeout cleanup
// is still blocked, so a second request is rejected fast rather than starting
// work outside the bound. Closing the stalled client ends the cleanup, frees the
// slot, and restores ordinary service.
func TestSecureBodyReaderStalledHTTP1BodyHoldsSlotUntilCleanup(t *testing.T) {
	const requestTimeout = 100 * time.Millisecond

	protector := NewResourceProtector(ResourceLimits{
		MaxRequestBodySize: 1 << 20,
		MaxRequestTimeout:  requestTimeout,
		MaxConcurrentReq:   1,
		RequestsPerSecond:  1000,
		BurstSize:          50,
	})

	handlerStarted := make(chan struct{})
	var handlerStartOnce sync.Once

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerStartOnce.Do(func() { close(handlerStarted) })
		if _, err := protector.SecureBodyReader(r); err != nil {
			http.Error(w, err.Error(), http.StatusRequestTimeout)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	// Close the stalled client connection before the server: net/http's
	// connection goroutine can be parked in the same stalled body read, and
	// closing the client unblocks it so the server can shut down.
	t.Cleanup(srv.Close)

	addr := strings.TrimPrefix(srv.URL, "http://")
	conn, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", addr)
	require.NoError(t, err)
	t.Cleanup(func() {
		// The stalled connection is closed best-effort; the server may already
		// have reset it.
		_ = conn.Close() //nolint:errcheck // best-effort test cleanup
	})

	// Complete headers that declare a body, then never send the body.
	_, err = io.WriteString(conn,
		"POST /stalled HTTP/1.1\r\nHost: "+addr+"\r\nContent-Length: 10\r\nConnection: close\r\n\r\n")
	require.NoError(t, err)

	select {
	case <-handlerStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("server handler never started")
	}

	// The handler closes handlerStarted before it acquires the slot, so first
	// wait for the stalled request to actually hold the slot. Without this the
	// checks below can pass spuriously in the pre-acquisition window.
	require.Eventually(t, func() bool {
		return len(protector.requestSemaphore) == 1
	}, requestTimeout+2*time.Second, 2*time.Millisecond,
		"stalled HTTP/1 request never acquired the request slot")

	// Past the timeout the cleanup is still blocked on the live socket, so the
	// slot must remain accounted rather than being freed early.
	time.Sleep(requestTimeout + 100*time.Millisecond)
	require.Equal(t, 1, len(protector.requestSemaphore),
		"the request slot was released while the stalled HTTP/1 cleanup was still pending")

	// Negative control: while cleanup is pending, an unrelated request is
	// rejected fast at admission and cannot accumulate behind the bound.
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/ok", nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusRequestTimeout, resp.StatusCode)

	// Closing the stalled client unblocks the reader and Close, so the timeout
	// cleanup finishes and the slot is released.
	require.NoError(t, conn.Close())
	require.Eventually(t, func() bool {
		return len(protector.requestSemaphore) == 0
	}, requestTimeout+2*time.Second, 2*time.Millisecond,
		"the request slot was not released after the stalled client disconnected")

	// Positive control after recovery: ordinary requests succeed again.
	resp2, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { require.NoError(t, resp2.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp2.StatusCode)
}

// TestSecureBodyReaderStalledConnectionsStayBoundedAcrossWindows drives several
// stalled HTTP/1 connections over repeated timeout windows. At every point the
// number of live slot-holders must stay at or below MaxConcurrentReq, surplus
// connections must be rejected fast instead of starting work, and complete
// requests must succeed once the stalled clients disconnect.
func TestSecureBodyReaderStalledConnectionsStayBoundedAcrossWindows(t *testing.T) {
	const (
		requestTimeout = 40 * time.Millisecond
		maxConcurrent  = 2
		windows        = 3
	)

	protector := NewResourceProtector(ResourceLimits{
		MaxRequestBodySize: 1 << 20,
		MaxRequestTimeout:  requestTimeout,
		MaxConcurrentReq:   maxConcurrent,
		RequestsPerSecond:  1000,
		BurstSize:          50,
	})

	handlerStarted := make(chan struct{}, windows*maxConcurrent+8)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerStarted <- struct{}{}
		if _, err := protector.SecureBodyReader(r); err != nil {
			http.Error(w, err.Error(), http.StatusRequestTimeout)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	addr := strings.TrimPrefix(srv.URL, "http://")
	dialStalled := func() net.Conn {
		t.Helper()
		conn, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", addr)
		require.NoError(t, err)
		_, err = io.WriteString(conn,
			"POST /stalled HTTP/1.1\r\nHost: "+addr+"\r\nContent-Length: 10\r\nConnection: close\r\n\r\n")
		require.NoError(t, err)
		return conn
	}

	for window := 0; window < windows; window++ {
		conns := make([]net.Conn, 0, maxConcurrent)
		for i := 0; i < maxConcurrent; i++ {
			conn := dialStalled()
			conns = append(conns, conn)
			select {
			case <-handlerStarted:
			case <-time.After(2 * time.Second):
				t.Fatalf("window %d: stalled handler %d never started", window, i)
			}
		}

		require.Eventually(t, func() bool {
			return len(protector.requestSemaphore) == maxConcurrent
		}, requestTimeout+2*time.Second, 2*time.Millisecond,
			"window %d: the stalled connections never filled the request bound", window)

		// The timeouts have all expired, but cleanup cannot finish on the live
		// sockets, so the bound must still be fully accounted.
		time.Sleep(requestTimeout + 40*time.Millisecond)
		require.Equal(t, maxConcurrent, len(protector.requestSemaphore),
			"window %d: a request slot was released while cleanup was still pending", window)

		// A surplus stalled connection is rejected fast at admission: it never
		// acquires a slot or starts its body read.
		extra := dialStalled()
		require.NoError(t, extra.SetReadDeadline(time.Now().Add(2*time.Second)))
		statusLine, err := bufio.NewReader(extra).ReadString('\n')
		require.NoError(t, err,
			"window %d: the surplus stalled request received no fast rejection", window)
		require.Contains(t, statusLine, "408",
			"window %d: the surplus request was not rejected at admission: %q", window, statusLine)
		require.NoError(t, extra.Close())

		// Disconnecting the window's stalled clients lets every cleanup finish.
		for _, conn := range conns {
			require.NoError(t, conn.Close())
		}
		require.Eventually(t, func() bool {
			return len(protector.requestSemaphore) == 0
		}, requestTimeout+2*time.Second, 2*time.Millisecond,
			"window %d: request slots did not recover after the stalled clients disconnected", window)
	}

	// The protector's own high-water mark never exceeded the configured bound,
	// across every window and connection.
	stats := protector.GetStats()
	require.LessOrEqual(t, stats.MaxConcurrentReq, int64(maxConcurrent),
		"the live request high-water mark escaped MaxConcurrentReq")
	require.Equal(t, int64(0), stats.ConcurrentRequests,
		"live request accounting did not return to zero after cleanup")

	// Ordinary success control after all windows.
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/ok", nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestSecureBodyReaderReadsCompleteHTTP1Body is the authorized-success control:
// a request that sends its whole declared body is read and returned normally.
func TestSecureBodyReaderReadsCompleteHTTP1Body(t *testing.T) {
	protector := NewResourceProtector(ResourceLimits{
		MaxRequestBodySize: 1 << 20,
		MaxRequestTimeout:  time.Second,
		MaxConcurrentReq:   4,
		RequestsPerSecond:  1000,
		BurstSize:          50,
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := protector.SecureBodyReader(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(body); err != nil { //nolint:gosec // test-only echo back to the authorized test client
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
