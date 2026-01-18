package utils

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	payment "github.com/theory-cloud/tabletheory/examples/payment"
	"github.com/theory-cloud/tabletheory/pkg/core"
	customerrors "github.com/theory-cloud/tabletheory/pkg/errors"
)

// ErrDuplicateRequest indicates a duplicate request was detected
var ErrDuplicateRequest = errors.New("duplicate request")

// IdempotencyMiddleware handles idempotent request processing
type IdempotencyMiddleware struct {
	db  core.ExtendedDB
	ttl time.Duration
}

// NewIdempotencyMiddleware creates a new idempotency middleware
func NewIdempotencyMiddleware(db core.ExtendedDB, ttl time.Duration) *IdempotencyMiddleware {
	return &IdempotencyMiddleware{
		db:  db,
		ttl: ttl,
	}
}

// Process executes a function with idempotency protection
func (m *IdempotencyMiddleware) Process(ctx context.Context, merchantID, key string, fn func() (any, error)) (any, error) {
	// Use the getRecord method to check if request already exists
	existing, err := m.getRecord(ctx, merchantID, key)
	if err == nil {
		// Request already exists
		if existing.Response != "" {
			// Parse and return existing response
			var response any
			if err := json.Unmarshal([]byte(existing.Response), &response); err != nil {
				return nil, fmt.Errorf("failed to unmarshal cached response: %w", err)
			}
			return response, ErrDuplicateRequest
		}
		// Request is still processing
		return nil, fmt.Errorf("request is still being processed")
	}

	// Only proceed if record not found, otherwise return the error
	if !errors.Is(err, customerrors.ErrItemNotFound) && err.Error() != "idempotency record not found" {
		return nil, fmt.Errorf("failed to check idempotency: %w", err)
	}

	// Create new idempotency record with pending status
	record := &payment.IdempotencyRecord{
		Key:        key,
		MerchantID: merchantID,
		CreatedAt:  time.Now(),
		ExpiresAt:  time.Now().Add(m.ttl).Unix(), // Unix timestamp for TTL
	}

	// Try to create the record (will fail if duplicate)
	if err := m.db.Model(record).Create(); err != nil {
		// Check if it's a duplicate key error
		if isDuplicateError(err) {
			// Another request is processing, wait and retry
			time.Sleep(100 * time.Millisecond)
			return m.Process(ctx, merchantID, key, fn)
		}
		return nil, fmt.Errorf("failed to create idempotency record: %w", err)
	}

	// Execute the function
	result, fnErr := fn()

	// Update record with result
	var responseData []byte
	statusCode := 200
	if fnErr != nil {
		statusCode = 500
		responseData, _ = json.Marshal(map[string]string{"error": fnErr.Error()})
	} else {
		responseData, _ = json.Marshal(result)
	}

	record.Response = string(responseData)
	record.StatusCode = statusCode

	// Update with response
	record.Response = string(responseData)
	record.StatusCode = 200

	err = m.db.Model(record).Update("Response", "StatusCode")
	if err != nil {
		// Log error but don't fail the request
		fmt.Printf("Failed to update idempotency record: %v\n", err)
	}

	if fnErr != nil {
		return nil, fnErr
	}

	return result, nil
}

// GenerateKey generates an idempotency key from request data
func (m *IdempotencyMiddleware) GenerateKey(merchantID string, data any) string {
	jsonData, _ := json.Marshal(data)
	hash := sha256.Sum256(append([]byte(merchantID), jsonData...))
	return fmt.Sprintf("%x", hash)
}

// getRecord retrieves an existing idempotency record
func (m *IdempotencyMiddleware) getRecord(ctx context.Context, merchantID, key string) (*payment.IdempotencyRecord, error) {
	// Context could be used here for request cancellation in the future
	_ = ctx

	var record payment.IdempotencyRecord
	err := m.db.Model(&payment.IdempotencyRecord{}).
		Where("Key", "=", key).
		Where("MerchantID", "=", merchantID).
		First(&record)

	if err != nil {
		if err == customerrors.ErrItemNotFound {
			return nil, fmt.Errorf("idempotency record not found")
		}
		return nil, fmt.Errorf("failed to get idempotency record: %w", err)
	}

	// Check if expired (ExpiresAt is Unix timestamp)
	if time.Now().Unix() > record.ExpiresAt {
		return nil, customerrors.ErrItemNotFound
	}

	return &record, nil
}

// CleanupExpired removes expired idempotency records
func (m *IdempotencyMiddleware) CleanupExpired(ctx context.Context) error {
	// Context could be used here for request cancellation in the future
	_ = ctx
	// DynamoDB TTL will handle this automatically
	// This method is for manual cleanup if needed
	return nil
}

// isDuplicateError checks if an error is due to duplicate key
func isDuplicateError(err error) bool {
	// Check for DynamoDB conditional check failed error
	return err != nil && (contains(err.Error(), "ConditionalCheckFailedException") ||
		contains(err.Error(), "duplicate") ||
		contains(err.Error(), "already exists"))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && (s[:len(substr)] == substr || contains(s[1:], substr)))
}
