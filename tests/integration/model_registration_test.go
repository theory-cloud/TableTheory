package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ModelRegTest is used for testing model registration
type ModelRegTest struct {
	CreatedAt time.Time `theorydb:"created_at"`
	ID        string    `theorydb:"pk"`
	Name      string    `theorydb:"sk"`
}

// TableName returns the DynamoDB table name
func (ModelRegTest) TableName() string {
	return "test_model_registration"
}

// BinRecordTest is the same structure from the user's report
type BinRecordTest struct {
	UpdatedRowAt    time.Time `json:"updated_row_at"`
	CardBin         string    `theorydb:"pk" json:"card_bin"`
	CardBinExtended string    `theorydb:"sk" json:"card_bin_extended"`
	CardBrand       string    `json:"card_brand"`
}

// TableName returns the DynamoDB table name
func (BinRecordTest) TableName() string {
	return "bin_record_test_debug"
}

// TestModelWithContext verifies that model registration works correctly when using WithContext
func TestModelWithContext(t *testing.T) {
	testCtx := InitTestDB(t)
	t.Cleanup(func() {
		if err := testCtx.Cleanup(); err != nil {
			t.Logf("cleanup failed: %v", err)
		}
	})

	// Ensure table exists
	err := testCtx.DB.AutoMigrate(&ModelRegTest{})
	require.NoError(t, err)

	t.Run("ModelRegistrationWithContext", func(t *testing.T) {
		ctx := context.Background()

		// Clean up any existing data first
		var existing []ModelRegTest
		err := testCtx.DB.Model(&ModelRegTest{}).All(&existing)
		require.NoError(t, err)
		for _, item := range existing {
			err = testCtx.DB.Model(&ModelRegTest{}).
				Where("ID", "=", item.ID).
				Where("Name", "=", item.Name).
				Delete()
			require.NoError(t, err)
		}

		// Create a record using WithContext
		model := &ModelRegTest{
			ID:   "test-1",
			Name: "Test Item",
		}

		// This should work - the model should be registered correctly
		err = testCtx.DB.WithContext(ctx).Model(model).Create()
		require.NoError(t, err, "Model registration should work with WithContext")

		// Verify the record was created
		var retrieved ModelRegTest
		err = testCtx.DB.Model(&ModelRegTest{}).
			Where("ID", "=", "test-1").
			Where("Name", "=", "Test Item").
			First(&retrieved)
		require.NoError(t, err)
		require.Equal(t, model.ID, retrieved.ID)
		require.Equal(t, model.Name, retrieved.Name)
	})

	t.Run("BatchCreateWithContext", func(t *testing.T) {
		ctx := context.Background()

		// Create multiple records using BatchCreate with WithContext
		models := []ModelRegTest{
			{ID: "batch-1", Name: "Item 1"},
			{ID: "batch-2", Name: "Item 2"},
			{ID: "batch-3", Name: "Item 3"},
		}

		// This should work - the model should be registered correctly
		err := testCtx.DB.WithContext(ctx).Model(&ModelRegTest{}).BatchCreate(models)
		require.NoError(t, err, "BatchCreate should work with WithContext")

		// Verify all records were created
		var retrieved []ModelRegTest
		err = testCtx.DB.Model(&ModelRegTest{}).
			Where("ID", "=", "batch-1").
			Scan(&retrieved)
		require.NoError(t, err)
		require.Len(t, retrieved, 1)
	})

	t.Run("ErrorMessageImprovement", func(t *testing.T) {
		// Test with an invalid model (missing primary key)
		type InvalidModel struct {
			Name string // No primary key
		}

		ctx := context.Background()
		model := &InvalidModel{Name: "test"}

		// This should return a clear error message
		err := testCtx.DB.WithContext(ctx).Model(model).Create()
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to register model")
		require.Contains(t, err.Error(), "*integration.InvalidModel")
	})
}

// TestBreakpointDebugging helps verify that breakpoints work correctly
func TestBreakpointDebugging(t *testing.T) {
	testCtx := InitTestDB(t)
	t.Cleanup(func() {
		if err := testCtx.Cleanup(); err != nil {
			t.Logf("cleanup failed: %v", err)
		}
	})

	// Ensure table exists
	err := testCtx.DB.AutoMigrate(&BinRecordTest{})
	require.NoError(t, err)

	t.Run("DebugBatchCreate", func(t *testing.T) {
		ctx := context.Background()

		// Simulate the user's code pattern
		records := []BinRecordTest{
			{
				CardBin:         "123456",
				CardBinExtended: "1234567890",
				CardBrand:       "VISA",
				UpdatedRowAt:    time.Now(),
			},
		}

		// Get intermediate values for debugging
		db := testCtx.DB.WithContext(ctx)
		query := db.Model(&BinRecordTest{})

		// Log the query type for debugging
		t.Logf("Query type: %T", query)

		// This should hit the actual BatchCreate method, not errorQuery
		err := query.BatchCreate(records)
		require.NoError(t, err, "BatchCreate should succeed")

		// Verify the record was created
		var retrieved []BinRecordTest
		err = testCtx.DB.Model(&BinRecordTest{}).Scan(&retrieved)
		require.NoError(t, err)
		require.Len(t, retrieved, 1)
	})
}
