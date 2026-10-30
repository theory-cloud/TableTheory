package utils

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"syscall"
	"time"

	"github.com/google/uuid"

	"github.com/theory-cloud/tabletheory/v6/examples/payment"
	"github.com/theory-cloud/tabletheory/v6/pkg/core"
)

// errUnsafeWebhookDestination marks a destination that may not receive a
// webhook request. It is permanent: retrying the same URL cannot make a
// non-HTTPS or non-public target safe.
var errUnsafeWebhookDestination = errors.New("unsafe webhook destination")

// nonPublicPrefixes lists address ranges that are not globally routable even
// though the net.IP helpers do not classify all of them as private. The list is
// intentionally conservative: webhook destinations must be public HTTPS.
var nonPublicPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("2001:db8::/32"),
}

// WebhookSender delivers webhooks synchronously inside the calling invocation.
// It starts no goroutine and owns no queue: Lambda freezes the execution
// environment as soon as the handler returns, so a delivery parked on an
// in-process channel would be frozen with it. Durable hand-off belongs in a
// queue or stream that outlives the invocation on purpose, not in process memory.
type WebhookSender struct {
	db       core.ExtendedDB
	client   *http.Client
	sendWait time.Duration
	lookupIP func(ctx context.Context, host string) ([]net.IPAddr, error)
	dial     func(ctx context.Context, network, address string) (net.Conn, error)
}

// NewWebhookSender creates a new webhook sender. Every connection resolves the
// destination, rejects any non-public answer, dials the validated address, and
// checks the address again at the socket boundary. Redirects pass through the
// same policy before the redirected request is sent.
func NewWebhookSender(db core.ExtendedDB) *WebhookSender {
	sender := &WebhookSender{
		db:       db,
		sendWait: 10 * time.Second,
		lookupIP: net.DefaultResolver.LookupIPAddr,
	}

	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
		Control:   rejectNonPublicDialAddress,
	}
	sender.dial = dialer.DialContext
	sender.client = &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			// No environment proxy: a proxy would move the connection away
			// from the validated address.
			Proxy:             nil,
			DialContext:       sender.dialPublic,
			TLSClientConfig:   &tls.Config{MinVersion: tls.VersionTLS12},
			ForceAttemptHTTP2: true,
			MaxIdleConns:      10,
			IdleConnTimeout:   30 * time.Second,
		},
		CheckRedirect: sender.checkRedirect,
	}

	return sender
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

	// Refuse an unsafe destination before persisting delivery work. Delivery
	// repeats these checks on every attempt and redirect.
	target, err := w.validateDestination(ctx, merchant.WebhookURL)
	if err != nil {
		return fmt.Errorf("refusing webhook destination: %w", err)
	}

	// Create webhook record
	webhook := &payment.Webhook{
		ID:         uuid.New().String(),
		MerchantID: job.MerchantID,
		EventType:  job.EventType,
		PaymentID:  job.PaymentID,
		URL:        target.String(),
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

// deliverWebhook attempts to deliver a webhook with exponential backoff.
// Destination validation runs before every attempt, so a DNS answer that
// changes after the record was created still cannot receive a request.
func (w *WebhookSender) deliverWebhook(ctx context.Context, webhook *payment.Webhook, secret string) error {
	maxAttempts := 5
	baseDelay := 1 * time.Second

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		webhook.Attempts = attempt
		webhook.LastAttempt = time.Now()

		if _, err := w.validateDestination(ctx, webhook.URL); err != nil {
			webhook.Status = payment.WebhookStatusFailed
			webhook.ResponseBody = err.Error()
			if updateErr := w.db.Model(webhook).Update("Attempts", "LastAttempt", "Status", "ResponseBody"); updateErr != nil {
				return fmt.Errorf("failed to record refused webhook destination: %w", updateErr)
			}
			return err
		}

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
			if errors.Is(err, errUnsafeWebhookDestination) {
				webhook.Status = payment.WebhookStatusFailed
				webhook.ResponseBody = fmt.Sprintf("Destination error: %v", err)
				if updateErr := w.db.Model(webhook).Update("Attempts", "LastAttempt", "Status", "ResponseBody"); updateErr != nil {
					return fmt.Errorf("failed to record refused webhook destination: %w", updateErr)
				}
				return err
			}

			webhook.Status = payment.WebhookStatusFailed
			webhook.ResponseBody = fmt.Sprintf("Network error: %v", err)
		} else {
			webhook.ResponseCode = resp.StatusCode
			_ = resp.Body.Close()

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

// checkRedirect reapplies the destination policy to every redirect before the
// next request is sent. The transport repeats the DNS and dialed-IP checks when
// it opens the redirected connection.
func (w *WebhookSender) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return unsafeDestination("stopped after 10 redirects")
	}
	if _, err := w.validateDestination(req.Context(), req.URL.String()); err != nil {
		return err
	}
	return nil
}

// validateDestination parses the URL and resolves the host to public addresses.
// It runs before record creation, before every delivery attempt, and on every
// redirect. Resolution failure is fail-closed: no bytes leave the process.
func (w *WebhookSender) validateDestination(ctx context.Context, raw string) (*url.URL, error) {
	target, err := parseWebhookURL(raw)
	if err != nil {
		return nil, err
	}
	if _, err := w.resolvePublic(ctx, target.Hostname()); err != nil {
		return nil, err
	}
	return target, nil
}

func parseWebhookURL(raw string) (*url.URL, error) {
	target, err := url.Parse(raw)
	if err != nil {
		return nil, unsafeDestination("parse URL: %v", err)
	}
	if target.Scheme != "https" {
		return nil, unsafeDestination("scheme %q is not HTTPS", target.Scheme)
	}
	if target.Host == "" || target.Hostname() == "" {
		return nil, unsafeDestination("host is required")
	}
	if target.User != nil {
		return nil, unsafeDestination("user information is not allowed")
	}
	if port := target.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return nil, unsafeDestination("invalid port %q", port)
		}
	}
	return target, nil
}

// resolvePublic returns every address for host only when all of them are
// public. A hostname with a mixed answer is rejected rather than filtered, so a
// DNS answer can never smuggle a private target past the policy.
func (w *WebhookSender) resolvePublic(ctx context.Context, host string) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		if !isPublicIP(ip) {
			return nil, unsafeDestination("address %s is not public", ip)
		}
		return []net.IP{ip}, nil
	}

	if w.lookupIP == nil {
		return nil, unsafeDestination("no resolver configured")
	}

	addresses, err := w.lookupIP(ctx, host)
	if err != nil {
		return nil, unsafeDestination("resolve %s: %v", host, err)
	}
	if len(addresses) == 0 {
		return nil, unsafeDestination("resolve %s: no addresses", host)
	}

	public := make([]net.IP, 0, len(addresses))
	for _, address := range addresses {
		if !isPublicIP(address.IP) {
			return nil, unsafeDestination("%s resolves to non-public address %s", host, address.IP)
		}
		public = append(public, address.IP)
	}
	return public, nil
}

// dialPublic resolves the host, dials only a validated address, and never lets
// the resolver run a second time between validation and connection.
func (w *WebhookSender) dialPublic(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}

	addresses, err := w.resolvePublic(ctx, host)
	if err != nil {
		return nil, err
	}
	if w.dial == nil {
		return nil, unsafeDestination("webhook dialer is not configured")
	}

	var lastErr error
	for _, ip := range addresses {
		conn, err := w.dial(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, unsafeDestination("no public address for %s", host)
}

// rejectNonPublicDialAddress is the last check at the socket boundary. It
// catches a dialed address that bypassed the resolver policy, including a
// literal private IP or a zone-qualified link-local address.
func rejectNonPublicDialAddress(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return unsafeDestination("dial address %q is not an IP address", address)
	}
	if !isPublicAddr(addr) {
		return unsafeDestination("dialed address %s is not public", address)
	}
	return nil
}

func isPublicIP(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	return isPublicAddr(addr)
}

func isPublicAddr(addr netip.Addr) bool {
	if !addr.IsValid() {
		return false
	}
	addr = addr.Unmap()

	if addr.IsLoopback() ||
		addr.IsPrivate() ||
		addr.IsUnspecified() ||
		addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() ||
		addr.IsMulticast() {
		return false
	}

	for _, prefix := range nonPublicPrefixes {
		if prefix.Contains(addr) {
			return false
		}
	}
	return true
}

func unsafeDestination(format string, args ...any) error {
	return fmt.Errorf("%w: %s", errUnsafeWebhookDestination, fmt.Sprintf(format, args...))
}
