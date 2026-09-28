package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWebhookProvider_BackoffIsInterruptibleByContext proves a notify deadline
// bounds the retry backoff: the provider returns as soon as ctx is done instead
// of sleeping through the whole schedule.
func TestWebhookProvider_BackoffIsInterruptibleByContext(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	provider := NewWebhookProvider(WebhookConfig{
		DefaultWebhookURL: server.URL,
		RetryAttempts:     3,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := provider.Send(ctx, &Notification{ID: "webhook-backoff"})
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	// The first backoff alone is a full second. Returning well inside it proves
	// the wait was cut short by the deadline rather than slept through.
	assert.Less(t, elapsed, time.Second)
	assert.Less(t, int(atomic.LoadInt32(&calls)), 3)
}

// TestWebhookProvider_RetriesUntilSuccess proves the retry path still delivers
// when an early attempt fails.
func TestWebhookProvider_RetriesUntilSuccess(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	provider := NewWebhookProvider(WebhookConfig{
		DefaultWebhookURL: server.URL,
		RetryAttempts:     2,
	})

	require.NoError(t, provider.Send(context.Background(), &Notification{ID: "webhook-retry"}))
	assert.Equal(t, int32(2), atomic.LoadInt32(&calls))
}

// TestWebhookProvider_CanceledContextDoesNotSend proves an already-canceled
// context short-circuits the retry loop before any request leaves the process.
func TestWebhookProvider_CanceledContextDoesNotSend(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	provider := NewWebhookProvider(WebhookConfig{
		DefaultWebhookURL: server.URL,
		RetryAttempts:     3,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := provider.Send(ctx, &Notification{ID: "webhook-canceled"})
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, atomic.LoadInt32(&calls))
}
