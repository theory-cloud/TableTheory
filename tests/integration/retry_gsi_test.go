package integration

import (
	"sync"
	"testing"
	"time"
)

// TestWithRetryGSI verifies that WithRetry works correctly for GSI queries
func TestWithRetryGSI(t *testing.T) {
	ctx := InitTestDB(t)
	t.Cleanup(func() {
		if err := ctx.Cleanup(); err != nil {
			t.Logf("cleanup failed: %v", err)
		}
	})
	db := ctx.DB

	// Create table
	ctx.CreateTable(t, &ConsistencyTestModel{})

	t.Run("WithRetry waits for GSI propagation", func(t *testing.T) {
		item := &ConsistencyTestModel{
			PK:       "USER#retry-gsi-test",
			SK:       "PROFILE",
			Email:    "retry-gsi@example.com",
			Username: "retrygsi",
			Name:     "Retry GSI Test",
		}

		// Create the item
		if err := db.Model(item).Create(); err != nil {
			t.Fatalf("Failed to create item: %v", err)
		}

		// Track query timing
		startTime := time.Now()

		// Query with retry - should retry until data appears
		var results []ConsistencyTestModel
		err := db.Model(&ConsistencyTestModel{}).
			Index("email-index").
			Where("Email", "=", item.Email).
			WithRetry(5, 100*time.Millisecond).
			All(&results)

		queryDuration := time.Since(startTime)

		if err != nil {
			t.Errorf("Query failed with error: %v", err)
		}

		// Even if no error, check if we got results
		if len(results) == 0 {
			t.Errorf("Expected to find item after retries, but got empty results")
		} else if results[0].Name != item.Name {
			t.Errorf("Expected name %s, got %s", item.Name, results[0].Name)
		}

		// If retries happened, query should have taken at least 100ms
		if len(results) > 0 && queryDuration < 100*time.Millisecond {
			t.Logf("Warning: Query succeeded immediately (took %v), GSI might have been already consistent", queryDuration)
		} else {
			t.Logf("Query took %v, indicating retries occurred", queryDuration)
		}
	})

	t.Run("WithRetry respects max attempts", func(t *testing.T) {
		// Query for non-existent item
		startTime := time.Now()

		var results []ConsistencyTestModel
		err := db.Model(&ConsistencyTestModel{}).
			Index("email-index").
			Where("Email", "=", "nonexistent@example.com").
			WithRetry(3, 50*time.Millisecond).
			All(&results)

		queryDuration := time.Since(startTime)

		// Should not error for empty results
		if err != nil {
			t.Errorf("Query should not error for empty results: %v", err)
		}

		// Should have empty results
		if len(results) != 0 {
			t.Errorf("Expected empty results for non-existent item")
		}

		// Should have taken at least 3 * 50ms = 150ms for retries
		expectedMinDuration := 3 * 50 * time.Millisecond
		if queryDuration < expectedMinDuration {
			t.Errorf("Query took %v, expected at least %v for 3 retries", queryDuration, expectedMinDuration)
		}

		t.Logf("Query correctly took %v for 3 retry attempts", queryDuration)
	})

	t.Run("Concurrent writes and reads with retry", func(t *testing.T) {
		var wg sync.WaitGroup

		// Writer goroutine
		wg.Add(1)
		go func() {
			defer wg.Done()
			item := &ConsistencyTestModel{
				PK:       "USER#concurrent-test",
				SK:       "PROFILE",
				Email:    "concurrent@example.com",
				Username: "concurrent",
				Name:     "Concurrent Test",
			}
			if err := db.Model(item).Create(); err != nil {
				t.Errorf("Failed to create item: %v", err)
			}
		}()

		// Reader goroutine with retry
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Give writer a small head start
			time.Sleep(10 * time.Millisecond)

			var results []ConsistencyTestModel
			err := db.Model(&ConsistencyTestModel{}).
				Index("email-index").
				Where("Email", "=", "concurrent@example.com").
				WithRetry(10, 50*time.Millisecond). // More retries for concurrent test
				All(&results)

			if err != nil {
				t.Errorf("Query failed: %v", err)
			}

			if len(results) == 0 {
				t.Errorf("Expected to find item after retries in concurrent test")
			}
		}()

		wg.Wait()
	})
}

// TestWithRetryTiming verifies the timing behavior of WithRetry
func TestWithRetryTiming(t *testing.T) {
	ctx := InitTestDB(t)
	t.Cleanup(func() {
		if err := ctx.Cleanup(); err != nil {
			t.Logf("cleanup failed: %v", err)
		}
	})
	db := ctx.DB

	// Create table
	ctx.CreateTable(t, &ConsistencyTestModel{})

	t.Run("Exponential backoff timing", func(t *testing.T) {
		startTime := time.Now()

		var results []ConsistencyTestModel
		err := db.Model(&ConsistencyTestModel{}).
			Index("email-index").
			Where("Email", "=", "backoff-test@example.com").
			WithRetry(4, 100*time.Millisecond). // 4 retries
			All(&results)

		queryDuration := time.Since(startTime)

		if err != nil {
			t.Errorf("Query failed: %v", err)
		}

		// Calculate expected duration with exponential backoff
		// Retry 1: 100ms
		// Retry 2: 200ms
		// Retry 3: 400ms
		// Retry 4: 800ms
		// Total: 1500ms
		expectedMinDuration := 1500 * time.Millisecond
		tolerance := 100 * time.Millisecond // Allow some variance

		if queryDuration < expectedMinDuration-tolerance {
			t.Errorf("Query took %v, expected at least %v with exponential backoff", queryDuration, expectedMinDuration)
		}

		t.Logf("Query took %v with exponential backoff (expected ~%v)", queryDuration, expectedMinDuration)
	})
}
