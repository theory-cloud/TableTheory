package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/google/uuid"

	"github.com/theory-cloud/tabletheory"
	payment "github.com/theory-cloud/tabletheory/examples/payment"
	"github.com/theory-cloud/tabletheory/examples/payment/utils"
	"github.com/theory-cloud/tabletheory/pkg/core"
)

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

// Handler processes payment requests
type Handler struct {
	db            core.ExtendedDB
	idempotency   *utils.IdempotencyMiddleware
	webhookSender *utils.WebhookSender
	jwtValidator  *utils.SimpleJWTValidator
}

// NewHandler creates a new payment handler
func NewHandler() (*Handler, error) {
	db, err := theorydb.New(theorydb.Config{
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

	// Initialize idempotency middleware
	idempotency := utils.NewIdempotencyMiddleware(db, 24*time.Hour)

	// Initialize webhook sender with 5 workers
	webhookSender := utils.NewWebhookSender(db, 5)

	// Initialize JWT validator
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "your-secret-key" // Use environment variable in production
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
	}, nil
}

// HandleRequest processes the payment request
func (h *Handler) HandleRequest(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// Extract merchant ID from JWT claims
	merchantID, err := h.extractMerchantID(request.Headers)
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

	// Create idempotency key if not provided
	if req.IdempotencyKey == "" {
		req.IdempotencyKey = generateIdempotencyKey(merchantID, &req)
	}

	// Process with idempotency
	result, err := h.idempotency.Process(ctx, merchantID, req.IdempotencyKey, func() (any, error) {
		return h.processPayment(ctx, merchantID, &req)
	})

	if err != nil {
		if err == utils.ErrDuplicateRequest {
			// Return cached response
			if cached, ok := result.(*payment.Payment); ok {
				return successResponse(http.StatusOK, cached), nil
			}
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
func (h *Handler) processPayment(ctx context.Context, merchantID string, req *ProcessPaymentRequest) (*payment.Payment, error) {
	// Create payment record first
	paymentRecord := &payment.Payment{
		ID:             uuid.New().String(),
		IdempotencyKey: req.IdempotencyKey,
		MerchantID:     merchantID,
		Amount:         req.Amount,
		Currency:       req.Currency,
		Status:         payment.PaymentStatusPending,
		PaymentMethod:  req.PaymentMethod,
		CustomerID:     req.CustomerID,
		Description:    req.Description,
		Metadata:       req.Metadata,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Version:        1,
	}

	// Create transaction record
	txRecord := &payment.Transaction{
		ID:          uuid.New().String(),
		PaymentID:   paymentRecord.ID,
		Type:        payment.TransactionTypeCapture,
		Amount:      req.Amount,
		Status:      "pending",
		ProcessedAt: time.Now(),
		AuditTrail:  []payment.AuditEntry{},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Version:     1,
	}

	// Add initial audit entry
	txRecord.AuditTrail = append(txRecord.AuditTrail, payment.AuditEntry{
		Timestamp: time.Now(),
		Action:    "transaction_created",
		Changes: map[string]any{
			"type":   payment.TransactionTypeCapture,
			"amount": req.Amount,
		},
	})

	// Begin transaction
	err := h.db.Transaction(func(tx *core.Tx) error {
		if err := tx.Create(paymentRecord); err != nil {
			return fmt.Errorf("failed to create payment: %w", err)
		}

		if err := tx.Create(txRecord); err != nil {
			return fmt.Errorf("failed to create transaction: %w", err)
		}

		// Simulate payment processing
		// In real implementation, this would call the payment processor
		paymentRecord.Status = payment.PaymentStatusSucceeded
		paymentRecord.UpdatedAt = time.Now()

		txRecord.Status = "succeeded"
		txRecord.ProcessorID = "PROC-" + uuid.New().String()
		txRecord.ResponseCode = "00"
		txRecord.ResponseText = "Approved"
		txRecord.UpdatedAt = time.Now()

		if err := tx.Update(paymentRecord, "Status", "UpdatedAt"); err != nil {
			return fmt.Errorf("failed to update payment: %w", err)
		}

		if err := tx.Update(txRecord, "Status", "ProcessorID", "ResponseCode", "ResponseText", "UpdatedAt"); err != nil {
			return fmt.Errorf("failed to update transaction: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to process payment: %w", err)
	}

	// Send webhook notification asynchronously
	webhookJob := &utils.WebhookJob{
		MerchantID: merchantID,
		EventType:  "payment.succeeded",
		PaymentID:  paymentRecord.ID,
		Data:       paymentRecord,
	}

	// Queue webhook for async delivery (non-blocking)
	go func() {
		if err := h.webhookSender.Send(webhookJob); err != nil {
			// Log error but don't fail the payment
			fmt.Printf("Failed to queue webhook: %v\n", err)
		}
	}()

	return paymentRecord, nil
}

// Helper functions

func (h *Handler) extractMerchantID(headers map[string]string) (string, error) {
	// Extract from Authorization header
	auth := headers["Authorization"]
	if auth == "" {
		auth = headers["authorization"]
	}

	// Use the JWT validator to extract merchant ID
	return utils.ValidateAndExtractMerchantID(auth, h.jwtValidator)
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

	// Ensure webhook sender is stopped gracefully
	defer handler.webhookSender.Stop()

	lambda.Start(handler.HandleRequest)
}
