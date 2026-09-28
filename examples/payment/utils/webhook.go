package utils

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/theory-cloud/tabletheory/v4/examples/payment"
	"github.com/theory-cloud/tabletheory/v4/pkg/core"
)

// WebhookSender delivers webhooks synchronously inside the calling invocation.
// It starts no goroutine and owns no queue: Lambda freezes the execution
// environment as soon as the handler returns, so a delivery parked on an
// in-process channel would be frozen with it. Durable hand-off belongs in a
// queue or stream that outlives the invocation on purpose, not in process memory.
type WebhookSender struct {
	db       core.ExtendedDB
	client   *http.Client
	sendWait time.Duration
}

// NewWebhookSender creates a new webhook sender
func NewWebhookSender(db core.ExtendedDB) *WebhookSender {
	return &WebhookSender{
		db:       db,
		client:   &http.Client{Timeout: 30 * time.Second},
		sendWait: 10 * time.Second,
	}
}

// SendSync delivers a webhook synchronously and returns only after the delivery
// attempt has finished. ctx bounds the whole attempt, and no work is left
// running when SendSync returns.
func (w *WebhookSender) SendSync(ctx context.Context, job *WebhookJob) error {
	ctx, cancel := context.WithTimeout(ctx, w.sendWait)
	defer cancel()
	return w.processWebhook(ctx, job)
}

// WebhookJob represents a webhook to be sent
type WebhookJob struct {
	Data       any
	MerchantID string
	EventType  string
	PaymentID  string
}

// WebhookPayload represents the webhook request body
type WebhookPayload struct {
	Created   time.Time `json:"created"`
	Data      any       `json:"data"`
	ID        string    `json:"id"`
	EventType string    `json:"event_type"`
}

// processWebhook handles the actual webhook delivery
func (w *WebhookSender) processWebhook(ctx context.Context, job *WebhookJob) error {
	// Get merchant details
	var merchant payment.Merchant
	err := w.db.Model(&payment.Merchant{}).
		Where("ID", "=", job.MerchantID).
		First(&merchant)

	if err != nil {
		return fmt.Errorf("failed to get merchant: %w", err)
	}

	if merchant.WebhookURL == "" {
		return nil // No webhook configured
	}

	// Create webhook record
	webhook := &payment.Webhook{
		ID:         uuid.New().String(),
		MerchantID: job.MerchantID,
		EventType:  job.EventType,
		PaymentID:  job.PaymentID,
		URL:        merchant.WebhookURL,
		Payload:    map[string]any{"data": job.Data},
		Attempts:   0,
		Status:     payment.WebhookStatusPending,
		CreatedAt:  time.Now(),
		ExpiresAt:  time.Now().Add(24 * time.Hour).Unix(), // Expire after 24 hours (Unix timestamp)
	}

	// Save webhook record
	if err := w.db.Model(webhook).Create(); err != nil {
		return fmt.Errorf("failed to create webhook record: %w", err)
	}

	// Attempt delivery with retries
	return w.deliverWebhook(ctx, webhook, merchant.WebhookSecret)
}

// deliverWebhook attempts to deliver a webhook with exponential backoff
func (w *WebhookSender) deliverWebhook(ctx context.Context, webhook *payment.Webhook, secret string) error {
	maxAttempts := 5
	baseDelay := 1 * time.Second

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Update attempt count
		webhook.Attempts = attempt
		webhook.LastAttempt = time.Now()

		// Create payload
		payload := WebhookPayload{
			ID:        webhook.ID,
			EventType: webhook.EventType,
			Created:   webhook.CreatedAt,
			Data:      webhook.Payload["data"],
		}

		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to marshal payload: %w", err)
		}

		// Create request
		req, err := http.NewRequestWithContext(ctx, "POST", webhook.URL, bytes.NewReader(payloadBytes))
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		// Add headers
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Webhook-ID", webhook.ID)
		req.Header.Set("X-Webhook-Timestamp", fmt.Sprintf("%d", webhook.CreatedAt.Unix()))

		// Add signature if secret is configured
		if secret != "" {
			signature := w.generateSignature(payloadBytes, secret, webhook.CreatedAt)
			req.Header.Set("X-Webhook-Signature", signature)
		}

		// Send request
		resp, err := w.client.Do(req)
		if err != nil {
			webhook.Status = payment.WebhookStatusFailed
			webhook.ResponseBody = fmt.Sprintf("Network error: %v", err)
		} else {
			defer func() {
				_ = resp.Body.Close()
			}()
			webhook.ResponseCode = resp.StatusCode

			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				webhook.Status = payment.WebhookStatusDelivered
				// Update webhook record
				return w.db.Model(webhook).Update("Attempts", "LastAttempt", "Status", "ResponseCode")
			}

			// Non-2xx response
			webhook.Status = payment.WebhookStatusFailed
		}

		// Calculate next retry time
		if attempt < maxAttempts {
			delay := baseDelay * time.Duration(1<<uint(attempt-1)) // Exponential backoff
			webhook.NextRetry = time.Now().Add(delay)
		} else {
			webhook.Status = payment.WebhookStatusExpired
		}

		// Update webhook record
		if err := w.db.Model(webhook).Update("Attempts", "LastAttempt", "Status", "ResponseCode", "ResponseBody", "NextRetry"); err != nil {
			return fmt.Errorf("failed to update webhook record: %w", err)
		}

		// If delivered or expired, we're done
		if webhook.Status == payment.WebhookStatusDelivered || webhook.Status == payment.WebhookStatusExpired {
			break
		}

		// Wait before retry
		if attempt < maxAttempts {
			delay := baseDelay * time.Duration(1<<uint(attempt-1))
			select {
			case <-time.After(delay):
				// Continue to next attempt
			case <-ctx.Done():
				return fmt.Errorf("webhook delivery canceled: %w", ctx.Err())
			}
		}
	}

	return nil
}

// generateSignature creates an HMAC signature for webhook verification
func (w *WebhookSender) generateSignature(payload []byte, secret string, timestamp time.Time) string {
	// Create signature payload: timestamp.payload
	signaturePayload := fmt.Sprintf("%d.%s", timestamp.Unix(), string(payload))

	// Generate HMAC SHA256
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(signaturePayload))

	return fmt.Sprintf("sha256=%x", h.Sum(nil))
}
