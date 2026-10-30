package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/google/uuid"

	"github.com/theory-cloud/tabletheory/v6"
	payment "github.com/theory-cloud/tabletheory/v6/examples/payment"
	"github.com/theory-cloud/tabletheory/v6/examples/payment/utils"
	"github.com/theory-cloud/tabletheory/v6/pkg/core"
	customerrors "github.com/theory-cloud/tabletheory/v6/pkg/errors"
)

var (
	errPaymentPermissionDenied = errors.New("token lacks the payments:write permission")
	errMerchantNotFound        = errors.New("merchant not found")
	errMerchantInactive        = errors.New("merchant is not active")
	errMerchantLimit           = errors.New("merchant limits do not permit this payment")
	errPaymentLimitExceeded    = errors.New("payment rate limit exceeded")
)

// paymentLimitError carries the transient rate-limit refusal. The idempotency
// middleware records HTTPStatus so a replay returns the same 429 instead of a
// generic server error. The stored message stays generic; transaction internals
// are not exposed to the caller.
type paymentLimitError struct{}

func (*paymentLimitError) Error() string { return errPaymentLimitExceeded.Error() }

func (*paymentLimitError) Unwrap() error { return errPaymentLimitExceeded }

func (*paymentLimitError) HTTPStatus() int { return http.StatusTooManyRequests }

// ProcessPaymentRequest represents the payment request payload
type ProcessPaymentRequest struct {
	Metadata       map[string]string `json:"metadata,omitempty"`
	IdempotencyKey string            `json:"idempotency_key"`
	Currency       string            `json:"currency"`
	PaymentMethod  string            `json:"payment_method"`
	CustomerID     string            `json:"customer_id,omitempty"`
	Description    string            `json:"description,omitempty"`
	UserID         string            `json:"user_id"`
	Amount         int64             `json:"amount"`
}

// webhookDispatcher is the payment-to-webhook boundary. It is an interface so
// the handler tests can record intended deliveries without a hosted endpoint.
type webhookDispatcher interface {
	SendSync(ctx context.Context, job *utils.WebhookJob) error
}

// Handler processes payment requests
type Handler struct {
	db            core.ExtendedDB
	idempotency   *utils.IdempotencyMiddleware
	webhookSender webhookDispatcher
	jwtValidator  *utils.SimpleJWTValidator
	now           func() time.Time
}

// NewHandler creates a new payment handler
func NewHandler() (*Handler, error) {
	db, err := tabletheory.New(tabletheory.Config{
		Region: os.Getenv("AWS_REGION"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize DynamoDB: %w", err)
	}

	// Register models
	db.Model(&payment.Payment{})
	db.Model(&payment.IdempotencyRecord{})
	db.Model(&payment.Transaction{})
	db.Model(&payment.AuditEntry{})
	db.Model(&payment.Merchant{})
	db.Model(&payment.Webhook{})
	db.Model(&payment.UsageCounter{})

	// Initialize idempotency middleware
	idempotency := utils.NewIdempotencyMiddleware(db, 24*time.Hour)

	// Initialize the webhook sender. It starts no goroutine: deliveries run
	// synchronously on the invocation path.
	webhookSender := utils.NewWebhookSender(db)

	// Initialize JWT validator
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET environment variable is required")
	}
	jwtValidator := utils.NewSimpleJWTValidator(
		jwtSecret,
		os.Getenv("JWT_ISSUER"),
		os.Getenv("JWT_AUDIENCE"),
	)

	return &Handler{
		db:            db,
		idempotency:   idempotency,
		webhookSender: webhookSender,
		jwtValidator:  jwtValidator,
		now:           time.Now,
	}, nil
}

// HandleRequest processes the payment request
func (h *Handler) HandleRequest(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// Extract and validate the JWT claims. The merchant ID alone is not enough:
	// the token must also carry the permission that allows a payment write.
	claims, err := h.extractClaims(request.Headers)
	if err != nil {
		return errorResponse(http.StatusUnauthorized, "Invalid authentication"), nil
	}

	// Parse request body
	var req ProcessPaymentRequest
	if err := json.Unmarshal([]byte(request.Body), &req); err != nil {
		return errorResponse(http.StatusBadRequest, "Invalid request body"), nil
	}

	// Validate request
	if err := validatePaymentRequest(&req); err != nil {
		return errorResponse(http.StatusBadRequest, err.Error()), nil
	}

	// Authorize the merchant before any idempotency reservation or payment
	// write. A refusal at this point leaves no payment, transaction, counter,
	// webhook, or idempotency record behind.
	merchant, err := h.authorizeMerchant(claims, req.Amount)
	if err != nil {
		if isPolicyDenial(err) {
			return errorResponse(http.StatusForbidden, "Payment not permitted"), nil
		}
		return errorResponse(http.StatusInternalServerError, "Payment processing failed"), nil
	}

	// Create idempotency key if not provided
	if req.IdempotencyKey == "" {
		req.IdempotencyKey = generateIdempotencyKey(merchant.ID, &req)
	}

	// Process with idempotency
	result, err := h.idempotency.Process(ctx, merchant.ID, req.IdempotencyKey, func() (any, error) {
		return h.processPayment(ctx, merchant, &req)
	})

	if err != nil {
		if errors.Is(err, utils.ErrDuplicateRequest) {
			// Replay the stored outcome without executing the payment again.
			if replay, ok := result.(*utils.Replay); ok {
				statusCode := replay.StatusCode
				if statusCode < 200 || statusCode > 599 {
					statusCode = http.StatusInternalServerError
				}
				if statusCode < 400 && replayBodyHasError(replay.Body) {
					statusCode = http.StatusInternalServerError
				}
				if statusCode >= 200 && statusCode < 300 {
					return successResponse(http.StatusOK, replay.Body), nil
				}
				return errorResponse(statusCode, replayErrorMessage(replay.Body)), nil
			}
			return errorResponse(http.StatusConflict, "Request is still being processed"), nil
		}
		if errors.Is(err, errPaymentLimitExceeded) {
			return errorResponse(http.StatusTooManyRequests, "Payment limit exceeded"), nil
		}
		return errorResponse(http.StatusInternalServerError, "Payment processing failed"), nil
	}

	// Return success response
	if payment, ok := result.(*payment.Payment); ok {
		return successResponse(http.StatusCreated, payment), nil
	}

	return errorResponse(http.StatusInternalServerError, "Unexpected error"), nil
}

// processPayment handles the actual payment processing
func (h *Handler) processPayment(ctx context.Context, merchant *payment.Merchant, req *ProcessPaymentRequest) (*payment.Payment, error) {
	now := h.currentTime()

	// Create payment record first
	paymentRecord := &payment.Payment{
		ID:             uuid.New().String(),
		IdempotencyKey: req.IdempotencyKey,
		MerchantID:     merchant.ID,
		Amount:         req.Amount,
		Currency:       req.Currency,
		Status:         payment.PaymentStatusPending,
		PaymentMethod:  req.PaymentMethod,
		CustomerID:     req.CustomerID,
		Description:    req.Description,
		Metadata:       req.Metadata,
		CreatedAt:      now,
		UpdatedAt:      now,
		Version:        1,
	}

	// Create transaction record
	txRecord := &payment.Transaction{
		ID:          uuid.New().String(),
		PaymentID:   paymentRecord.ID,
		Type:        payment.TransactionTypeCapture,
		Amount:      req.Amount,
		Status:      "pending",
		ProcessedAt: now,
		AuditTrail:  []payment.AuditEntry{},
		CreatedAt:   now,
		UpdatedAt:   now,
		Version:     1,
	}

	// Add initial audit entry
	txRecord.AuditTrail = append(txRecord.AuditTrail, payment.AuditEntry{
		Timestamp: now,
		Action:    "transaction_created",
		Changes: map[string]any{
			"type":   payment.TransactionTypeCapture,
			"amount": req.Amount,
		},
	})

	// Simulate payment processing before composing the atomic writes.
	// In real implementation, this would call the payment processor.
	paymentRecord.Status = payment.PaymentStatusSucceeded
	paymentRecord.UpdatedAt = now

	txRecord.Status = "succeeded"
	txRecord.ProcessorID = "PROC-" + uuid.New().String()
	txRecord.ResponseCode = "00"
	txRecord.ResponseText = "Approved"
	txRecord.UpdatedAt = now

	// The payment, its transaction, and both usage counters share one
	// transaction. A conditional failure on either counter rolls back every
	// write, so concurrent requests cannot overshoot the configured limit and a
	// refused request leaves no payment or counter increment behind.
	minuteCounter := payment.NewUsageCounter(merchant.ID, payment.UsageWindowMinute, now)
	dayCounter := payment.NewUsageCounter(merchant.ID, payment.UsageWindowDay, now)

	err := h.db.Transact().
		WithContext(ctx).
		Create(paymentRecord).
		Create(txRecord).
		UpdateWithBuilder(
			minuteCounter,
			usageIncrement(minuteCounter),
			usageLimitCondition(merchant.RateLimits.PaymentsPerMinute),
		).
		UpdateWithBuilder(
			dayCounter,
			usageIncrement(dayCounter),
			usageLimitCondition(merchant.RateLimits.PaymentsPerDay),
		).
		Execute()

	if err != nil {
		if errors.Is(err, customerrors.ErrConditionFailed) {
			return nil, &paymentLimitError{}
		}
		return nil, fmt.Errorf("failed to process payment: %w", err)
	}

	// Send the webhook notification for this payment.
	webhookJob := &utils.WebhookJob{
		MerchantID: merchant.ID,
		EventType:  "payment.succeeded",
		PaymentID:  paymentRecord.ID,
		Data:       paymentRecord,
	}

	// Deliver the webhook synchronously inside this invocation. Lambda freezes
	// the execution environment the moment the handler returns, so a delivery
	// launched in a goroutine here would be frozen mid-flight and could resume
	// against an invocation that is already over. SendSync is bounded by ctx and
	// a short internal timeout, and its failure must not fail the payment: the
	// payment is already committed and the webhook record is persisted with a
	// retry schedule.
	if err := h.webhookSender.SendSync(ctx, webhookJob); err != nil {
		fmt.Printf("Failed to deliver webhook: %v\n", err)
	}

	return paymentRecord, nil
}

// Helper functions

func (h *Handler) extractClaims(headers map[string]string) (*utils.JWTClaims, error) {
	// Extract from Authorization header
	auth := headers["Authorization"]
	if auth == "" {
		auth = headers["authorization"]
	}

	return utils.ValidateAndExtractClaims(auth, h.jwtValidator)
}

// authorizeMerchant enforces the example's merchant policy. Permission, status,
// and every configured limit are checked before the caller reserves an
// idempotency key or writes payment state. Zero or negative limits are treated
// as disabled, so they deny positive payments instead of meaning "unlimited".
func (h *Handler) authorizeMerchant(claims *utils.JWTClaims, amount int64) (*payment.Merchant, error) {
	if !hasExactPermission(claims.Permissions, payment.PermissionPaymentsWrite) {
		return nil, errPaymentPermissionDenied
	}

	var merchant payment.Merchant
	err := h.db.Model(&payment.Merchant{}).
		Where("ID", "=", claims.MerchantID).
		First(&merchant)
	if err != nil {
		if errors.Is(err, customerrors.ErrItemNotFound) {
			return nil, errMerchantNotFound
		}
		return nil, fmt.Errorf("load merchant: %w", err)
	}

	if merchant.Status != payment.MerchantStatusActive {
		return nil, errMerchantInactive
	}

	limits := merchant.RateLimits
	if limits.MaxPaymentAmount <= 0 {
		return nil, fmt.Errorf("%w: max_payment_amount must be positive", errMerchantLimit)
	}
	if amount > limits.MaxPaymentAmount {
		return nil, fmt.Errorf("%w: amount exceeds max_payment_amount", errMerchantLimit)
	}
	if limits.PaymentsPerMinute <= 0 {
		return nil, fmt.Errorf("%w: payments_per_minute must be positive", errMerchantLimit)
	}
	if limits.PaymentsPerDay <= 0 {
		return nil, fmt.Errorf("%w: payments_per_day must be positive", errMerchantLimit)
	}

	return &merchant, nil
}

// hasExactPermission requires the exact permission string. Prefixes, casing
// variants, and similarly named permissions are not accepted.
func hasExactPermission(permissions []string, required string) bool {
	for _, permission := range permissions {
		if permission == required {
			return true
		}
	}
	return false
}

func isPolicyDenial(err error) bool {
	return errors.Is(err, errPaymentPermissionDenied) ||
		errors.Is(err, errMerchantNotFound) ||
		errors.Is(err, errMerchantInactive) ||
		errors.Is(err, errMerchantLimit)
}

func (h *Handler) currentTime() time.Time {
	if h.now != nil {
		return h.now()
	}
	return time.Now()
}

// usageIncrement adds one payment to a window counter and refreshes its
// descriptive attributes. Version is incremented atomically with the count so
// the stored row shows how many times the counter has advanced.
func usageIncrement(counter *payment.UsageCounter) func(core.UpdateBuilder) error {
	return func(ub core.UpdateBuilder) error {
		ub.Add("Count", int64(1))
		ub.Add("Version", int64(1))
		ub.Set("MerchantID", counter.MerchantID)
		ub.Set("WindowKind", counter.WindowKind)
		ub.Set("WindowStart", counter.WindowStart)
		ub.Set("ExpiresAt", counter.ExpiresAt)
		return nil
	}
}

// usageLimitCondition is the atomic guard for one counter. A missing counter
// starts at zero; an existing counter must still be below the limit.
func usageLimitCondition(limit int) core.TransactCondition {
	return tabletheory.ConditionExpression(
		"attribute_not_exists(usageCount) OR usageCount < :usage_limit",
		map[string]any{":usage_limit": int64(limit)},
	)
}

func replayBodyHasError(body any) bool {
	values, ok := body.(map[string]any)
	if !ok {
		return false
	}
	_, hasError := values["error"]
	return hasError
}

func replayErrorMessage(body any) string {
	if values, ok := body.(map[string]any); ok {
		if message, ok := values["error"].(string); ok && message != "" {
			return message
		}
	}
	return "Payment processing failed"
}

func validatePaymentRequest(req *ProcessPaymentRequest) error {
	if req.Amount <= 0 {
		return fmt.Errorf("amount must be greater than 0")
	}
	if req.Currency == "" {
		return fmt.Errorf("currency is required")
	}
	if req.PaymentMethod == "" {
		return fmt.Errorf("payment_method is required")
	}
	return nil
}

func generateIdempotencyKey(merchantID string, req *ProcessPaymentRequest) string {
	data := fmt.Sprintf("%s:%d:%s:%s:%d",
		merchantID,
		req.Amount,
		req.Currency,
		req.PaymentMethod,
		time.Now().Unix(),
	)
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash)
}

func successResponse(statusCode int, data any) events.APIGatewayProxyResponse {
	body, _ := json.Marshal(map[string]any{
		"success": true,
		"data":    data,
	})

	return events.APIGatewayProxyResponse{
		StatusCode: statusCode,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}
}

func errorResponse(statusCode int, message string) events.APIGatewayProxyResponse {
	body, _ := json.Marshal(map[string]any{
		"success": false,
		"error":   message,
	})

	return events.APIGatewayProxyResponse{
		StatusCode: statusCode,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}
}

func main() {
	handler, err := NewHandler()
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize handler: %v", err))
	}

	lambda.Start(handler.HandleRequest)
}
