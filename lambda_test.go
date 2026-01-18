package theorydb

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLambdaEnvironmentDetection(t *testing.T) {
	// Test without Lambda environment
	assert.False(t, IsLambdaEnvironment())
	assert.Equal(t, 0, GetLambdaMemoryMB())

	// Set Lambda environment variables
	t.Setenv("AWS_LAMBDA_FUNCTION_NAME", "test-function")
	t.Setenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE", "512")

	assert.True(t, IsLambdaEnvironment())
	assert.Equal(t, 512, GetLambdaMemoryMB())
}

func TestLambdaDBCreation(t *testing.T) {
	// Skip if no AWS credentials available
	if os.Getenv("AWS_ACCESS_KEY_ID") == "" && os.Getenv("AWS_PROFILE") == "" {
		t.Skip("Skipping test - AWS credentials not available")
		return
	}

	// Set test environment
	t.Setenv("AWS_REGION", "us-east-1")

	db, err := NewLambdaOptimized()
	require.NoError(t, err)
	assert.NotNil(t, db)

	// Test that subsequent calls return the same instance (warm start)
	db2, err := NewLambdaOptimized()
	require.NoError(t, err)
	assert.Equal(t, db, db2, "Should return cached instance")
}

func TestLambdaTimeout(t *testing.T) {
	// Skip if no AWS credentials available
	if os.Getenv("AWS_ACCESS_KEY_ID") == "" && os.Getenv("AWS_PROFILE") == "" {
		t.Skip("Skipping test - AWS credentials not available")
		return
	}

	db, err := NewLambdaOptimized()
	require.NoError(t, err)

	// Create context with short deadline
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Apply Lambda timeout
	lambdaDB := db.WithLambdaTimeout(ctx)
	assert.NotNil(t, lambdaDB)

	// Wait for timeout
	time.Sleep(150 * time.Millisecond)

	// Operations should fail with timeout
	type TestModel struct {
		ID   string `theorydb:"pk"`
		Data string
	}

	// First register the model
	err = lambdaDB.PreRegisterModels(&TestModel{})
	assert.NoError(t, err)

	// Since we've waited past the deadline, this should fail
	var result TestModel
	err = lambdaDB.Model(&TestModel{}).Where("ID", "=", "test-1").First(&result)
	assert.Error(t, err)
	// The error could be various things depending on when/how it fails
	// Just check that we got an error
}

func TestPartnerContext(t *testing.T) {
	ctx := context.Background()

	// Add partner to context
	partnerCtx := PartnerContext(ctx, "partner123")

	// Retrieve partner from context
	partnerID := GetPartnerFromContext(partnerCtx)
	assert.Equal(t, "partner123", partnerID)

	// Test with no partner in context
	noPartnerID := GetPartnerFromContext(ctx)
	assert.Equal(t, "", noPartnerID)
}

func TestGetRemainingTimeMillis(t *testing.T) {
	// Test with no deadline
	ctx := context.Background()
	remaining := GetRemainingTimeMillis(ctx)
	assert.Equal(t, int64(-1), remaining)

	// Test with deadline
	deadline := time.Now().Add(5 * time.Second)
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	remaining = GetRemainingTimeMillis(ctx)
	assert.Greater(t, remaining, int64(4000))
	assert.LessOrEqual(t, remaining, int64(5000))
}

// Benchmark cold start performance
func BenchmarkLambdaColdStart(b *testing.B) {
	// Clear global instance to simulate cold start
	globalLambdaDB = nil

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		globalLambdaDB = nil // Reset for each iteration

		startTime := time.Now()
		db, err := NewLambdaOptimized()
		if err != nil {
			b.Fatal(err)
		}

		coldStartTime := time.Since(startTime)
		if i == 0 {
			b.Logf("Cold start time: %v", coldStartTime)
		}
		_ = db
	}
}

// Benchmark warm start performance
func BenchmarkLambdaWarmStart(b *testing.B) {
	// Initialize once
	if _, err := NewLambdaOptimized(); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		startTime := time.Now()
		db, err := NewLambdaOptimized()
		if err != nil {
			b.Fatal(err)
		}

		warmStartTime := time.Since(startTime)
		if i == 0 {
			b.Logf("Warm start time: %v", warmStartTime)
		}

		// Verify it's the cached instance
		if db != globalLambdaDB {
			b.Fatal("Did not get cached instance")
		}
	}
}

// Benchmark multi-account partner switching
func BenchmarkMultiAccountPartnerSwitch(b *testing.B) {
	accounts := make(map[string]AccountConfig)
	for i := 0; i < 10; i++ {
		accounts[fmt.Sprintf("partner%d", i)] = AccountConfig{
			RoleARN:    fmt.Sprintf("arn:aws:iam::123456789012:role/TableTheoryRole%d", i),
			ExternalID: fmt.Sprintf("external-id-%d", i),
			Region:     "us-east-1",
		}
	}

	multiDB, err := NewMultiAccount(accounts)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		partnerID := fmt.Sprintf("partner%d", i%10)
		_, err := multiDB.Partner(partnerID)
		if err != nil {
			// Skip errors in benchmark (likely due to missing AWS creds)
			b.Skip("Skipping due to AWS credential error")
		}
	}
}
