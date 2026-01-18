package protection

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestResourceLimitsDefaults tests default resource limits
func TestResourceLimitsDefaults(t *testing.T) {
	defaults := DefaultResourceLimits()

	assert.Equal(t, int64(10*1024*1024), defaults.MaxRequestBodySize)
	assert.Equal(t, 30*time.Second, defaults.MaxRequestTimeout)
	assert.Equal(t, 100, defaults.MaxConcurrentReq)
	assert.Equal(t, 25, defaults.MaxBatchSize)
	assert.Equal(t, 10, defaults.MaxConcurrentBatch)
	assert.Equal(t, float64(100), defaults.BatchRateLimit)
	assert.Equal(t, int64(500), defaults.MaxMemoryMB)
	assert.Equal(t, 5*time.Second, defaults.MemoryCheckInterval)
	assert.Equal(t, 0.9, defaults.MemoryPanicThreshold)
	assert.Equal(t, float64(1000), defaults.RequestsPerSecond)
	assert.Equal(t, 50, defaults.BurstSize)
}

// TestResourceProtectorCreation tests resource protector creation
func TestResourceProtectorCreation(t *testing.T) {
	config := DefaultResourceLimits()
	protector := NewResourceProtector(config)

	assert.NotNil(t, protector)
	assert.NotNil(t, protector.globalLimiter)
	assert.NotNil(t, protector.batchLimiter)
	assert.NotNil(t, protector.memoryMonitor)
	assert.NotNil(t, protector.stats)
	assert.Equal(t, config, protector.config)
}

// TestSimpleRateLimiter tests the simple rate limiter implementation
func TestSimpleRateLimiter(t *testing.T) {
	t.Run("AllowsInitialBurst", func(t *testing.T) {
		limiter := NewSimpleLimiter(10, 5) // 10 RPS, burst of 5

		// Should allow initial burst
		for i := 0; i < 5; i++ {
			assert.True(t, limiter.Allow(), "Should allow initial burst request %d", i)
		}

		// Should reject after burst
		assert.False(t, limiter.Allow(), "Should reject after burst")
	})

	t.Run("RefillsTokensOverTime", func(t *testing.T) {
		limiter := NewSimpleLimiter(100, 1) // 100 RPS, burst of 1

		// Use up the initial token
		assert.True(t, limiter.Allow())
		assert.False(t, limiter.Allow())

		// Wait for refill (10ms at 100 RPS)
		time.Sleep(15 * time.Millisecond)

		// Should have refilled
		assert.True(t, limiter.Allow())
	})

	t.Run("DoesNotExceedMaxTokens", func(t *testing.T) {
		limiter := NewSimpleLimiter(1000, 2) // High RPS, burst of 2

		// Wait longer than needed to refill many tokens
		time.Sleep(100 * time.Millisecond)

		// Should only allow burst amount
		assert.True(t, limiter.Allow())
		assert.True(t, limiter.Allow())
		assert.False(t, limiter.Allow())
	})

	t.Run("ConcurrentSafety", func(t *testing.T) {
		limiter := NewSimpleLimiter(10, 100) // Lower RPS to prevent refills during test

		const numGoroutines = 50
		const requestsPerGoroutine = 10

		var wg sync.WaitGroup
		results := make(chan bool, numGoroutines*requestsPerGoroutine)

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < requestsPerGoroutine; j++ {
					results <- limiter.Allow()
				}
			}()
		}

		wg.Wait()
		close(results)

		// Count allowed requests
		allowedCount := 0
		for result := range results {
			if result {
				allowedCount++
			}
		}

		// Should have allowed some requests (up to burst limit + small refill allowance)
		assert.Greater(t, allowedCount, 0)
		assert.LessOrEqual(t, allowedCount, 110) // Allow for small refill during test
	})
}

// TestSecureBodyReader tests HTTP body reading protection
func TestSecureBodyReader(t *testing.T) {
	config := ResourceLimits{
		MaxRequestBodySize: 1024, // 1KB limit
		MaxRequestTimeout:  100 * time.Millisecond,
		MaxConcurrentReq:   2,
		RequestsPerSecond:  100,
		BurstSize:          10,
	}
	protector := NewResourceProtector(config)

	t.Run("ReadsNormalRequest", func(t *testing.T) {
		body := strings.NewReader("normal request body")
		req := &http.Request{Body: io.NopCloser(body)}
		req = req.WithContext(context.Background())

		data, err := protector.SecureBodyReader(req)
		assert.NoError(t, err)
		assert.Equal(t, "normal request body", string(data))

		stats := protector.GetStats()
		assert.Equal(t, int64(1), stats.TotalRequests)
	})

	t.Run("RejectsOversizedRequest", func(t *testing.T) {
		largeBody := strings.NewReader(strings.Repeat("a", 2048)) // 2KB > 1KB limit
		req := &http.Request{Body: io.NopCloser(largeBody)}
		req = req.WithContext(context.Background())

		_, err := protector.SecureBodyReader(req)
		assert.Error(t, err)
		// Should be rejected by http.MaxBytesReader before our protection
	})

	t.Run("EnforcesConcurrencyLimit", func(t *testing.T) {
		// Use blocking readers to test concurrency
		blockingBodies := make([]io.ReadCloser, 5)
		for i := range blockingBodies {
			blockingBodies[i] = &slowReader{delay: 200 * time.Millisecond}
		}

		var wg sync.WaitGroup
		results := make(chan error, 5)

		// Start 5 concurrent requests (limit is 2)
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				req := &http.Request{Body: blockingBodies[idx]}
				req = req.WithContext(context.Background())
				_, err := protector.SecureBodyReader(req)
				results <- err
			}(i)
		}

		wg.Wait()
		close(results)

		// Count concurrency limit errors
		concurrencyErrors := 0
		for err := range results {
			if IsResourceProtectionError(err) && GetResourceProtectionType(err) == "ConcurrencyLimitExceeded" {
				concurrencyErrors++
			}
		}

		// Should have rejected some requests due to concurrency limit
		assert.Greater(t, concurrencyErrors, 0)
	})

	t.Run("EnforcesRateLimit", func(t *testing.T) {
		// Create limiter with very low rate
		lowRateConfig := ResourceLimits{
			MaxRequestBodySize: 1024,
			MaxRequestTimeout:  100 * time.Millisecond,
			MaxConcurrentReq:   10,
			RequestsPerSecond:  1, // Very low rate
			BurstSize:          2, // Small burst
		}
		lowRateProtector := NewResourceProtector(lowRateConfig)

		// Make requests beyond burst limit
		rateLimitErrors := 0
		for i := 0; i < 5; i++ {
			body := strings.NewReader("test")
			req := &http.Request{Body: io.NopCloser(body)}
			req = req.WithContext(context.Background())

			_, err := lowRateProtector.SecureBodyReader(req)
			if IsResourceProtectionError(err) && GetResourceProtectionType(err) == "RateLimitExceeded" {
				rateLimitErrors++
			}
		}

		// Should have hit rate limit
		assert.Greater(t, rateLimitErrors, 0)

		stats := lowRateProtector.GetStats()
		assert.Greater(t, stats.RateLimitHits, int64(0))
	})
}

// TestBatchLimiter tests batch operation protection
func TestBatchLimiter(t *testing.T) {
	config := ResourceLimits{
		MaxBatchSize:       10,
		MaxConcurrentBatch: 2,
		BatchRateLimit:     100,
	}
	protector := NewResourceProtector(config)
	limiter := protector.GetBatchLimiter()

	t.Run("AllowsValidBatch", func(t *testing.T) {
		ctx := context.Background()
		err := limiter.AcquireBatch(ctx, 5)
		assert.NoError(t, err)

		limiter.ReleaseBatch()

		stats := protector.GetStats()
		assert.Equal(t, int64(1), stats.TotalBatchOps)
	})

	t.Run("RejectsOversizedBatch", func(t *testing.T) {
		ctx := context.Background()
		err := limiter.AcquireBatch(ctx, 15) // > 10 limit
		assert.Error(t, err)
		assert.True(t, IsResourceProtectionError(err))
		assert.Equal(t, "BatchSizeExceeded", GetResourceProtectionType(err))

		stats := protector.GetStats()
		assert.Greater(t, stats.RejectedBatchOps, int64(0))
	})

	t.Run("EnforcesConcurrencyLimit", func(t *testing.T) {
		ctx := context.Background()

		// Acquire max concurrent batches
		err1 := limiter.AcquireBatch(ctx, 1)
		assert.NoError(t, err1)

		err2 := limiter.AcquireBatch(ctx, 1)
		assert.NoError(t, err2)

		// Third should be rejected
		err3 := limiter.AcquireBatch(ctx, 1)
		assert.Error(t, err3)
		assert.Equal(t, "BatchConcurrencyExceeded", GetResourceProtectionType(err3))

		// Release and try again
		limiter.ReleaseBatch()
		err4 := limiter.AcquireBatch(ctx, 1)
		assert.NoError(t, err4)

		limiter.ReleaseBatch()
		limiter.ReleaseBatch()
	})

	t.Run("EnforcesRateLimit", func(t *testing.T) {
		// Create limiter with very low batch rate
		lowRateConfig := ResourceLimits{
			MaxBatchSize:       10,
			MaxConcurrentBatch: 10,
			BatchRateLimit:     2, // Low but not too low
		}
		lowRateProtector := NewResourceProtector(lowRateConfig)
		lowRateLimiter := lowRateProtector.GetBatchLimiter()

		ctx := context.Background()

		// Use up the burst allowance (should be 1 for rate 2)
		err1 := lowRateLimiter.AcquireBatch(ctx, 1)
		assert.NoError(t, err1)
		lowRateLimiter.ReleaseBatch()

		// Make rapid requests to hit rate limit
		rateLimitErrors := 0
		successCount := 0
		for i := 0; i < 5; i++ {
			err := lowRateLimiter.AcquireBatch(ctx, 1)
			if IsResourceProtectionError(err) && GetResourceProtectionType(err) == "BatchRateLimitExceeded" {
				rateLimitErrors++
			} else if err == nil {
				successCount++
				lowRateLimiter.ReleaseBatch()
			}
		}

		// Should have hit rate limit at least once
		assert.Greater(t, rateLimitErrors, 0, "Expected at least one rate limit error")
		t.Logf("Rate limit errors: %d, Successful requests: %d", rateLimitErrors, successCount)
	})
}

// TestMemoryMonitoring tests memory usage monitoring
func TestMemoryMonitoring(t *testing.T) {
	config := ResourceLimits{
		MaxMemoryMB:          100, // Low limit for testing
		MemoryCheckInterval:  50 * time.Millisecond,
		MemoryPanicThreshold: 0.1, // Very low threshold for testing
	}
	protector := NewResourceProtector(config)

	t.Run("MonitorsMemoryUsage", func(t *testing.T) {
		alerts := make(chan MemoryAlert, 1)
		alertCallback := func(alert MemoryAlert) {
			select {
			case alerts <- alert:
			default:
			}
		}

		protector.StartMemoryMonitoring(alertCallback)
		defer protector.StopMemoryMonitoring()

		// Wait for monitoring to detect current memory usage
		time.Sleep(100 * time.Millisecond)

		stats := protector.GetStats()
		assert.GreaterOrEqual(t, stats.CurrentMemoryMB, int64(0), "Memory usage should be non-negative")

		// Should trigger alert due to low threshold
		select {
		case alert := <-alerts:
			assert.Equal(t, "MemoryThresholdExceeded", alert.Type)
			assert.Greater(t, alert.UsagePercent, 0.0)
			assert.NotEmpty(t, alert.Severity)
		case <-time.After(200 * time.Millisecond):
			// Alert might not trigger if actual memory usage is very low
			t.Log("No memory alert triggered - actual memory usage might be below threshold")
		}
	})

	t.Run("StopsMonitoring", func(t *testing.T) {
		protector.StartMemoryMonitoring(nil)
		assert.Equal(t, int32(1), atomic.LoadInt32(&protector.memoryMonitor.running))

		protector.StopMemoryMonitoring()

		// Give monitor loop time to exit
		time.Sleep(10 * time.Millisecond)

		assert.Equal(t, int32(0), atomic.LoadInt32(&protector.memoryMonitor.running))
	})
}

func TestMemoryMonitor_determineSeverity_COV6(t *testing.T) {
	mm := &MemoryMonitor{}

	assert.Equal(t, "CRITICAL", mm.determineSeverity(0.95))
	assert.Equal(t, "HIGH", mm.determineSeverity(0.90))
	assert.Equal(t, "MEDIUM", mm.determineSeverity(0.80))
	assert.Equal(t, "LOW", mm.determineSeverity(0.10))
}

func TestMemoryMonitor_checkMemory_TriggersAlertAndGC_COV6(t *testing.T) {
	config := DefaultResourceLimits()
	config.MaxMemoryMB = 1
	config.MemoryPanicThreshold = 0.0

	protector := NewResourceProtector(config)

	// Ensure the heap is non-trivially allocated so memory usage crosses the threshold.
	buf := make([]byte, 10*1024*1024)
	for i := range buf {
		buf[i] = byte(i)
	}

	alerts := make(chan MemoryAlert, 1)
	protector.memoryMonitor.mu.Lock()
	protector.memoryMonitor.alertCallback = func(alert MemoryAlert) {
		select {
		case alerts <- alert:
		default:
		}
	}
	protector.memoryMonitor.mu.Unlock()

	protector.memoryMonitor.checkMemory()

	stats := protector.GetStats()
	assert.Greater(t, stats.MemoryAlerts, int64(0))
	assert.GreaterOrEqual(t, stats.CurrentMemoryMB, int64(0))

	select {
	case alert := <-alerts:
		assert.Equal(t, "MemoryThresholdExceeded", alert.Type)
		assert.NotEmpty(t, alert.Severity)
		assert.Greater(t, alert.UsagePercent, 0.0)
	case <-time.After(2 * time.Second):
		t.Fatalf("expected memory alert callback to run")
	}
}

// TestProtectionErrors tests protection error handling
func TestProtectionErrors(t *testing.T) {
	t.Run("ProtectionErrorInterface", func(t *testing.T) {
		err := &ProtectionError{
			Type:   "TestError",
			Detail: "test detail",
		}

		assert.True(t, IsResourceProtectionError(err))
		assert.Equal(t, "TestError", GetResourceProtectionType(err))
		assert.Contains(t, err.Error(), "TestError")
		assert.Contains(t, err.Error(), "test detail")
	})

	t.Run("NonProtectionError", func(t *testing.T) {
		err := assert.AnError
		assert.False(t, IsResourceProtectionError(err))
		assert.Empty(t, GetResourceProtectionType(err))
	})
}

// TestHealthCheck tests resource protection health checks
func TestHealthCheck(t *testing.T) {
	config := DefaultResourceLimits()
	protector := NewResourceProtector(config)

	health := protector.HealthCheck()

	assert.NotNil(t, health)
	assert.Equal(t, "healthy", health["status"])

	checks, ok := health["checks"].(map[string]any)
	assert.True(t, ok)
	if !ok {
		return
	}
	assert.Contains(t, checks, "memory")
	assert.Contains(t, checks, "concurrency")
	assert.Contains(t, checks, "rate_limiting")

	memoryCheck, ok := checks["memory"].(map[string]any)
	assert.True(t, ok)
	if !ok {
		return
	}
	assert.Equal(t, "ok", memoryCheck["status"])
	assert.GreaterOrEqual(t, memoryCheck["current_mb"], int64(0))
}

func TestHealthCheck_DegradesOnHighUsage_COV6(t *testing.T) {
	config := DefaultResourceLimits()
	config.MaxMemoryMB = 100
	config.MaxConcurrentReq = 10

	protector := NewResourceProtector(config)
	atomic.StoreInt64(&protector.stats.CurrentMemoryMB, 95)
	atomic.StoreInt64(&protector.stats.ConcurrentRequests, 9)

	health := protector.HealthCheck()
	assert.Equal(t, "degraded", health["status"])

	checks, ok := health["checks"].(map[string]any)
	assert.True(t, ok)
	if !ok {
		return
	}

	memoryCheck, ok := checks["memory"].(map[string]any)
	assert.True(t, ok)
	if !ok {
		return
	}
	assert.Equal(t, "warning", memoryCheck["status"])

	concurrencyCheck, ok := checks["concurrency"].(map[string]any)
	assert.True(t, ok)
	if !ok {
		return
	}
	assert.Equal(t, "warning", concurrencyCheck["status"])
}

// TestResourceStats tests statistics tracking
func TestResourceStats(t *testing.T) {
	config := DefaultResourceLimits()
	protector := NewResourceProtector(config)

	// Make some requests to generate stats
	body := strings.NewReader("test")
	req := &http.Request{Body: io.NopCloser(body)}
	req = req.WithContext(context.Background())

	_, err := protector.SecureBodyReader(req)
	assert.NoError(t, err)

	stats := protector.GetStats()
	assert.Greater(t, stats.TotalRequests, int64(0))
	assert.NotZero(t, stats.LastStatsUpdate)
}

// slowReader simulates a slow request body for testing timeouts
type slowReader struct {
	delay time.Duration
	read  bool
}

func (sr *slowReader) Read(p []byte) (n int, err error) {
	if !sr.read {
		time.Sleep(sr.delay)
		sr.read = true
		copy(p, []byte("slow data"))
		return 9, nil
	}
	return 0, io.EOF
}

func (sr *slowReader) Close() error {
	return nil
}

// Benchmark tests for performance monitoring
func BenchmarkResourceProtection(b *testing.B) {
	config := DefaultResourceLimits()
	protector := NewResourceProtector(config)

	b.Run("SecureBodyReader", func(b *testing.B) {
		body := "benchmark test data"

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			reader := strings.NewReader(body)
			req := &http.Request{Body: io.NopCloser(reader)}
			req = req.WithContext(context.Background())

			if _, err := protector.SecureBodyReader(req); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("BatchLimiter", func(b *testing.B) {
		limiter := protector.GetBatchLimiter()
		ctx := context.Background()

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			err := limiter.AcquireBatch(ctx, 1)
			if err == nil {
				limiter.ReleaseBatch()
			}
		}
	})

	b.Run("RateLimiter", func(b *testing.B) {
		limiter := NewSimpleLimiter(1000, 100)

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			limiter.Allow()
		}
	})
}
