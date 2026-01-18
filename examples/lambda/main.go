package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/aws/aws-lambda-go/lambda"

	"github.com/theory-cloud/tabletheory"
)

// Payment model
type Payment struct {
	CreatedAt time.Time `dynamodb:"created_at"`
	UpdatedAt time.Time `dynamodb:"updated_at"`
	ID        string    `dynamodb:"id,hash"`
	PartnerID string    `dynamodb:"partner_id" index:"partner-index,hash"`
	Currency  string    `dynamodb:"currency"`
	Status    string    `dynamodb:"status"`
	Amount    int64     `dynamodb:"amount"`
}

var (
	db   *tabletheory.MultiAccountDB
	once sync.Once
)

// Initialize DB once during cold start
func init() {
	once.Do(func() {
		log.Println("Initializing TableTheory for Lambda...")
		startTime := time.Now()

		// Configure partner accounts from environment
		accounts := map[string]tabletheory.AccountConfig{
			"partner1": {
				RoleARN:    os.Getenv("PARTNER1_ROLE_ARN"),
				ExternalID: os.Getenv("PARTNER1_EXTERNAL_ID"),
				Region:     getEnvOrDefault("PARTNER1_REGION", "us-east-1"),
			},
			"partner2": {
				RoleARN:    os.Getenv("PARTNER2_ROLE_ARN"),
				ExternalID: os.Getenv("PARTNER2_EXTERNAL_ID"),
				Region:     getEnvOrDefault("PARTNER2_REGION", "us-east-1"),
			},
		}

		var err error
		db, err = tabletheory.NewMultiAccount(accounts)
		if err != nil {
			log.Fatalf("Failed to initialize TableTheory: %v", err)
		}

		// Pre-register all models to reduce cold start
		baseDB, err := db.Partner("")
		if err != nil {
			log.Fatalf("Failed to get base DB: %v", err)
		}

		err = baseDB.PreRegisterModels(&Payment{})
		if err != nil {
			log.Fatalf("Failed to register models: %v", err)
		}

		log.Printf("TableTheory initialized in %v", time.Since(startTime))
	})
}

// Event structure for Lambda input
type Event struct {
	Data      map[string]any `json:"data"`
	PartnerID string         `json:"partnerId"`
	Action    string         `json:"action"`
}

// Response structure for Lambda output
type Response struct {
	Data    any    `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
	Success bool   `json:"success"`
}

// Main Lambda handler
func handler(ctx context.Context, event Event) (Response, error) {
	log.Printf("Processing request: action=%s, partner=%s", event.Action, event.PartnerID)

	// Get partner-specific DB with Lambda timeout
	partnerDB, err := db.Partner(event.PartnerID)
	if err != nil {
		return Response{Success: false, Error: fmt.Sprintf("invalid partner: %v", err)}, nil
	}

	// Apply Lambda timeout handling
	partnerDB = partnerDB.WithLambdaTimeout(ctx)

	// Route based on action
	switch event.Action {
	case "getPayment":
		return handleGetPayment(partnerDB, event.Data)
	case "createPayment":
		return handleCreatePayment(partnerDB, event.Data)
	default:
		return Response{Success: false, Error: "unknown action"}, nil
	}
}

// Get payment by ID
func handleGetPayment(db *tabletheory.LambdaDB, data map[string]any) (Response, error) {
	paymentID, ok := data["paymentId"].(string)
	if !ok {
		return Response{Success: false, Error: "paymentId required"}, nil
	}

	var payment Payment
	err := db.Model(&Payment{}).Where("ID", "=", paymentID).First(&payment)
	if err != nil {
		return Response{Success: false, Error: fmt.Sprintf("payment not found: %v", err)}, nil
	}

	return Response{Success: true, Data: payment}, nil
}

// Create new payment
func handleCreatePayment(db *tabletheory.LambdaDB, data map[string]any) (Response, error) {
	amount, _ := data["amount"].(float64)
	currency, _ := data["currency"].(string)

	if amount == 0 || currency == "" {
		return Response{Success: false, Error: "amount and currency required"}, nil
	}

	payment := Payment{
		ID:        fmt.Sprintf("pay_%d", time.Now().Unix()),
		PartnerID: data["partnerId"].(string),
		Amount:    int64(amount * 100), // Convert to cents
		Currency:  currency,
		Status:    "pending",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := db.Model(&payment).Create()
	if err != nil {
		return Response{Success: false, Error: fmt.Sprintf("failed to create payment: %v", err)}, nil
	}

	return Response{Success: true, Data: payment}, nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func main() {
	lambda.Start(handler)
}
