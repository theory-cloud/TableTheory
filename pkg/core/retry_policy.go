package core

import "time"

// RetryPolicy defines exponential backoff settings for retryable DynamoDB operations.
type RetryPolicy struct {
	// MaxRetries is the maximum number of retry attempts before giving up.
	MaxRetries int
	// InitialDelay is the base delay between attempts.
	InitialDelay time.Duration
	// MaxDelay caps the exponential backoff delay.
	MaxDelay time.Duration
	// BackoffFactor controls how quickly the delay grows between attempts.
	BackoffFactor float64
	// Jitter adds randomness (as a percentage between 0 and 1) to each delay to avoid thundering herds.
	Jitter float64
}

// DefaultRetryPolicy returns a conservative retry policy suitable for most batch operations.
func DefaultRetryPolicy() *RetryPolicy {
	return &RetryPolicy{
		MaxRetries:    3,
		InitialDelay:  100 * time.Millisecond,
		MaxDelay:      5 * time.Second,
		BackoffFactor: 2.0,
		Jitter:        0.25,
	}
}

// Clone returns a deep copy of the policy so callers can modify it without affecting the original.
func (p *RetryPolicy) Clone() *RetryPolicy {
	if p == nil {
		return nil
	}
	clone := *p
	return &clone
}
