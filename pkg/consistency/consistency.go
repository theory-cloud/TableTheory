// Package consistency provides utilities for handling eventual consistency in DynamoDB
//
// This package offers several patterns for dealing with DynamoDB's eventual consistency:
//
// 1. ConsistentRead() - Use strongly consistent reads on main table queries
// 2. WithRetry() - Retry queries with exponential backoff for GSI eventual consistency
// 3. ReadAfterWriteHelper - Patterns for handling read-after-write scenarios
//
// Example usage:
//
//	// Strong consistency on main table
//	err := db.Model(&User{}).
//	    Where("ID", "=", "123").
//	    ConsistentRead().
//	    First(&user)
//
//	// Retry for GSI eventual consistency
//	err := db.Model(&User{}).
//	    Index("email-index").
//	    Where("Email", "=", "user@example.com").
//	    WithRetry(5, 100*time.Millisecond).
//	    First(&user)
//
//	// Read-after-write pattern
//	helper := consistency.NewReadAfterWriteHelper(db)
//	err := helper.CreateAndQueryGSI(
//	    &user,
//	    "email-index",
//	    "Email",
//	    "user@example.com",
//	    &result,
//	)
package consistency

import (
	"time"
)

// ConsistencyStrategy defines different approaches to handle consistency
type ConsistencyStrategy int

const (
	// StrategyStrongConsistency uses ConsistentRead on main table
	StrategyStrongConsistency ConsistencyStrategy = iota

	// StrategyRetryWithBackoff retries queries with exponential backoff
	StrategyRetryWithBackoff

	// StrategyDelayedRead waits before reading to allow propagation
	StrategyDelayedRead

	// StrategyMainTableFallback falls back to main table if GSI fails
	StrategyMainTableFallback
)

// BestPractices provides recommended patterns for different scenarios
type BestPractices struct{}

// ForGSIQuery returns the recommended approach for GSI queries after writes
func (bp *BestPractices) ForGSIQuery() ConsistencyStrategy {
	// For GSI queries, retry with backoff is usually the best approach
	return StrategyRetryWithBackoff
}

// ForCriticalReads returns the recommended approach for critical reads
func (bp *BestPractices) ForCriticalReads() ConsistencyStrategy {
	// For critical reads, use strong consistency on main table
	return StrategyStrongConsistency
}

// ForHighThroughput returns the recommended approach for high-throughput scenarios
func (bp *BestPractices) ForHighThroughput() ConsistencyStrategy {
	// For high throughput, use delayed reads to reduce retries
	return StrategyDelayedRead
}

// RecommendedRetryConfig returns a recommended retry configuration
func RecommendedRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxRetries:    5,
		InitialDelay:  100 * time.Millisecond,
		MaxDelay:      2 * time.Second,
		BackoffFactor: 2.0,
	}
}

// RecommendedGSIPropagationDelay returns the recommended delay for GSI propagation
// Based on AWS documentation, GSIs typically propagate within seconds
func RecommendedGSIPropagationDelay() time.Duration {
	return 500 * time.Millisecond
}
