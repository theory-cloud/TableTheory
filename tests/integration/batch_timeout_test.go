package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory"
	"github.com/theory-cloud/tabletheory/pkg/session"
	"github.com/theory-cloud/tabletheory/tests"
)

// BinRecord represents the structure from the issue report
type BinRecord struct {
	UpdatedRowAt    time.Time `json:"updated_row_at"`
	CardBin         string    `theorydb:"pk" json:"card_bin" validate:"required,min=6,max=6"`
	CardBinExtended string    `theorydb:"sk" json:"card_bin_extended"`
	CardBrand       string    `json:"card_brand" validate:"required"`
	CardType        string    `json:"card_type" validate:"required"`
	CardSubType     string    `json:"card_sub_type" validate:"required"`
	CountryCode     string    `json:"country_code" validate:"required,len=3"`
	CountryCodeNum  string    `json:"country_code_num" validate:"required,len=3"`
}

func (BinRecord) TableName() string {
	return "bin_records_test"
}

func TestBatchCreateTimeout(t *testing.T) {
	tests.RequireDynamoDBLocal(t)

	// Create DB connection
	sessionConfig := session.Config{
		Region:   "us-east-1",
		Endpoint: "http://localhost:8000",
		AWSConfigOptions: []func(*config.LoadOptions) error{
			config.WithCredentialsProvider(
				credentials.NewStaticCredentialsProvider("dummy", "dummy", ""),
			),
			config.WithRegion("us-east-1"),
		},
	}

	db, err := theorydb.New(sessionConfig)
	require.NoError(t, err)

	// Create table
	err = db.AutoMigrate(&BinRecord{})
	require.NoError(t, err)

	t.Run("BatchCreateWithShortTimeout", func(t *testing.T) {
		// Skip this test as DynamoDB Local is too fast to reliably test timeouts
		// In production with real DynamoDB, network latency and throttling make timeouts more relevant
		t.Skip("Skipping timeout test - DynamoDB Local completes operations too quickly to test reliably")
	})

	t.Run("BatchCreateWithProperTimeout", func(t *testing.T) {
		// Create a context with reasonable timeout
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Create test records with unique keys
		records := make([]BinRecord, 10) // Smaller batch
		for i := 0; i < 10; i++ {
			records[i] = BinRecord{
				CardBin:         "654321",
				CardBinExtended: fmt.Sprintf("9876543210987654321%d", i), // Unique sort key
				CardBrand:       "MASTERCARD",
				CardType:        "DEBIT",
				CardSubType:     "STANDARD",
				CountryCode:     "USA",
				CountryCodeNum:  "840",
				UpdatedRowAt:    time.Now(),
			}
		}

		// This should succeed with proper timeout
		err := db.WithContext(ctx).Model(&BinRecord{}).BatchCreate(records)
		require.NoError(t, err)

		// Verify records were created
		var retrievedRecords []BinRecord
		err = db.Model(&BinRecord{}).Scan(&retrievedRecords)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(retrievedRecords), 10)
	})

	// Cleanup
	t.Cleanup(func() {
		var allRecords []BinRecord
		if err := db.Model(&BinRecord{}).Scan(&allRecords); err != nil {
			t.Logf("cleanup scan failed: %v", err)
			return
		}
		for _, record := range allRecords {
			if err := db.Model(&BinRecord{}).
				Where("CardBin", "=", record.CardBin).
				Where("CardBinExtended", "=", record.CardBinExtended).
				Delete(); err != nil {
				t.Logf("cleanup delete failed for %s/%s: %v", record.CardBin, record.CardBinExtended, err)
			}
		}
	})
}

func TestBatchCreateTimeoutCheck(t *testing.T) {
	tests.RequireDynamoDBLocal(t)

	// Create DB connection
	sessionConfig := session.Config{
		Region:   "us-east-1",
		Endpoint: "http://localhost:8000",
		AWSConfigOptions: []func(*config.LoadOptions) error{
			config.WithCredentialsProvider(
				credentials.NewStaticCredentialsProvider("dummy", "dummy", ""),
			),
			config.WithRegion("us-east-1"),
		},
	}

	db, err := theorydb.New(sessionConfig)
	require.NoError(t, err)

	// Create table
	err = db.AutoMigrate(&BinRecord{})
	require.NoError(t, err)

	t.Run("BatchCreateWithLambdaTimeout", func(t *testing.T) {
		// Create a context with Lambda timeout simulation
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Use WithLambdaTimeout to simulate Lambda environment
		dbWithTimeout := db.WithLambdaTimeout(ctx)

		// Create test records with unique keys
		records := make([]BinRecord, 5)
		for i := 0; i < 5; i++ {
			records[i] = BinRecord{
				CardBin:         "111111",
				CardBinExtended: fmt.Sprintf("1111111111111111111%d", i), // Unique sort key
				CardBrand:       "AMEX",
				CardType:        "CREDIT",
				CardSubType:     "PREMIUM",
				CountryCode:     "USA",
				CountryCodeNum:  "840",
				UpdatedRowAt:    time.Now(),
			}
		}

		// This should work with proper Lambda timeout handling
		err := dbWithTimeout.Model(&BinRecord{}).BatchCreate(records)
		require.NoError(t, err)
	})

	// Cleanup
	t.Cleanup(func() {
		var allRecords []BinRecord
		if err := db.Model(&BinRecord{}).Scan(&allRecords); err != nil {
			t.Logf("cleanup scan failed: %v", err)
			return
		}
		for _, record := range allRecords {
			if err := db.Model(&BinRecord{}).
				Where("CardBin", "=", record.CardBin).
				Where("CardBinExtended", "=", record.CardBinExtended).
				Delete(); err != nil {
				t.Logf("cleanup delete failed for %s/%s: %v", record.CardBin, record.CardBinExtended, err)
			}
		}
	})
}

// TestBatchCreateReproduceIssue attempts to reproduce the exact issue from the report
func TestBatchCreateReproduceIssue(t *testing.T) {
	tests.RequireDynamoDBLocal(t)

	// Create DB connection
	sessionConfig := session.Config{
		Region:   "us-east-1",
		Endpoint: "http://localhost:8000",
		AWSConfigOptions: []func(*config.LoadOptions) error{
			config.WithCredentialsProvider(
				credentials.NewStaticCredentialsProvider("dummy", "dummy", ""),
			),
			config.WithRegion("us-east-1"),
		},
	}

	db, err := theorydb.New(sessionConfig)
	require.NoError(t, err)

	// Create table
	err = db.AutoMigrate(&BinRecord{})
	require.NoError(t, err)

	// Simulate the writeChunk function from the issue report
	writeChunk := func(ctx context.Context, records []BinRecord, batchSize int) error {
		for index := 0; index < len(records); index += batchSize {
			end := min(index+batchSize, len(records))
			chunk := records[index:end]
			for index2 := range chunk {
				chunk[index2].UpdatedRowAt = time.Now()
			}
			err := db.WithContext(ctx).Model(&BinRecord{}).BatchCreate(chunk)
			if err != nil {
				return err
			}
		}
		return nil
	}

	t.Run("ReproduceTimeoutIssue", func(t *testing.T) {
		// Create a large number of records to process
		const totalRecords = 200
		const batchSize = 25

		records := make([]BinRecord, totalRecords)
		for i := 0; i < totalRecords; i++ {
			records[i] = BinRecord{
				CardBin:         "999999",
				CardBinExtended: fmt.Sprintf("9999999999999999999%d", i), // Unique sort key
				CardBrand:       "DISCOVER",
				CardType:        "CREDIT",
				CardSubType:     "STANDARD",
				CountryCode:     "USA",
				CountryCodeNum:  "840",
				UpdatedRowAt:    time.Now(),
			}
		}

		// Test with very short timeout to trigger the issue
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
		defer cancel()

		err := writeChunk(ctx, records, batchSize)

		// We expect this to fail with a timeout error
		require.Error(t, err)
		t.Logf("Reproduced timeout error: %v", err)

		// The error should contain "deadline", "timeout", or "retries" (from our improved retry logic)
		errorStr := err.Error()
		require.True(t,
			contains(errorStr, "deadline") || contains(errorStr, "timeout") || contains(errorStr, "retries"),
			"Expected timeout-related error, got: %s", errorStr)
	})

	t.Run("SuccessfulBatchProcessing", func(t *testing.T) {
		// Test with reasonable timeout
		const totalRecords = 50
		const batchSize = 25

		records := make([]BinRecord, totalRecords)
		for i := 0; i < totalRecords; i++ {
			records[i] = BinRecord{
				CardBin:         "888888",
				CardBinExtended: fmt.Sprintf("8888888888888888888%d", i), // Unique sort key
				CardBrand:       "VISA",
				CardType:        "DEBIT",
				CardSubType:     "STANDARD",
				CountryCode:     "USA",
				CountryCodeNum:  "840",
				UpdatedRowAt:    time.Now(),
			}
		}

		// Test with reasonable timeout
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err := writeChunk(ctx, records, batchSize)
		require.NoError(t, err)

		// Verify all records were created
		var retrievedRecords []BinRecord
		err = db.Model(&BinRecord{}).Where("CardBin", "=", "888888").Scan(&retrievedRecords)
		require.NoError(t, err)
		require.Equal(t, totalRecords, len(retrievedRecords))
	})

	// Cleanup
	t.Cleanup(func() {
		var allRecords []BinRecord
		if err := db.Model(&BinRecord{}).Scan(&allRecords); err != nil {
			t.Logf("cleanup scan failed: %v", err)
			return
		}
		for _, record := range allRecords {
			if err := db.Model(&BinRecord{}).
				Where("CardBin", "=", record.CardBin).
				Where("CardBinExtended", "=", record.CardBinExtended).
				Delete(); err != nil {
				t.Logf("cleanup delete failed for %s/%s: %v", record.CardBin, record.CardBinExtended, err)
			}
		}
	})
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || (len(s) > len(substr) &&
		(s[:len(substr)] == substr || contains(s[1:], substr))))
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
