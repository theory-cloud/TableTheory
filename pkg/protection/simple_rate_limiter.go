package protection

import (
	"sync"
	"time"
)

// SimpleLimiter is a basic rate limiter implementation
type SimpleLimiter struct {
	lastRefill time.Time
	tokens     int
	maxTokens  int
	refillRate time.Duration
	mu         sync.Mutex
}

// NewSimpleLimiter creates a new simple rate limiter
func NewSimpleLimiter(rps float64, burst int) *SimpleLimiter {
	refillInterval := time.Duration(float64(time.Second) / rps)

	return &SimpleLimiter{
		tokens:     burst,
		maxTokens:  burst,
		refillRate: refillInterval,
		lastRefill: time.Now(),
	}
}

// Allow checks if a request is allowed
func (l *SimpleLimiter) Allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()

	// Refill tokens based on time elapsed
	elapsed := now.Sub(l.lastRefill)
	tokensToAdd := int(elapsed / l.refillRate)

	if tokensToAdd > 0 {
		l.tokens += tokensToAdd
		if l.tokens > l.maxTokens {
			l.tokens = l.maxTokens
		}
		l.lastRefill = now
	}

	// Check if we have tokens
	if l.tokens > 0 {
		l.tokens--
		return true
	}

	return false
}
