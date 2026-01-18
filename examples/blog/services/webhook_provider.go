package services

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// WebhookConfig holds webhook provider configuration
type WebhookConfig struct {
	DefaultWebhookURL string
	SigningSecret     string
	Timeout           time.Duration
	RetryAttempts     int
	TestMode          bool // If true, logs webhooks instead of sending
}

// WebhookProvider implements NotificationProvider for webhook notifications
type WebhookProvider struct {
	client *http.Client
	config WebhookConfig
}

// NewWebhookProvider creates a new webhook provider
func NewWebhookProvider(config WebhookConfig) *WebhookProvider {
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	if config.RetryAttempts == 0 {
		config.RetryAttempts = 3
	}

	return &WebhookProvider{
		config: config,
		client: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// WebhookPayload represents the webhook payload structure
type WebhookPayload struct {
	Timestamp time.Time      `json:"timestamp"`
	Data      map[string]any `json:"data"`
	ID        string         `json:"id"`
	Type      string         `json:"type"`
}

// Send sends a webhook notification
func (p *WebhookProvider) Send(ctx context.Context, notification *Notification) error {
	webhookURL := notification.Recipient.Webhook
	if webhookURL == "" {
		webhookURL = p.config.DefaultWebhookURL
	}
	if webhookURL == "" {
		return fmt.Errorf("webhook URL is required")
	}

	// Prepare webhook payload
	payload := WebhookPayload{
		ID:        notification.ID,
		Type:      string(notification.Type),
		Timestamp: time.Now(),
		Data:      notification.Data,
	}

	// Marshal payload
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	// In test mode, just log the webhook
	if p.config.TestMode {
		log.Printf("TEST MODE - Webhook notification:\nURL: %s\nPayload: %s\n",
			webhookURL,
			string(jsonPayload))
		return nil
	}

	// Generate signature if secret is configured
	var signature string
	if p.config.SigningSecret != "" {
		signature = p.generateSignature(jsonPayload)
	}

	// Send webhook with retries
	var lastErr error
	for attempt := 0; attempt < p.config.RetryAttempts; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			time.Sleep(time.Duration(attempt) * time.Second)
		}

		err = p.sendWebhook(ctx, webhookURL, jsonPayload, signature)
		if err == nil {
			log.Printf("Webhook sent successfully to %s (attempt %d)", webhookURL, attempt+1)
			return nil
		}

		lastErr = err
		log.Printf("Failed to send webhook to %s (attempt %d): %v", webhookURL, attempt+1, err)
	}

	return fmt.Errorf("failed to send webhook after %d attempts: %w", p.config.RetryAttempts, lastErr)
}

// sendWebhook sends a single webhook request
func (p *WebhookProvider) sendWebhook(ctx context.Context, url string, payload []byte, signature string) error {
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "TableTheory-Blog-Webhook/1.0")

	if signature != "" {
		req.Header.Set("X-Webhook-Signature", signature)
	}

	// Send request
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	// Read response body
	body, _ := io.ReadAll(resp.Body)

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// generateSignature generates HMAC SHA256 signature for the payload
func (p *WebhookProvider) generateSignature(payload []byte) string {
	h := hmac.New(sha256.New, []byte(p.config.SigningSecret))
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}

// CanHandle checks if this provider can handle the notification
func (p *WebhookProvider) CanHandle(notification *Notification) bool {
	// Can handle if there's a webhook URL or default webhook is configured
	return notification.Recipient.Webhook != "" || p.config.DefaultWebhookURL != ""
}

// Name returns the provider name
func (p *WebhookProvider) Name() string {
	return "WebhookProvider"
}
