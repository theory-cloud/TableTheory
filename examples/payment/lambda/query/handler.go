package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/theory-cloud/tabletheory/pkg/core"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/theory-cloud/tabletheory"
	payment "github.com/theory-cloud/tabletheory/examples/payment"
	"github.com/theory-cloud/tabletheory/examples/payment/utils"
	customerrors "github.com/theory-cloud/tabletheory/pkg/errors"
)

// QueryRequest represents the query parameters
type QueryRequest struct {
	Status     string `json:"status,omitempty"`
	StartDate  string `json:"start_date,omitempty"`
	EndDate    string `json:"end_date,omitempty"`
	CustomerID string `json:"customer_id,omitempty"`
	Cursor     string `json:"cursor,omitempty"`
	MinAmount  int64  `json:"min_amount,omitempty"`
	MaxAmount  int64  `json:"max_amount,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}

// QueryResponse represents the paginated response
type QueryResponse struct {
	NextCursor string             `json:"next_cursor,omitempty"`
	Payments   []*payment.Payment `json:"payments"`
	Total      int                `json:"total"`
	HasMore    bool               `json:"has_more"`
}

// PaymentSummary provides aggregated statistics
type PaymentSummary struct {
	ByStatus      map[string]int   `json:"by_status"`
	ByCurrency    map[string]int64 `json:"by_currency"`
	TotalAmount   int64            `json:"total_amount"`
	TotalCount    int              `json:"total_count"`
	AverageAmount int64            `json:"average_amount"`
}

// ExportJob represents an export job in the queue
type ExportJob struct {
	CreatedAt  time.Time              `theorydb:"created_at" json:"created_at"`
	UpdatedAt  time.Time              `theorydb:"updated_at" json:"updated_at"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	ID         string                 `theorydb:"pk" json:"id"`
	MerchantID string                 `theorydb:"index:gsi-merchant,pk" json:"merchant_id"`
	Status     string                 `theorydb:"index:gsi-status,pk" json:"status"`
	Format     string                 `json:"format"`
	ResultURL  string                 `json:"result_url,omitempty"`
	Error      string                 `json:"error,omitempty"`
	Query      QueryRequest           `json:"query"`
	ExpiresAt  int64                  `theorydb:"ttl" json:"expires_at"`
}

// QueryHandler handles payment query requests
type QueryHandler struct {
	db           core.ExtendedDB
	jwtValidator *utils.SimpleJWTValidator
}

// NewHandler creates a new query handler
func NewHandler() (*QueryHandler, error) {
	db, err := tabletheory.New(tabletheory.Config{
		Region: os.Getenv("AWS_REGION"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize DynamoDB: %w", err)
	}

	// Register models
	db.Model(&payment.Payment{})
	db.Model(&payment.Customer{})
	db.Model(&payment.Transaction{})
	db.Model(&payment.AuditEntry{})
	db.Model(&ExportJob{})

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

	return &QueryHandler{
		db:           db,
		jwtValidator: jwtValidator,
	}, nil
}

// HandleRequest processes query requests
func (h *QueryHandler) HandleRequest(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// Extract merchant ID from JWT
	merchantID, err := h.extractMerchantID(request.Headers)
	if err != nil {
		return errorResponse(http.StatusUnauthorized, "Invalid authentication"), nil
	}

	// Parse query parameters
	query := parseQueryParams(request.QueryStringParameters)

	// Handle different endpoints
	switch request.Path {
	case "/payments":
		return h.queryPayments(ctx, merchantID, query)
	case "/payments/summary":
		return h.getPaymentSummary(ctx, merchantID, query)
	case "/payments/export":
		return h.exportPayments(ctx, merchantID, query)
	default:
		if strings.HasPrefix(request.Path, "/payments/") {
			// Get single payment
			paymentID := strings.TrimPrefix(request.Path, "/payments/")
			return h.getPayment(ctx, merchantID, paymentID)
		}
		return errorResponse(http.StatusNotFound, "Endpoint not found"), nil
	}
}

// queryPayments returns paginated payment results
func (h *QueryHandler) queryPayments(_ context.Context, merchantID string, req *QueryRequest) (events.APIGatewayProxyResponse, error) {
	// Build query
	query := h.db.Model(&payment.Payment{}).
		Index("gsi-merchant").
		Where("MerchantID", "=", merchantID).
		OrderBy("CreatedAt", "DESC").
		Limit(req.Limit)

	// Apply status filter if provided
	if req.Status != "" {
		query = query.Filter("Status", "=", req.Status)
	}

	// Apply date range filter
	if req.StartDate != "" || req.EndDate != "" {
		if req.StartDate != "" && req.EndDate != "" {
			query = query.Filter("CreatedAt", "BETWEEN", []string{req.StartDate, req.EndDate})
		} else if req.StartDate != "" {
			query = query.Filter("CreatedAt", ">=", req.StartDate)
		} else {
			query = query.Filter("CreatedAt", "<=", req.EndDate)
		}
	}

	// Apply customer filter if provided
	if req.CustomerID != "" {
		query = query.Filter("CustomerID", "=", req.CustomerID)
	}

	// Apply amount filters if provided
	if req.MinAmount > 0 {
		query = query.Filter("Amount", ">=", req.MinAmount)
	}

	if req.MaxAmount > 0 {
		query = query.Filter("Amount", "<=", req.MaxAmount)
	}

	// Execute query
	var payments []*payment.Payment
	err := query.All(&payments)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, "Failed to query payments"), nil
	}

	// Calculate total count (would require a separate count query in production)
	totalCount := len(payments)

	// Build response
	response := QueryResponse{
		Payments: payments,
		Total:    totalCount,
		HasMore:  len(payments) == req.Limit,
	}

	return successResponse(http.StatusOK, response), nil
}

// getPayment returns a single payment
func (h *QueryHandler) getPayment(_ context.Context, merchantID, paymentID string) (events.APIGatewayProxyResponse, error) {
	// Get payment details
	var paymentRecord payment.Payment
	err := h.db.Model(&payment.Payment{}).
		Where("ID", "=", paymentID).
		First(&paymentRecord)

	if err != nil {
		if err == customerrors.ErrItemNotFound {
			return errorResponse(http.StatusNotFound, "Payment not found"), nil
		}
		return errorResponse(http.StatusInternalServerError, "Failed to fetch payment"), nil
	}

	// Verify payment belongs to merchant
	if paymentRecord.MerchantID != merchantID {
		return errorResponse(http.StatusNotFound, "Payment not found"), nil
	}

	// Get all transactions for this payment
	var transactions []*payment.Transaction
	err = h.db.Model(&payment.Transaction{}).
		Index("gsi-payment").
		Where("PaymentID", "=", paymentID).
		OrderBy("CreatedAt", "DESC").
		All(&transactions)

	if err != nil {
		return errorResponse(http.StatusInternalServerError, "Failed to fetch transactions"), nil
	}

	// Build detailed response
	response := map[string]any{
		"payment":      paymentRecord,
		"transactions": transactions,
	}

	return successResponse(http.StatusOK, response), nil
}

// getPaymentSummary returns aggregated statistics
func (h *QueryHandler) getPaymentSummary(_ context.Context, merchantID string, req *QueryRequest) (events.APIGatewayProxyResponse, error) {
	// For large datasets, this would typically use DynamoDB Streams + aggregation
	// For demo purposes, we'll do a simplified version

	var payments []*payment.Payment
	query := h.db.Model(&payment.Payment{}).
		Index("gsi-merchant").
		Where("MerchantID", "=", merchantID)

	// Apply date filters for summary
	if req.StartDate != "" {
		startTime, _ := time.Parse("2006-01-02", req.StartDate)
		query = query.Where("CreatedAt", ">=", startTime)
	}

	if req.EndDate != "" {
		endTime, _ := time.Parse("2006-01-02", req.EndDate)
		endTime = endTime.Add(24 * time.Hour)
		query = query.Where("CreatedAt", "<", endTime)
	}

	// Scan all payments (in production, use aggregation tables)
	err := query.Scan(&payments)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, "Failed to calculate summary"), nil
	}

	// Calculate summary
	summary := &PaymentSummary{
		ByStatus:   make(map[string]int),
		ByCurrency: make(map[string]int64),
	}

	for _, p := range payments {
		summary.TotalAmount += p.Amount
		summary.TotalCount++
		summary.ByStatus[p.Status]++
		summary.ByCurrency[p.Currency] += p.Amount
	}

	if summary.TotalCount > 0 {
		summary.AverageAmount = summary.TotalAmount / int64(summary.TotalCount)
	}

	return successResponse(http.StatusOK, summary), nil
}

// exportPayments creates an export job in the queue
func (h *QueryHandler) exportPayments(_ context.Context, merchantID string, req *QueryRequest) (events.APIGatewayProxyResponse, error) {
	// Create export job
	exportJob := &ExportJob{
		ID:         fmt.Sprintf("export-%s-%d", merchantID, time.Now().Unix()),
		MerchantID: merchantID,
		Status:     "pending",
		Query:      *req,
		Format:     "csv", // Default to CSV
		Metadata: map[string]interface{}{
			"requested_at": time.Now().Format(time.RFC3339),
			"requested_by": "API",
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour).Unix(), // Expire after 7 days (Unix timestamp)
	}

	// Save export job to DynamoDB
	if err := h.db.Model(exportJob).Create(); err != nil {
		return errorResponse(http.StatusInternalServerError, "Failed to create export job"), nil
	}

	// In a real implementation, a separate worker process would:
	// 1. Poll for pending export jobs
	// 2. Execute the query
	// 3. Generate the CSV/JSON file
	// 4. Upload to S3
	// 5. Update the job with the result URL
	// 6. Send notification to the user

	// Return immediate response
	response := map[string]any{
		"export_id": exportJob.ID,
		"status":    exportJob.Status,
		"message":   "Export job created. You will receive a notification when complete.",
		"check_url": fmt.Sprintf("/exports/%s", exportJob.ID),
	}

	return successResponse(http.StatusAccepted, response), nil
}

// Helper functions

func (h *QueryHandler) extractMerchantID(headers map[string]string) (string, error) {
	auth := headers["Authorization"]
	if auth == "" {
		auth = headers["authorization"]
	}

	// Use the JWT validator to extract merchant ID
	return utils.ValidateAndExtractMerchantID(auth, h.jwtValidator)
}

func parseQueryParams(params map[string]string) *QueryRequest {
	req := &QueryRequest{}

	req.Status = params["status"]
	req.StartDate = params["start_date"]
	req.EndDate = params["end_date"]
	req.CustomerID = params["customer_id"]
	req.Cursor = params["cursor"]

	// Set default limit if not provided
	if limit, err := strconv.Atoi(params["limit"]); err == nil && limit > 0 {
		req.Limit = limit
	} else {
		req.Limit = 20 // Default limit
	}

	if minAmount, err := strconv.ParseInt(params["min_amount"], 10, 64); err == nil {
		req.MinAmount = minAmount
	}

	if maxAmount, err := strconv.ParseInt(params["max_amount"], 10, 64); err == nil {
		req.MaxAmount = maxAmount
	}

	return req
}

func successResponse(statusCode int, data any) events.APIGatewayProxyResponse {
	body, _ := json.Marshal(map[string]any{
		"success": true,
		"data":    data,
	})

	return events.APIGatewayProxyResponse{
		StatusCode: statusCode,
		Headers: map[string]string{
			"Content-Type":                "application/json",
			"Access-Control-Allow-Origin": "*",
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
			"Content-Type":                "application/json",
			"Access-Control-Allow-Origin": "*",
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
