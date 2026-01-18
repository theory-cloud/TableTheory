package utils

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/theory-cloud/tabletheory"
	"github.com/theory-cloud/tabletheory/examples/payment"
	"github.com/theory-cloud/tabletheory/pkg/core"
)

// WebhookSender handles async webhook deliveries
type WebhookSender struct {
	db       core.ExtendedDB
	ctx      context.Context
	client   *http.Client
	workers  chan struct{}
	queue    chan *WebhookJob
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	retryMax int
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

// NewWebhookSender creates a new webhook sender
func NewWebhookSender(db core.ExtendedDB, workers int) *WebhookSender {
	ctx, cancel := context.WithCancel(context.Background())

	sender := &WebhookSender{
		db:       db,
		client:   &http.Client{Timeout: 30 * time.Second},
		workers:  make(chan struct{}, workers),
		retryMax: 3,
		queue:    make(chan *WebhookJob, 1000),
		ctx:      ctx,
		cancel:   cancel,
	}

	// Start workers
	for i := 0; i < workers; i++ {
		sender.wg.Add(1)
		go sender.worker()
	}

	return sender
}

// Send queues a webhook for delivery
func (w *WebhookSender) Send(job *WebhookJob) error {
	select {
	case w.queue <- job:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("webhook queue full")
	}
}

// Stop gracefully shuts down the webhook sender
func (w *WebhookSender) Stop() {
	close(w.queue)
	w.cancel()
	w.wg.Wait()
}

// worker processes webhook jobs
func (w *WebhookSender) worker() {
	defer w.wg.Done()

	for job := range w.queue {
		if err := w.processWebhook(job); err != nil {
			// Log error (in production, use proper logging)
			fmt.Printf("Failed to process webhook: %v\n", err)
		}
	}
}

// processWebhook handles the actual webhook delivery
func (w *WebhookSender) processWebhook(job *WebhookJob) error {
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
	return w.deliverWebhook(webhook, merchant.WebhookSecret)
}

// deliverWebhook attempts to deliver a webhook with exponential backoff
func (w *WebhookSender) deliverWebhook(webhook *payment.Webhook, secret string) error {
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
		req, err := http.NewRequestWithContext(w.ctx, "POST", webhook.URL, bytes.NewReader(payloadBytes))
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
			case <-w.ctx.Done():
				return fmt.Errorf("webhook delivery cancelled")
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

// RetryWorker processes failed webhooks from the retry queue
type RetryWorker struct {
	db            *tabletheory.DB
	webhookSender *WebhookSender
	stop          chan struct{}
	interval      time.Duration
}

// NewRetryWorker creates a new retry worker
func NewRetryWorker(db *tabletheory.DB, sender *WebhookSender, interval time.Duration) *RetryWorker {
	return &RetryWorker{
		db:            db,
		webhookSender: sender,
		interval:      interval,
		stop:          make(chan struct{}),
	}
}

// Start begins processing the retry queue
func (r *RetryWorker) Start() {
	go r.run()
}

// Stop gracefully shuts down the retry worker
func (r *RetryWorker) Stop() {
	close(r.stop)
}

// run processes the retry queue
func (r *RetryWorker) run() {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.processRetries()
		case <-r.stop:
			return
		}
	}
}

// processRetries finds and retries failed webhooks
func (r *RetryWorker) processRetries() {
	var webhooks []*payment.Webhook

	// Find webhooks ready for retry
	err := r.db.Model(&payment.Webhook{}).
		Index("gsi-retry").
		Where("NextRetry", "<=", time.Now()).
		Where("Status", "=", payment.WebhookStatusFailed).
		Limit(100).
		All(&webhooks)

	if err != nil {
		fmt.Printf("Failed to query retry webhooks: %v\n", err)
		return
	}

	// Process each webhook
	for _, webhook := range webhooks {
		// Get merchant secret
		var merchant payment.Merchant
		err := r.db.Model(&payment.Merchant{}).
			Where("ID", "=", webhook.MerchantID).
			First(&merchant)

		if err != nil {
			continue
		}

		// Retry delivery
		if err := r.webhookSender.deliverWebhook(webhook, merchant.WebhookSecret); err != nil {
			log.Printf("failed to redeliver webhook %s: %v", webhook.ID, err)
		}
	}
}
