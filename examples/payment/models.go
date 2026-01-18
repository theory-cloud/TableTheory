package payment

import (
	"time"
)

// Payment represents a payment transaction with idempotency support
type Payment struct {
	UpdatedAt      time.Time         `theorydb:"updated_at" json:"updated_at"`
	CreatedAt      time.Time         `theorydb:"created_at" json:"created_at"`
	Metadata       map[string]string `theorydb:"json" json:"metadata,omitempty"`
	PaymentMethod  string            `json:"payment_method"`
	Currency       string            `json:"currency"`
	Status         string            `theorydb:"index:gsi-merchant,sk,prefix:status" json:"status"`
	ID             string            `theorydb:"pk" json:"id"`
	CustomerID     string            `theorydb:"index:gsi-customer" json:"customer_id,omitempty"`
	Description    string            `json:"description,omitempty"`
	MerchantID     string            `theorydb:"index:gsi-merchant,pk" json:"merchant_id"`
	IdempotencyKey string            `theorydb:"index:gsi-idempotency" json:"idempotency_key"`
	Amount         int64             `json:"amount"`
	Version        int               `theorydb:"version" json:"version"`
}

// PaymentStatus constants
const (
	PaymentStatusPending    = "pending"
	PaymentStatusProcessing = "processing"
	PaymentStatusSucceeded  = "succeeded"
	PaymentStatusFailed     = "failed"
	PaymentStatusCanceled   = "canceled"
)

// Transaction represents a transaction on a payment (capture, refund, void)
type Transaction struct {
	CreatedAt    time.Time    `theorydb:"created_at" json:"created_at"`
	UpdatedAt    time.Time    `theorydb:"updated_at" json:"updated_at"`
	ProcessedAt  time.Time    `json:"processed_at"`
	PaymentID    string       `theorydb:"index:gsi-payment" json:"payment_id"`
	Type         string       `json:"type"`
	Status       string       `json:"status"`
	ProcessorID  string       `json:"processor_id,omitempty"`
	ResponseCode string       `json:"response_code,omitempty"`
	ResponseText string       `json:"response_text,omitempty"`
	ID           string       `theorydb:"pk" json:"id"`
	AuditTrail   []AuditEntry `theorydb:"json" json:"audit_trail"`
	Amount       int64        `json:"amount"`
	Version      int          `theorydb:"version" json:"version"`
}

// TransactionType constants
const (
	TransactionTypeCapture = "capture"
	TransactionTypeRefund  = "refund"
	TransactionTypeVoid    = "void"
)

// Customer represents a customer with PCI-compliant payment methods
type Customer struct {
	CreatedAt      time.Time         `theorydb:"created_at" json:"created_at"`
	UpdatedAt      time.Time         `theorydb:"updated_at" json:"updated_at"`
	Metadata       map[string]string `theorydb:"json" json:"metadata,omitempty"`
	ID             string            `theorydb:"pk" json:"id"`
	MerchantID     string            `theorydb:"index:gsi-merchant,pk" json:"merchant_id"`
	EmailHash      string            `theorydb:"index:gsi-email,pk" json:"email_hash"`
	Email          string            `theorydb:"encrypted" json:"email"`
	Name           string            `theorydb:"encrypted" json:"name"`
	Phone          string            `theorydb:"encrypted" json:"phone,omitempty"`
	DefaultMethod  string            `json:"default_method,omitempty"`
	PaymentMethods []PaymentMethod   `theorydb:"json,encrypted:pci" json:"payment_methods"`
	Version        int               `theorydb:"version" json:"version"`
}

// PaymentMethod represents a customer's payment method
type PaymentMethod struct {
	CreatedAt   time.Time `json:"created_at"`
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Last4       string    `json:"last4"`
	Brand       string    `json:"brand,omitempty"`
	BankName    string    `json:"bank_name,omitempty"`
	AccountType string    `json:"account_type,omitempty"`
	Token       string    `json:"-"`
	ExpiryMonth int       `json:"expiry_month,omitempty"`
	ExpiryYear  int       `json:"expiry_year,omitempty"`
	IsDefault   bool      `json:"is_default"`
}

// Merchant represents a merchant account
type Merchant struct {
	CreatedAt       time.Time      `theorydb:"created_at" json:"created_at"`
	UpdatedAt       time.Time      `theorydb:"updated_at" json:"updated_at"`
	ProcessorConfig map[string]any `theorydb:"json,encrypted" json:"-"`
	ID              string         `theorydb:"pk" json:"id"`
	Name            string         `json:"name"`
	Email           string         `theorydb:"index:gsi-email,pk" json:"email"`
	Status          string         `json:"status"`
	WebhookURL      string         `json:"webhook_url,omitempty"`
	WebhookSecret   string         `theorydb:"encrypted" json:"-"`
	Features        []string       `theorydb:"set" json:"features"`
	RateLimits      RateLimits     `theorydb:"json" json:"rate_limits"`
	Version         int            `theorydb:"version" json:"version"`
}

// RateLimits defines rate limiting configuration
type RateLimits struct {
	PaymentsPerMinute int   `json:"payments_per_minute"`
	PaymentsPerDay    int   `json:"payments_per_day"`
	MaxPaymentAmount  int64 `json:"max_payment_amount"`
}

// AuditEntry represents an entry in the audit trail
type AuditEntry struct {
	Timestamp time.Time      `json:"timestamp"`
	Action    string         `json:"action"`
	UserID    string         `json:"user_id,omitempty"`
	IPAddress string         `json:"ip_address,omitempty"`
	Changes   map[string]any `json:"changes,omitempty"`
	Reason    string         `json:"reason,omitempty"`
}

// IdempotencyRecord tracks idempotent requests
type IdempotencyRecord struct {
	CreatedAt   time.Time `theorydb:"created_at" json:"created_at"`
	Key         string    `theorydb:"pk" json:"key"`
	MerchantID  string    `theorydb:"index:gsi-merchant,pk" json:"merchant_id"`
	RequestHash string    `json:"request_hash"`
	Response    string    `theorydb:"json" json:"response"`
	StatusCode  int       `json:"status_code"`
	ExpiresAt   int64     `theorydb:"ttl" json:"expires_at"`
}

// Settlement represents a batch settlement
type Settlement struct {
	ProcessedAt      time.Time          `json:"processed_at,omitempty"`
	CreatedAt        time.Time          `theorydb:"created_at" json:"created_at"`
	UpdatedAt        time.Time          `theorydb:"updated_at" json:"updated_at"`
	ID               string             `theorydb:"pk" json:"id"`
	MerchantID       string             `theorydb:"index:gsi-merchant,pk" json:"merchant_id"`
	Date             string             `theorydb:"index:gsi-merchant,sk" json:"date"`
	Status           string             `json:"status"`
	BatchID          string             `json:"batch_id"`
	Transactions     []SettlementDetail `theorydb:"json" json:"transactions"`
	TotalAmount      int64              `json:"total_amount"`
	TransactionCount int                `json:"transaction_count"`
}

// SettlementDetail represents a transaction in a settlement
type SettlementDetail struct {
	PaymentID     string `json:"payment_id"`
	TransactionID string `json:"transaction_id"`
	Amount        int64  `json:"amount"`
	Fee           int64  `json:"fee"`
	NetAmount     int64  `json:"net_amount"`
}

// Webhook represents a webhook delivery attempt
type Webhook struct {
	CreatedAt    time.Time      `theorydb:"created_at" json:"created_at"`
	NextRetry    time.Time      `theorydb:"index:gsi-retry" json:"next_retry,omitempty"`
	LastAttempt  time.Time      `json:"last_attempt,omitempty"`
	Payload      map[string]any `theorydb:"json" json:"payload"`
	Status       string         `json:"status"`
	URL          string         `json:"url"`
	PaymentID    string         `json:"payment_id,omitempty"`
	EventType    string         `theorydb:"index:gsi-merchant,sk,prefix:event" json:"event_type"`
	ID           string         `theorydb:"pk" json:"id"`
	ResponseBody string         `json:"response_body,omitempty"`
	MerchantID   string         `theorydb:"index:gsi-merchant,pk" json:"merchant_id"`
	Attempts     int            `json:"attempts"`
	ResponseCode int            `json:"response_code,omitempty"`
	ExpiresAt    int64          `theorydb:"ttl" json:"expires_at"`
}

// WebhookStatus constants
const (
	WebhookStatusPending   = "pending"
	WebhookStatusDelivered = "delivered"
	WebhookStatusFailed    = "failed"
	WebhookStatusExpired   = "expired"
)
