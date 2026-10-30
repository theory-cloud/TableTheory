package utils

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	payment "github.com/theory-cloud/tabletheory/v6/examples/payment"
	"github.com/theory-cloud/tabletheory/v6/pkg/core"
	customerrors "github.com/theory-cloud/tabletheory/v6/pkg/errors"
)

// ErrDuplicateRequest indicates a duplicate request was detected
var ErrDuplicateRequest = errors.New("duplicate request")

// Replay is the stored outcome of an idempotent request. A caller that receives
// ErrDuplicateRequest with a *Replay must return the recorded status and body
// instead of executing the handler again.
type Replay struct {
	StatusCode int
	Body       any
}

// httpStatusError lets a callback classify a refusal that is not a server
// error, such as a transient rate-limit denial.
type httpStatusError interface {
	HTTPStatus() int
}

const (
	// idempotencyKeyVersionPrefix marks a merchant-scoped stored key so it can
	// never be mistaken for a legacy raw key.
	idempotencyKeyVersionPrefix = "v2#"
	// maxIdempotencyCreateAttempts bounds same-merchant reservation contention.
	maxIdempotencyCreateAttempts = 3
)

// scopedIdempotencyKey derives the stored partition key for a merchant. The
// merchant ID is base64url-encoded without padding so the "#" separators are
// unambiguous and two merchants can never produce the same stored key, even
// when one supplies a key crafted to look like another merchant's prefix.
func scopedIdempotencyKey(merchantID, key string) string {
	encodedMerchant := base64.RawURLEncoding.EncodeToString([]byte(merchantID))
	return idempotencyKeyVersionPrefix + encodedMerchant + "#" + key
}

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
			return &Replay{StatusCode: existing.StatusCode, Body: response}, ErrDuplicateRequest
		}
		// Request is still processing
		return nil, fmt.Errorf("request is still being processed")
	}

	// Only proceed if record not found, otherwise return the error
	if !errors.Is(err, customerrors.ErrItemNotFound) && err.Error() != "idempotency record not found" {
		return nil, fmt.Errorf("failed to check idempotency: %w", err)
	}

	// Reserve a merchant-scoped key. A key another merchant already reserved is
	// a different partition and can no longer deny this merchant.
	record := &payment.IdempotencyRecord{
		Key:        scopedIdempotencyKey(merchantID, key),
		MerchantID: merchantID,
		CreatedAt:  time.Now(),
		ExpiresAt:  time.Now().Add(m.ttl).Unix(), // Unix timestamp for TTL
	}

	created := false
	backoff := 100 * time.Millisecond
	for attempt := 1; attempt <= maxIdempotencyCreateAttempts; attempt++ {
		err := m.db.Model(record).Create()
		if err == nil {
			created = true
			break
		}
		if !isDuplicateError(err) {
			return nil, fmt.Errorf("failed to create idempotency record: %w", err)
		}
		if attempt == maxIdempotencyCreateAttempts {
			break
		}
		time.Sleep(backoff)
		backoff *= 2
	}

	if !created {
		// The reservation belongs to a concurrent same-merchant request. Re-read
		// once: report its response when it has completed, otherwise surface
		// contention. Never execute the handler twice.
		existing, err := m.getRecord(ctx, merchantID, key)
		if err == nil {
			if existing.Response != "" {
				var response any
				if err := json.Unmarshal([]byte(existing.Response), &response); err != nil {
					return nil, fmt.Errorf("failed to unmarshal cached response: %w", err)
				}
				return &Replay{StatusCode: existing.StatusCode, Body: response}, ErrDuplicateRequest
			}
			return nil, fmt.Errorf("request is still being processed")
		}
		return nil, fmt.Errorf("%w: idempotency reservation contention", ErrDuplicateRequest)
	}

	// Execute the function
	result, fnErr := fn()

	// Update record with the outcome. The status is stored with the body so a
	// replay returns the same refusal instead of silently succeeding.
	var responseData []byte
	statusCode := http.StatusOK
	if fnErr != nil {
		statusCode = http.StatusInternalServerError
		var statusErr httpStatusError
		if errors.As(fnErr, &statusErr) {
			statusCode = statusErr.HTTPStatus()
		}
		responseData, _ = json.Marshal(map[string]string{"error": fnErr.Error()})
	} else {
		responseData, _ = json.Marshal(result)
	}

	record.Response = string(responseData)
	record.StatusCode = statusCode

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

	// Read the merchant-scoped key first, then the legacy raw key so records
	// written before scoping remain readable. Both reads are filtered by
	// MerchantID, so one merchant can never read another merchant's row.
	for _, candidate := range []string{scopedIdempotencyKey(merchantID, key), key} {
		var record payment.IdempotencyRecord
		err := m.db.Model(&payment.IdempotencyRecord{}).
			Where("Key", "=", candidate).
			Where("MerchantID", "=", merchantID).
			First(&record)

		if err != nil {
			if errors.Is(err, customerrors.ErrItemNotFound) || err.Error() == "idempotency record not found" {
				continue
			}
			return nil, fmt.Errorf("failed to get idempotency record: %w", err)
		}

		// Check if expired (ExpiresAt is Unix timestamp)
		if time.Now().Unix() > record.ExpiresAt {
			return nil, fmt.Errorf("%w: idempotency record expired", customerrors.ErrItemNotFound)
		}

		return &record, nil
	}

	return nil, fmt.Errorf("%w: idempotency record not found", customerrors.ErrItemNotFound)
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
