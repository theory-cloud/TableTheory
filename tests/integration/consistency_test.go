package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/theory-cloud/tabletheory/pkg/consistency"
)

// TestModel for consistency tests
type ConsistencyTestModel struct {
	UpdatedAt time.Time `theorydb:"updated_at"`
	PK        string    `theorydb:"pk"`
	SK        string    `theorydb:"sk"`
	Email     string    `theorydb:"index:email-index,pk"`
	Username  string    `theorydb:"index:username-index,pk"`
	Name      string
	Version   int `theorydb:"version"`
}

func TestConsistentRead(t *testing.T) {
	ctx := InitTestDB(t)
	t.Cleanup(func() {
		if err := ctx.Cleanup(); err != nil {
			t.Logf("cleanup failed: %v", err)
		}
	})
	db := ctx.DB

	// Register model
	db.Model(&ConsistencyTestModel{})
	ctx.CreateTableIfNotExists(t, &ConsistencyTestModel{})

	// Create test item with unique ID
	testID := uuid.New().String()
	item := &ConsistencyTestModel{
		PK:       "USER#consistent-read-test-" + testID,
		SK:       "PROFILE",
		Email:    "consistent-" + testID + "@example.com",
		Username: "consistentuser-" + testID,
		Name:     "Consistent Read Test",
	}

	if err := db.Model(item).Create(); err != nil {
		t.Fatalf("Failed to create item: %v", err)
	}

	t.Run("ConsistentRead on main table", func(t *testing.T) {
		var result ConsistencyTestModel
		err := db.Model(&ConsistencyTestModel{}).
			Where("PK", "=", item.PK).
			Where("SK", "=", item.SK).
			ConsistentRead().
			First(&result)

		if err != nil {
			t.Errorf("Failed to read with ConsistentRead: %v", err)
		}

		if result.Name != item.Name {
			t.Errorf("Expected name %s, got %s", item.Name, result.Name)
		}
	})

	t.Run("ConsistentRead ignored on GSI", func(t *testing.T) {
		// ConsistentRead should be ignored when using GSI
		var result ConsistencyTestModel
		err := db.Model(&ConsistencyTestModel{}).
			Index("email-index").
			Where("Email", "=", item.Email).
			ConsistentRead(). // This should be ignored
			First(&result)

		if err != nil {
			t.Errorf("Failed to read from GSI: %v", err)
		}
	})
}

func TestWithRetry(t *testing.T) {
	ctx := InitTestDB(t)
	t.Cleanup(func() {
		if err := ctx.Cleanup(); err != nil {
			t.Logf("cleanup failed: %v", err)
		}
	})
	db := ctx.DB

	// Register model
	db.Model(&ConsistencyTestModel{})
	ctx.CreateTableIfNotExists(t, &ConsistencyTestModel{})

	t.Run("Retry on GSI query", func(t *testing.T) {
		testID := uuid.New().String()
		item := &ConsistencyTestModel{
			PK:       "USER#retry-test-" + testID,
			SK:       "PROFILE",
			Email:    "retry-" + testID + "@example.com",
			Username: "retryuser-" + testID,
			Name:     "Retry Test",
		}

		if err := db.Model(item).Create(); err != nil {
			t.Fatalf("Failed to create item: %v", err)
		}

		// Query with retry - should eventually find the item
		var result ConsistencyTestModel
		err := db.Model(&ConsistencyTestModel{}).
			Index("email-index").
			Where("Email", "=", item.Email).
			WithRetry(5, 50*time.Millisecond).
			First(&result)

		if err != nil {
			t.Errorf("Failed to read with retry: %v", err)
		}

		if result.Name != item.Name {
			t.Errorf("Expected name %s, got %s", item.Name, result.Name)
		}
	})

	t.Run("Retry with All query", func(t *testing.T) {
		// Create multiple items with unique prefix
		testPrefix := "USER#retry-all-" + uuid.New().String()[:8]
		for i := 0; i < 3; i++ {
			item := &ConsistencyTestModel{
				PK:       fmt.Sprintf("%s-%d", testPrefix, i),
				SK:       "PROFILE",
				Email:    fmt.Sprintf("retry-all-%s-%d@example.com", testPrefix, i),
				Username: fmt.Sprintf("retryall%s%d", testPrefix, i),
				Name:     fmt.Sprintf("Retry All Test %d", i),
			}

			if err := db.Model(item).Create(); err != nil {
				t.Fatalf("Failed to create item %d: %v", i, err)
			}
		}

		// Query all with retry
		var results []ConsistencyTestModel
		err := db.Model(&ConsistencyTestModel{}).
			Where("PK", "BEGINS_WITH", testPrefix).
			WithRetry(5, 50*time.Millisecond).
			All(&results)

		if err != nil {
			t.Errorf("Failed to read all with retry: %v", err)
		}

		if len(results) != 3 {
			t.Errorf("Expected 3 results, got %d", len(results))
		}
	})
}

func TestReadAfterWritePatterns(t *testing.T) {
	ctx := InitTestDB(t)
	t.Cleanup(func() {
		if err := ctx.Cleanup(); err != nil {
			t.Logf("cleanup failed: %v", err)
		}
	})
	db := ctx.DB

	// Register model
	db.Model(&ConsistencyTestModel{})
	ctx.CreateTableIfNotExists(t, &ConsistencyTestModel{})

	helper := consistency.NewReadAfterWriteHelper(db)

	t.Run("CreateWithConsistency", func(t *testing.T) {
		testID := uuid.New().String()
		item := &ConsistencyTestModel{
			PK:       "USER#create-consistency-" + testID,
			SK:       "PROFILE",
			Email:    "create-consistency-" + testID + "@example.com",
			Username: "createconsistency-" + testID,
			Name:     "Create Consistency Test",
		}

		err := helper.CreateWithConsistency(item, &consistency.WriteOptions{
			VerifyWrite:           true,
			WaitForGSIPropagation: 100 * time.Millisecond,
		})

		if err != nil {
			t.Errorf("Failed to create with consistency: %v", err)
		}

		// Immediately query GSI - should work due to wait
		var result ConsistencyTestModel
		err = db.Model(&ConsistencyTestModel{}).
			Index("email-index").
			Where("Email", "=", item.Email).
			First(&result)

		if err != nil {
			t.Errorf("Failed to query GSI after wait: %v", err)
		}
	})

	t.Run("UpdateWithConsistency", func(t *testing.T) {
		testID := uuid.New().String()
		item := &ConsistencyTestModel{
			PK:       "USER#update-consistency-" + testID,
			SK:       "PROFILE",
			Email:    "update-consistency-" + testID + "@example.com",
			Username: "updateconsistency-" + testID,
			Name:     "Original Name",
		}

		if err := db.Model(item).Create(); err != nil {
			t.Fatalf("Failed to create item: %v", err)
		}

		// Update with verification
		item.Name = "Updated Name"
		err := helper.UpdateWithConsistency(item, []string{"Name"}, &consistency.WriteOptions{
			VerifyWrite: true,
		})

		if err != nil {
			t.Errorf("Failed to update with consistency: %v", err)
		}

		if item.Name != "Updated Name" {
			t.Errorf("Expected name to be updated")
		}
	})

	t.Run("QueryAfterWrite patterns", func(t *testing.T) {
		testID := uuid.New().String()
		item := &ConsistencyTestModel{
			PK:       "USER#query-after-write-" + testID,
			SK:       "PROFILE",
			Email:    "query-after-write-" + testID + "@example.com",
			Username: "queryafterwrite-" + testID,
			Name:     "Query After Write Test",
		}

		if err := db.Model(item).Create(); err != nil {
			t.Fatalf("Failed to create item: %v", err)
		}

		// Test 1: Use main table for immediate consistency
		var result1 ConsistencyTestModel
		err := helper.QueryAfterWrite(&ConsistencyTestModel{}, &consistency.QueryAfterWriteOptions{
			UseMainTable: true,
		}).
			Where("PK", "=", item.PK).
			Where("SK", "=", item.SK).
			First(&result1)

		if err != nil {
			t.Errorf("Failed to query with main table: %v", err)
		}

		// Test 2: Use GSI with retry
		var result2 ConsistencyTestModel
		err = helper.QueryAfterWrite(&ConsistencyTestModel{}, &consistency.QueryAfterWriteOptions{
			RetryConfig: consistency.RecommendedRetryConfig(),
		}).
			Index("email-index").
			Where("Email", "=", item.Email).
			First(&result2)

		if err != nil {
			t.Errorf("Failed to query GSI with retry: %v", err)
		}

		// Test 3: Custom verification function
		var result3 ConsistencyTestModel
		err = helper.QueryAfterWrite(&ConsistencyTestModel{}, &consistency.QueryAfterWriteOptions{
			RetryConfig: consistency.RecommendedRetryConfig(),
			VerifyFunc: func(result any) bool {
				r, ok := result.(*ConsistencyTestModel)
				return ok && r.Name == item.Name
			},
		}).
			Index("username-index").
			Where("Username", "=", item.Username).
			First(&result3)

		if err != nil {
			t.Errorf("Failed to query with custom verification: %v", err)
		}
	})
}

func TestWriteAndReadPattern(t *testing.T) {
	ctx := InitTestDB(t)
	t.Cleanup(func() {
		if err := ctx.Cleanup(); err != nil {
			t.Logf("cleanup failed: %v", err)
		}
	})
	db := ctx.DB

	// Register model
	db.Model(&ConsistencyTestModel{})
	ctx.CreateTableIfNotExists(t, &ConsistencyTestModel{})

	pattern := consistency.NewWriteAndReadPattern(db)

	t.Run("CreateAndQueryGSI", func(t *testing.T) {
		testID := uuid.New().String()
		item := &ConsistencyTestModel{
			PK:       "USER#create-query-gsi-" + testID,
			SK:       "PROFILE",
			Email:    "create-query-gsi-" + testID + "@example.com",
			Username: "createquerygsi-" + testID,
			Name:     "Create and Query GSI Test",
		}

		var result ConsistencyTestModel
		err := pattern.CreateAndQueryGSI(
			item,
			"email-index",
			"Email",
			item.Email,
			&result,
		)

		if err != nil {
			t.Errorf("Failed CreateAndQueryGSI: %v", err)
		}

		if result.Name != item.Name {
			t.Errorf("Expected name %s, got %s", item.Name, result.Name)
		}
	})

	t.Run("UpdateAndVerify", func(t *testing.T) {
		testID := uuid.New().String()
		item := &ConsistencyTestModel{
			PK:       "USER#update-verify-" + testID,
			SK:       "PROFILE",
			Email:    "update-verify-" + testID + "@example.com",
			Username: "updateverify-" + testID,
			Name:     "Original Name",
		}

		if err := db.Model(item).Create(); err != nil {
			t.Fatalf("Failed to create item: %v", err)
		}

		// Update and verify
		item.Name = "Verified Update"
		err := pattern.UpdateAndVerify(item, []string{"Name"})

		if err != nil {
			t.Errorf("Failed UpdateAndVerify: %v", err)
		}

		// Double-check with a fresh read
		var verifyResult ConsistencyTestModel
		err = db.Model(&ConsistencyTestModel{}).
			Where("PK", "=", item.PK).
			Where("SK", "=", item.SK).
			ConsistentRead().
			First(&verifyResult)

		if err != nil {
			t.Errorf("Failed to verify update: %v", err)
		}

		if verifyResult.Name != "Verified Update" {
			t.Errorf("Expected name 'Verified Update', got %s", verifyResult.Name)
		}
	})
}

func TestConsistencyEdgeCases(t *testing.T) {
	ctx := InitTestDB(t)
	t.Cleanup(func() {
		if err := ctx.Cleanup(); err != nil {
			t.Logf("cleanup failed: %v", err)
		}
	})
	db := ctx.DB

	// Register model
	db.Model(&ConsistencyTestModel{})
	ctx.CreateTableIfNotExists(t, &ConsistencyTestModel{})

	t.Run("Retry timeout with context", func(t *testing.T) {
		// Use a short timeout and reasonable retry settings
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		// Try to query non-existent item with retry (should fail but respect timeout)
		var result ConsistencyTestModel
		err := db.WithContext(ctx).
			Model(&ConsistencyTestModel{}).
			Index("email-index").
			Where("Email", "=", "nonexistent@example.com").
			WithRetry(3, 100*time.Millisecond). // 3 retries @ 100ms = ~300ms max
			First(&result)

		// Should fail since item doesn't exist
		if err == nil {
			t.Errorf("Expected error for non-existent item")
		}
	})

	t.Run("Mixed consistency strategies", func(t *testing.T) {
		testID := uuid.New().String()
		item := &ConsistencyTestModel{
			PK:       "USER#mixed-strategy-" + testID,
			SK:       "PROFILE",
			Email:    "mixed-" + testID + "@example.com",
			Username: "mixeduser-" + testID,
			Name:     "Mixed Strategy Test",
		}

		if err := db.Model(item).Create(); err != nil {
			t.Fatalf("Failed to create item: %v", err)
		}

		// Try ConsistentRead with WithRetry (ConsistentRead should take precedence on main table)
		var result ConsistencyTestModel
		err := db.Model(&ConsistencyTestModel{}).
			Where("PK", "=", item.PK).
			Where("SK", "=", item.SK).
			ConsistentRead().
			WithRetry(3, 50*time.Millisecond).
			First(&result)

		if err != nil {
			t.Errorf("Failed mixed strategy query: %v", err)
		}
	})
}
