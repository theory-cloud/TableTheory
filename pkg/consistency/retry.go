package consistency

import (
	"context"
	"fmt"
	"time"

	"github.com/theory-cloud/tabletheory/pkg/core"
)

// RetryConfig configures retry behavior for eventually consistent reads
type RetryConfig struct {
	RetryCondition func(result any, err error) bool
	MaxRetries     int
	InitialDelay   time.Duration
	MaxDelay       time.Duration
	BackoffFactor  float64
}

// DefaultRetryConfig returns a default retry configuration
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxRetries:    5,
		InitialDelay:  100 * time.Millisecond,
		MaxDelay:      5 * time.Second,
		BackoffFactor: 2.0,
		RetryCondition: func(result any, err error) bool {
			// By default, retry on empty results or errors
			return err != nil || result == nil
		},
	}
}

// RetryableQuery wraps a Query with retry capability for eventual consistency
type RetryableQuery struct {
	query  core.Query
	config *RetryConfig
}

// WithRetry adds retry capability to a query for handling eventual consistency
func WithRetry(query core.Query, config *RetryConfig) *RetryableQuery {
	if config == nil {
		config = DefaultRetryConfig()
	}
	return &RetryableQuery{
		query:  query,
		config: config,
	}
}

func (r *RetryableQuery) executeWithRetry(dest any, exec func(any) error) error {
	var lastErr error
	delay := r.config.InitialDelay

	for attempt := 0; attempt <= r.config.MaxRetries; attempt++ {
		// Execute the query
		err := exec(dest)

		// Check if we should retry
		if !r.config.RetryCondition(dest, err) {
			return err
		}

		lastErr = err

		// Don't sleep on the last attempt
		if attempt < r.config.MaxRetries {
			time.Sleep(delay)

			// Calculate next delay with exponential backoff
			delay = time.Duration(float64(delay) * r.config.BackoffFactor)
			if delay > r.config.MaxDelay {
				delay = r.config.MaxDelay
			}
		}
	}

	if lastErr != nil {
		return fmt.Errorf("query failed after %d retries: %w", r.config.MaxRetries, lastErr)
	}
	return fmt.Errorf("query returned no results after %d retries", r.config.MaxRetries)
}

// First executes the query with retries
func (r *RetryableQuery) First(dest any) error {
	return r.executeWithRetry(dest, r.query.First)
}

// All executes the query with retries
func (r *RetryableQuery) All(dest any) error {
	return r.executeWithRetry(dest, r.query.All)
}

// RetryWithVerification retries a query until a verification function returns true
func RetryWithVerification(ctx context.Context, query core.Query, dest any, verify func(any) bool, config *RetryConfig) error {
	if config == nil {
		config = DefaultRetryConfig()
	}

	delay := config.InitialDelay

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Execute the query
		err := query.First(dest)
		if err != nil {
			// If there's an error, we might want to retry
			if attempt < config.MaxRetries {
				time.Sleep(delay)
				delay = time.Duration(float64(delay) * config.BackoffFactor)
				if delay > config.MaxDelay {
					delay = config.MaxDelay
				}
				continue
			}
			return fmt.Errorf("query failed after %d retries: %w", config.MaxRetries, err)
		}

		// Verify the result
		if verify(dest) {
			return nil
		}

		// Don't sleep on the last attempt
		if attempt < config.MaxRetries {
			time.Sleep(delay)
			delay = time.Duration(float64(delay) * config.BackoffFactor)
			if delay > config.MaxDelay {
				delay = config.MaxDelay
			}
		}
	}

	return fmt.Errorf("verification failed after %d retries", config.MaxRetries)
}
