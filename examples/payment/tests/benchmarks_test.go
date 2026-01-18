package tests

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/google/uuid"

	"github.com/theory-cloud/tabletheory"
	payment "github.com/theory-cloud/tabletheory/examples/payment"
	"github.com/theory-cloud/tabletheory/examples/payment/utils"
	"github.com/theory-cloud/tabletheory/pkg/core"
)

// BenchmarkPaymentCreate benchmarks single payment creation
func BenchmarkPaymentCreate(b *testing.B) {
	db, err := initBenchDB(b)
	if err != nil {
		b.Fatal(err)
	}

	merchant := createBenchMerchant(b, db)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			payment := &payment.Payment{
				ID:             uuid.New().String(),
				IdempotencyKey: uuid.New().String(),
				MerchantID:     merchant.ID,
				Amount:         1000,
				Currency:       "USD",
				Status:         payment.PaymentStatusPending,
				PaymentMethod:  "card",
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
				Version:        1,
			}

			if err := db.Model(payment).Create(); err != nil {
				b.Error(err)
			}
		}
	})

	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "payments/sec")
}

// BenchmarkIdempotencyCheck benchmarks idempotency key checking
func BenchmarkIdempotencyCheck(b *testing.B) {
	db, err := initBenchDB(b)
	if err != nil {
		b.Fatal(err)
	}

	merchant := createBenchMerchant(b, db)
	idempotency := utils.NewIdempotencyMiddleware(db, 24*time.Hour)

	// Pre-populate some idempotency records
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("bench-key-%d", i)
		_, _ = idempotency.Process(context.TODO(), merchant.ID, key, func() (any, error) {
			return &payment.Payment{ID: fmt.Sprintf("payment-%d", i)}, nil
		})
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			// Mix of existing and new keys
			key := fmt.Sprintf("bench-key-%d", i%1500)
			_, _ = idempotency.Process(context.TODO(), merchant.ID, key, func() (any, error) {
				return &payment.Payment{ID: fmt.Sprintf("payment-new-%d", i)}, nil
			})
			i++
		}
	})

	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "checks/sec")
}

// BenchmarkBatchPaymentCreate benchmarks batch payment creation
func BenchmarkBatchPaymentCreate(b *testing.B) {
	db, err := initBenchDB(b)
	if err != nil {
		b.Fatal(err)
	}

	merchant := createBenchMerchant(b, db)
	batchSizes := []int{10, 25, 50, 100}

	for _, batchSize := range batchSizes {
		b.Run(fmt.Sprintf("BatchSize-%d", batchSize), func(b *testing.B) {
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				payments := make([]*payment.Payment, batchSize)
				for j := 0; j < batchSize; j++ {
					payments[j] = &payment.Payment{
						ID:             fmt.Sprintf("batch-%d-%d", i, j),
						IdempotencyKey: fmt.Sprintf("batch-key-%d-%d", i, j),
						MerchantID:     merchant.ID,
						Amount:         int64(1000 + j),
						Currency:       "USD",
						Status:         payment.PaymentStatusSucceeded,
						PaymentMethod:  "card",
						CreatedAt:      time.Now(),
						UpdatedAt:      time.Now(),
						Version:        1,
					}
				}

				// Batch create
				if err := db.Model(&payment.Payment{}).BatchCreate(payments); err != nil {
					b.Error(err)
				}
			}

			totalPayments := b.N * batchSize
			b.ReportMetric(float64(totalPayments)/b.Elapsed().Seconds(), "payments/sec")
			b.ReportMetric(b.Elapsed().Seconds()/float64(b.N), "sec/batch")
		})
	}
}

// BenchmarkQueryMerchantPayments benchmarks querying payments by merchant
func BenchmarkQueryMerchantPayments(b *testing.B) {
	db, err := initBenchDB(b)
	if err != nil {
		b.Fatal(err)
	}

	merchant := createBenchMerchant(b, db)

	// Pre-populate payments
	for i := 0; i < 10000; i++ {
		payment := &payment.Payment{
			ID:             fmt.Sprintf("query-payment-%d", i),
			IdempotencyKey: fmt.Sprintf("query-key-%d", i),
			MerchantID:     merchant.ID,
			Amount:         int64(1000 + (i % 1000)),
			Currency:       "USD",
			Status:         getRandomStatus(i),
			PaymentMethod:  "card",
			CustomerID:     fmt.Sprintf("customer-%d", i%100),
			CreatedAt:      time.Now().Add(-time.Duration(i) * time.Minute),
			UpdatedAt:      time.Now(),
			Version:        1,
		}

		if err := db.Model(payment).Create(); err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	b.Run("AllPayments", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			var payments []*payment.Payment
			err := db.Model(&payment.Payment{}).
				Index("gsi-merchant").
				Where("MerchantID", "=", merchant.ID).
				Limit(100).
				All(&payments)

			if err != nil {
				b.Error(err)
			}
		}
		b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "queries/sec")
	})

	b.Run("FilteredByStatus", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			var payments []*payment.Payment
			err := db.Model(&payment.Payment{}).
				Index("gsi-merchant").
				Where("MerchantID", "=", merchant.ID).
				Where("Status", "=", payment.PaymentStatusSucceeded).
				Limit(100).
				All(&payments)

			if err != nil {
				b.Error(err)
			}
		}
		b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "queries/sec")
	})

	b.Run("WithPagination", func(b *testing.B) {
		offset := 0
		pageSize := 20

		for i := 0; i < b.N; i++ {
			var payments []*payment.Payment
			err := db.Model(&payment.Payment{}).
				Index("gsi-merchant").
				Where("MerchantID", "=", merchant.ID).
				Limit(pageSize).
				Offset(offset).
				All(&payments)

			if err != nil {
				b.Error(err)
			}

			// Reset offset after 5 pages
			if i%5 == 0 {
				offset = 0
			} else {
				offset += pageSize
			}
		}
		b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "pages/sec")
	})
}

// BenchmarkComplexTransaction benchmarks complex payment transactions
func BenchmarkComplexTransaction(b *testing.B) {
	db, err := initBenchDB(b)
	if err != nil {
		b.Fatal(err)
	}

	merchant := createBenchMerchant(b, db)
	customer := createBenchCustomer(b, db, merchant.ID)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Create payment
		paymentRecord := &payment.Payment{
			ID:             fmt.Sprintf("tx-payment-%d", i),
			IdempotencyKey: fmt.Sprintf("tx-key-%d", i),
			MerchantID:     merchant.ID,
			CustomerID:     customer.ID,
			Amount:         5000,
			Currency:       "USD",
			Status:         payment.PaymentStatusPending,
			PaymentMethod:  "card",
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
			Version:        1,
		}

		// Use transaction
		err := db.Transaction(func(tx *core.Tx) error {
			if err := tx.Create(paymentRecord); err != nil {
				return err
			}

			// Create transaction record
			txRecord := &payment.Transaction{
				ID:          fmt.Sprintf("tx-trans-%d", i),
				PaymentID:   paymentRecord.ID,
				Type:        payment.TransactionTypeCapture,
				Amount:      paymentRecord.Amount,
				Status:      "processing",
				ProcessedAt: time.Now(),
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
				Version:     1,
			}

			if err := tx.Create(txRecord); err != nil {
				return err
			}

			// Update payment status
			paymentRecord.Status = payment.PaymentStatusSucceeded
			paymentRecord.UpdatedAt = time.Now()

			return tx.Update(paymentRecord, "Status", "UpdatedAt")
		})

		if err != nil {
			b.Error(err)
		}
	}

	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "transactions/sec")
}

// BenchmarkConcurrentOperations benchmarks concurrent payment operations
func BenchmarkConcurrentOperations(b *testing.B) {
	db, err := initBenchDB(b)
	if err != nil {
		b.Fatal(err)
	}

	merchant := createBenchMerchant(b, db)
	concurrencyLevels := []int{1, 5, 10, 20, 50}

	for _, concurrency := range concurrencyLevels {
		b.Run(fmt.Sprintf("Concurrency-%d", concurrency), func(b *testing.B) {
			b.ResetTimer()

			var wg sync.WaitGroup
			semaphore := make(chan struct{}, concurrency)

			startTime := time.Now()
			totalOps := b.N

			for i := 0; i < totalOps; i++ {
				wg.Add(1)
				semaphore <- struct{}{}

				go func(idx int) {
					defer wg.Done()
					defer func() { <-semaphore }()

					// Mix of operations
					switch idx % 4 {
					case 0: // Create
						payment := &payment.Payment{
							ID:             fmt.Sprintf("concurrent-%d", idx),
							IdempotencyKey: fmt.Sprintf("concurrent-key-%d", idx),
							MerchantID:     merchant.ID,
							Amount:         1000,
							Currency:       "USD",
							Status:         payment.PaymentStatusPending,
							PaymentMethod:  "card",
							CreatedAt:      time.Now(),
							UpdatedAt:      time.Now(),
							Version:        1,
						}
						_ = db.Model(payment).Create()

					case 1: // Query
						var payments []*payment.Payment
						_ = db.Model(&payment.Payment{}).
							Index("gsi-merchant").
							Where("MerchantID", "=", merchant.ID).
							Limit(10).
							All(&payments)

					case 2: // Update
						p := &payment.Payment{
							ID:        fmt.Sprintf("concurrent-%d", idx-1),
							Status:    payment.PaymentStatusSucceeded,
							UpdatedAt: time.Now(),
						}
						_ = db.Model(p).Update("Status", "UpdatedAt")

					case 3: // Get
						var p payment.Payment
						_ = db.Model(&payment.Payment{}).
							Where("ID", "=", fmt.Sprintf("concurrent-%d", idx-2)).
							First(&p)
					}
				}(i)
			}

			wg.Wait()
			elapsed := time.Since(startTime)

			b.ReportMetric(float64(totalOps)/elapsed.Seconds(), "ops/sec")
			b.ReportMetric(float64(concurrency), "concurrency")
		})
	}
}

// BenchmarkQueryWithFilters benchmarks queries with multiple filters
func BenchmarkQueryWithFilters(b *testing.B) {
	db := setupBenchmark(b)
	defer teardownBenchmark(b, db)

	// Create test data
	createBenchmarkData(b, db, 10000)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			var payments []*payment.Payment
			err := db.Model(&payment.Payment{}).
				Index("gsi-merchant").
				Where("MerchantID", "=", fmt.Sprintf("merchant-%d", b.N%10)).
				Filter("Status", "=", getRandomStatus(b.N)).
				Filter("Amount", ">", 100).
				Limit(100).
				All(&payments)

			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkQueryPagination benchmarks paginated queries
func BenchmarkQueryPagination(b *testing.B) {
	db := setupBenchmark(b)
	defer teardownBenchmark(b, db)

	// Create test data
	createBenchmarkData(b, db, 10000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Pagination is handled by the client using limit/offset
		pageSize := 50
		offset := 0

		for {
			var payments []*payment.Payment
			err := db.Model(&payment.Payment{}).
				Index("gsi-merchant").
				Where("MerchantID", "=", "merchant-0").
				OrderBy("CreatedAt", "DESC").
				Limit(pageSize).
				Offset(offset).
				All(&payments)

			if err != nil {
				b.Fatal(err)
			}

			if len(payments) < pageSize {
				break
			}

			offset += pageSize
		}
	}
}

// BenchmarkTransactionOperations benchmarks complex transactional operations
func BenchmarkTransactionOperations(b *testing.B) {
	db := setupBenchmark(b)
	defer teardownBenchmark(b, db)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		paymentRecord := createTestPayment()

		err := db.Transaction(func(tx *core.Tx) error {
			// Create payment
			if err := tx.Create(paymentRecord); err != nil {
				return err
			}

			// Create transaction
			txRecord := createTestTransaction(paymentRecord.ID)
			if err := tx.Create(txRecord); err != nil {
				return err
			}

			// Update payment status
			paymentRecord.Status = payment.PaymentStatusSucceeded
			paymentRecord.UpdatedAt = time.Now()

			if err := tx.Update(paymentRecord, "Status", "UpdatedAt"); err != nil {
				return err
			}

			return nil
		})

		if err != nil {
			b.Fatal(err)
		}
	}
}

// Helper functions

func initBenchDB(_ *testing.B) (core.ExtendedDB, error) {
	db, err := theorydb.New(theorydb.Config{
		Region:   "us-east-1",
		Endpoint: "http://localhost:8000",
		AWSConfigOptions: []func(*config.LoadOptions) error{
			config.WithCredentialsProvider(aws.CredentialsProviderFunc(
				func(ctx context.Context) (aws.Credentials, error) {
					return aws.Credentials{
						AccessKeyID:     "dummy",
						SecretAccessKey: "dummy",
					}, nil
				})),
		},
	})
	if err != nil {
		return nil, err
	}

	// Register models
	models := []any{
		&payment.Payment{},
		&payment.Transaction{},
		&payment.Customer{},
		&payment.Merchant{},
		&payment.IdempotencyRecord{},
		&utils.AuditLog{},
	}

	for _, model := range models {
		db.Model(model)
	}

	return db, nil
}

func createBenchMerchant(b *testing.B, db core.ExtendedDB) *payment.Merchant {
	merchant := &payment.Merchant{
		ID:       fmt.Sprintf("bench-merchant-%d", time.Now().UnixNano()),
		Name:     "Benchmark Merchant",
		Email:    "bench@example.com",
		Status:   "active",
		Features: []string{"payments", "refunds"},
		RateLimits: payment.RateLimits{
			PaymentsPerMinute: 1000,
			PaymentsPerDay:    100000,
			MaxPaymentAmount:  1000000,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Version:   1,
	}

	if err := db.Model(merchant).Create(); err != nil {
		b.Fatal(err)
	}

	return merchant
}

func createBenchCustomer(b *testing.B, db core.ExtendedDB, merchantID string) *payment.Customer {
	customer := &payment.Customer{
		ID:         uuid.New().String(),
		MerchantID: merchantID,
		Email:      "bench-customer@example.com",
		Name:       "Benchmark Customer",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		Version:    1,
	}

	if err := db.Model(customer).Create(); err != nil {
		b.Fatal(err)
	}

	return customer
}

func getRandomStatus(i int) string {
	statuses := []string{
		payment.PaymentStatusPending,
		payment.PaymentStatusProcessing,
		payment.PaymentStatusSucceeded,
		payment.PaymentStatusFailed,
		payment.PaymentStatusCanceled,
	}
	return statuses[i%len(statuses)]
}

func createTestTransaction(paymentID string) *payment.Transaction {
	return &payment.Transaction{
		ID:          uuid.New().String(),
		PaymentID:   paymentID,
		Type:        payment.TransactionTypeCapture,
		Amount:      1000,
		Status:      "succeeded",
		ProcessedAt: time.Now(),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Version:     1,
	}
}

func createTestPayment() *payment.Payment {
	return &payment.Payment{
		ID:             uuid.New().String(),
		IdempotencyKey: uuid.New().String(),
		MerchantID:     "merchant-123",
		Amount:         1000,
		Currency:       "USD",
		Status:         payment.PaymentStatusPending,
		PaymentMethod:  "card",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Version:        1,
	}
}

// setupBenchmark initializes the benchmark environment
func setupBenchmark(b *testing.B) core.ExtendedDB {
	db, err := initBenchDB(b)
	if err != nil {
		b.Fatal(err)
	}
	return db
}

// teardownBenchmark cleans up after benchmarks
func teardownBenchmark(b *testing.B, db core.ExtendedDB) {
	// In production, you might want to clean up test data
	// For benchmarks, we'll just close the connection
	if err := db.Close(); err != nil {
		b.Logf("Failed to close database connection: %v", err)
	}
}

// createBenchmarkData creates test data for benchmarks
func createBenchmarkData(b *testing.B, db core.ExtendedDB, count int) {
	// Create a test merchant
	merchant := createBenchMerchant(b, db)

	// Create payments in batches
	batchSize := 100
	for i := 0; i < count; i += batchSize {
		batch := make([]*payment.Payment, 0, batchSize)

		for j := 0; j < batchSize && i+j < count; j++ {
			p := &payment.Payment{
				ID:             fmt.Sprintf("bench-payment-%d", i+j),
				IdempotencyKey: fmt.Sprintf("bench-key-%d", i+j),
				MerchantID:     merchant.ID,
				Amount:         int64(1000 + (i+j)%10000),
				Currency:       "USD",
				Status:         getRandomStatus(i + j),
				PaymentMethod:  "card",
				CustomerID:     fmt.Sprintf("customer-%d", (i+j)%100),
				CreatedAt:      time.Now().Add(-time.Duration(i+j) * time.Minute),
				UpdatedAt:      time.Now(),
				Version:        1,
			}
			batch = append(batch, p)
		}

		if err := db.Model(&payment.Payment{}).BatchCreate(batch); err != nil {
			b.Fatal(err)
		}
	}
}

// setupTestDB function removed - using initBenchDB for benchmarks
