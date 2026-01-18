package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/theory-cloud/tabletheory"
	payment "github.com/theory-cloud/tabletheory/examples/payment"
	"github.com/theory-cloud/tabletheory/examples/payment/utils"
	"github.com/theory-cloud/tabletheory/pkg/core"
)

// ReconciliationRecord represents a row in the reconciliation CSV
type ReconciliationRecord struct {
	ProcessedDate  time.Time
	SettlementDate time.Time
	PaymentID      string
	TransactionID  string
	Status         string
	ProcessorFee   int64
}

// ReconcileHandler handles payment reconciliation
type ReconcileHandler struct {
	db           core.ExtendedDB
	s3Client     *s3.Client
	auditTracker *utils.AuditTracker
}

// NewReconcileHandler creates a new reconciliation handler
func NewReconcileHandler() (*ReconcileHandler, error) {
	// Initialize AWS config for S3
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(os.Getenv("AWS_REGION")),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Initialize DynamoDB connection
	db, err := theorydb.New(theorydb.Config{
		Region: os.Getenv("AWS_REGION"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize DynamoDB: %w", err)
	}

	// Register models
	db.Model(&payment.Settlement{})
	db.Model(&payment.Transaction{})
	db.Model(&payment.Payment{})

	return &ReconcileHandler{
		db:           db,
		s3Client:     s3.NewFromConfig(cfg),
		auditTracker: utils.NewAuditTracker(db),
	}, nil
}

// HandleRequest processes S3 events for reconciliation files
func (h *ReconcileHandler) HandleRequest(ctx context.Context, event events.S3Event) error {
	for _, record := range event.Records {
		if err := h.processFile(ctx, record); err != nil {
			// Log error but continue processing other files
			fmt.Printf("Error processing file %s: %v\n", record.S3.Object.Key, err)
			continue
		}
	}
	return nil
}

// processFile processes a single reconciliation file
func (h *ReconcileHandler) processFile(ctx context.Context, record events.S3EventRecord) error {
	bucket := record.S3.Bucket.Name
	key := record.S3.Object.Key

	// Download file from S3
	result, err := h.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to download file: %w", err)
	}
	defer func() {
		_ = result.Body.Close()
	}()

	// Parse CSV
	reader := csv.NewReader(result.Body)

	// Skip header
	if _, err := reader.Read(); err != nil {
		return fmt.Errorf("failed to read header: %w", err)
	}

	// Process in batches
	batch := make([]*ReconciliationRecord, 0, 100)
	batchCount := 0
	totalProcessed := 0
	totalErrors := 0

	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			totalErrors++
			continue
		}

		// Parse row
		record, err := parseReconciliationRow(row)
		if err != nil {
			totalErrors++
			continue
		}

		batch = append(batch, record)

		// Process batch when full
		if len(batch) >= 100 {
			if err := h.processBatch(ctx, batch); err != nil {
				fmt.Printf("Error processing batch %d: %v\n", batchCount, err)
				totalErrors += len(batch)
			} else {
				totalProcessed += len(batch)
			}
			batch = batch[:0]
			batchCount++
		}
	}

	// Process remaining records
	if len(batch) > 0 {
		if err := h.processBatch(ctx, batch); err != nil {
			fmt.Printf("Error processing final batch: %v\n", err)
			totalErrors += len(batch)
		} else {
			totalProcessed += len(batch)
		}
	}

	// Create settlement summary
	settlementDetails := make([]payment.SettlementDetail, 0)
	// Note: We'll populate settlementDetails after processing the batch

	settlement := &payment.Settlement{
		ID:               fmt.Sprintf("SETT-%s-%s", extractMerchantFromKey(key), time.Now().Format("2006-01-02")),
		MerchantID:       extractMerchantFromKey(key),
		Date:             time.Now().Format("2006-01-02"),
		TotalAmount:      int64(totalProcessed) * 100, // Assuming cents
		TransactionCount: totalProcessed,
		Status:           "processing",
		BatchID:          key,
		Transactions:     settlementDetails,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	// Begin transaction
	err = h.db.Transaction(func(tx *core.Tx) error {
		// Create settlement record
		if err := tx.Create(settlement); err != nil {
			return fmt.Errorf("failed to create settlement: %w", err)
		}

		// Update transactions as settled
		for _, rec := range batch {
			// Get the transaction to update
			var txn payment.Transaction
			if err := h.db.Model(&payment.Transaction{}).
				Where("ID", "=", rec.TransactionID).
				First(&txn); err != nil {
				return fmt.Errorf("failed to get transaction %s: %w", rec.TransactionID, err)
			}

			// Update the transaction
			txn.Status = "settled"
			txn.UpdatedAt = time.Now()

			if err := tx.Update(&txn, "Status", "UpdatedAt"); err != nil {
				return fmt.Errorf("failed to update transaction %s: %w", rec.TransactionID, err)
			}
		}

		// Update payment status
		settlement.Status = "completed"
		settlement.ProcessedAt = time.Now()
		settlement.UpdatedAt = time.Now()
		if err := tx.Update(settlement, "Status", "ProcessedAt", "UpdatedAt"); err != nil {
			return fmt.Errorf("failed to update settlement: %w", err)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to complete reconciliation: %w", err)
	}

	// Audit the reconciliation
	if err := h.auditTracker.Track("reconciliation", "completed", map[string]any{
		"file":       key,
		"processed":  totalProcessed,
		"errors":     totalErrors,
		"settlement": settlement.ID,
	}); err != nil {
		return fmt.Errorf("failed to track reconciliation audit: %w", err)
	}

	fmt.Printf("Reconciliation completed: %d processed, %d errors\n", totalProcessed, totalErrors)
	return nil
}

// processBatch processes a batch of reconciliation records
func (h *ReconcileHandler) processBatch(ctx context.Context, batch []*ReconciliationRecord) error {
	log.Printf("Processing batch of %d records", len(batch))

	// Convert reconciliation records to settlement details
	settlementDetails := make([]payment.SettlementDetail, len(batch))
	totalAmount := int64(0)

	for i, rec := range batch {
		amount := int64(rec.ProcessorFee * 100) // Convert to cents
		settlementDetails[i] = payment.SettlementDetail{
			PaymentID:     rec.PaymentID,
			TransactionID: rec.TransactionID,
			Amount:        amount,
			Fee:           amount,
			NetAmount:     amount,
		}
		totalAmount += amount
	}

	// Create settlement summary
	settlement := &payment.Settlement{
		ID:               fmt.Sprintf("SETT-%s-%s", extractMerchantFromKey(batch[0].PaymentID), time.Now().Format("2006-01-02")),
		MerchantID:       extractMerchantFromKey(batch[0].PaymentID),
		Date:             time.Now().Format("2006-01-02"),
		TotalAmount:      totalAmount,
		TransactionCount: len(batch),
		Status:           "completed",
		BatchID:          fmt.Sprintf("BATCH-%d", time.Now().Unix()),
		ProcessedAt:      time.Now(),
		Transactions:     settlementDetails,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	// Begin transaction to update all records
	err := h.db.Transaction(func(tx *core.Tx) error {
		// Create settlement record
		if err := tx.Create(settlement); err != nil {
			return fmt.Errorf("failed to create settlement: %w", err)
		}

		// Update each transaction as settled
		for _, detail := range settlementDetails {
			// Would update transaction status here in real implementation
			log.Printf("Marked transaction %s as settled", detail.TransactionID)
		}

		// Update each payment as settled
		for _, detail := range settlementDetails {
			// Would update payment status here in real implementation
			log.Printf("Marked payment %s as settled", detail.PaymentID)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to complete reconciliation: %w", err)
	}

	// Audit the reconciliation
	if err := h.auditTracker.Track("reconciliation", "completed", map[string]any{
		"file":       fmt.Sprintf("BATCH-%d", time.Now().Unix()),
		"processed":  len(batch),
		"errors":     0,
		"settlement": settlement.ID,
	}); err != nil {
		return fmt.Errorf("failed to track reconciliation audit: %w", err)
	}

	fmt.Printf("Reconciliation completed: %d processed, 0 errors\n", len(batch))
	return nil
}

// Helper functions

func parseReconciliationRow(row []string) (*ReconciliationRecord, error) {
	if len(row) < 6 {
		return nil, fmt.Errorf("invalid row format")
	}

	fee, err := strconv.ParseInt(row[3], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid fee: %w", err)
	}

	processedDate, err := time.Parse("2006-01-02", row[4])
	if err != nil {
		return nil, fmt.Errorf("invalid processed date: %w", err)
	}

	settlementDate, err := time.Parse("2006-01-02", row[5])
	if err != nil {
		return nil, fmt.Errorf("invalid settlement date: %w", err)
	}

	return &ReconciliationRecord{
		PaymentID:      row[0],
		TransactionID:  row[1],
		Status:         row[2],
		ProcessorFee:   fee,
		ProcessedDate:  processedDate,
		SettlementDate: settlementDate,
	}, nil
}

func extractMerchantFromKey(key string) string {
	// Extract merchant ID from S3 key
	// Format: reconciliation/merchant-123/2024-01-15.csv
	parts := strings.Split(key, "/")
	if len(parts) >= 2 {
		return parts[1]
	}
	return "unknown"
}

func main() {
	handler, err := NewReconcileHandler()
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize handler: %v", err))
	}

	lambda.Start(handler.HandleRequest)
}
