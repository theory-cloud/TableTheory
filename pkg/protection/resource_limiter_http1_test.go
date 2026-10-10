package protection

import (
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

// TestSecureBodyReaderTimeoutReleasesSlotBeforeBlockedCleanup pins the core
// guarantee: when a stalled body cannot be unblocked, the request slot is
// released no later than MaxRequestTimeout even though Close and the reader join
// stay blocked. The call itself may remain blocked (the reader is joined, not
// abandoned), but the shared slot is already free, so it can no longer be
// exhausted by stalled connections.
func TestSecureBodyReaderTimeoutReleasesSlotBeforeBlockedCleanup(t *testing.T) {
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

	// The slot must be free within the documented bound (MaxRequestTimeout),
	// even though the call itself is still blocked in body cleanup.
	require.Eventually(t, func() bool {
		return len(protector.requestSemaphore) == 0
	}, requestTimeout+2*time.Second, 2*time.Millisecond,
		"request slot retained while the stalled body reader is still blocked")

	select {
	case err := <-result:
		t.Fatalf("SecureBodyReader returned (%v) before the stalled body was unblocked", err)
	default:
	}

	// Unblocking the body ends the cleanup and the call, still reporting the
	// timeout and leaving the slot free.
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

// TestSecureBodyReaderStalledHTTP1BodyReleasesRequestSlot reproduces the
// reported attack over real HTTP/1 connections: a client declares a body and
// withholds it. The shared request slot must be released within the documented
// bound, and an unrelated complete request must succeed afterwards. The
// previous implementation released the slot only after an unbounded Close and
// reader join, so it held the slot until the client disconnected — which this
// test would observe as the slot never freeing.
func TestSecureBodyReaderStalledHTTP1BodyReleasesRequestSlot(t *testing.T) {
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
	// release check below can pass spuriously — observing a free slot before the
	// stalled request ever acquired one — and the follow-up control request then
	// races the still-in-flight hold and is correctly rejected as over the
	// concurrency limit.
	require.Eventually(t, func() bool {
		return len(protector.requestSemaphore) == 1
	}, requestTimeout+2*time.Second, 2*time.Millisecond,
		"stalled HTTP/1 request never acquired the request slot")

	// The slot is released within the documented bound even though the client
	// never sends the declared body.
	require.Eventually(t, func() bool {
		return len(protector.requestSemaphore) == 0
	}, requestTimeout+2*time.Second, 2*time.Millisecond,
		"stalled HTTP/1 body held the request slot past the documented bound")

	// Positive control: an unrelated complete request succeeds while the
	// stalled connection is still open.
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/ok", nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, resp.Body.Close()) })
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
