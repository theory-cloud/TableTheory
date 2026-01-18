package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory"
	"github.com/theory-cloud/tabletheory/pkg/session"
	"github.com/theory-cloud/tabletheory/tests"
)

// BatchTestItem represents a test item for batch operations
type BatchTestItem struct {
	CreatedAt time.Time `theorydb:"created_at"`
	UpdatedAt time.Time `theorydb:"updated_at"`
	ID        string    `theorydb:"pk"`
	SKValue   string    `theorydb:"sk"`
	Name      string
	Category  string
	Tags      []string
	Value     int
	Price     float64
	Active    bool
}

func (BatchTestItem) TableName() string {
	return "BatchTestItems"
}

func TestBatchOperations(t *testing.T) {
	// Initialize test context with automatic cleanup
	testCtx := InitTestDB(t)
	ctx := context.Background()

	// Create table with automatic cleanup
	testCtx.CreateTableIfNotExists(t, &BatchTestItem{})

	t.Run("BatchCreate", func(t *testing.T) {
		// Create test items
		items := []BatchTestItem{
			{
				ID:       "batch1",
				SKValue:  "item1",
				Name:     "Batch Item 1",
				Category: "electronics",
				Value:    100,
				Price:    99.99,
				Active:   true,
				Tags:     []string{"new", "featured"},
			},
			{
				ID:       "batch1",
				SKValue:  "item2",
				Name:     "Batch Item 2",
				Category: "electronics",
				Value:    200,
				Price:    199.99,
				Active:   true,
				Tags:     []string{"sale"},
			},
			{
				ID:       "batch1",
				SKValue:  "item3",
				Name:     "Batch Item 3",
				Category: "books",
				Value:    50,
				Price:    24.99,
				Active:   false,
			},
		}

		// Batch create
		err := testCtx.DB.Model(&BatchTestItem{}).WithContext(ctx).BatchCreate(items)
		require.NoError(t, err)

		// Verify all items were created
		var results []BatchTestItem
		err = testCtx.DB.Model(&BatchTestItem{}).
			Where("ID", "=", "batch1").
			WithContext(ctx).
			All(&results)
		require.NoError(t, err)
		assert.Len(t, results, 3)
	})

	t.Run("BatchCreateWithLargeSet", func(t *testing.T) {
		// Create 30 items (exceeds batch limit of 25)
		var items []BatchTestItem
		for i := 0; i < 30; i++ {
			items = append(items, BatchTestItem{
				ID:       "batch2",
				SKValue:  fmt.Sprintf("item%d", i),
				Name:     fmt.Sprintf("Large Batch Item %d", i),
				Category: "test",
				Value:    i * 10,
				Price:    float64(i) * 9.99,
				Active:   i%2 == 0,
			})
		}

		// This should succeed by processing items in batches of 25
		err := testCtx.DB.Model(&BatchTestItem{}).WithContext(ctx).BatchCreate(items)
		assert.NoError(t, err)

		// Verify all 30 items were created
		var results []BatchTestItem
		err = testCtx.DB.Model(&BatchTestItem{}).
			Where("ID", "=", "batch2").
			WithContext(ctx).
			All(&results)
		require.NoError(t, err)
		assert.Len(t, results, 30)
	})

	t.Run("BatchGet", func(t *testing.T) {
		// Setup: Create items first
		setupItems := []BatchTestItem{
			{ID: "batch3", SKValue: "get1", Name: "Get Item 1", Value: 100},
			{ID: "batch3", SKValue: "get2", Name: "Get Item 2", Value: 200},
			{ID: "batch3", SKValue: "get3", Name: "Get Item 3", Value: 300},
		}

		for _, item := range setupItems {
			err := testCtx.DB.Model(&item).WithContext(ctx).Create()
			require.NoError(t, err)
		}

		// Batch get with keys
		keys := []any{
			BatchTestItem{ID: "batch3", SKValue: "get1"},
			BatchTestItem{ID: "batch3", SKValue: "get2"},
			BatchTestItem{ID: "batch3", SKValue: "get3"},
		}

		var results []BatchTestItem
		err := testCtx.DB.Model(&BatchTestItem{}).WithContext(ctx).BatchGet(keys, &results)
		require.NoError(t, err)
		assert.Len(t, results, 3)

		// Verify values
		for _, result := range results {
			assert.Equal(t, "batch3", result.ID)
			assert.Contains(t, []string{"get1", "get2", "get3"}, result.SKValue)
		}
	})

	t.Run("BatchGetLarge", func(t *testing.T) {
		baseID := "batch-large"
		seed := make([]BatchTestItem, 0, 120)
		for i := 0; i < 120; i++ {
			seed = append(seed, BatchTestItem{
				ID:      baseID,
				SKValue: fmt.Sprintf("item-%03d", i),
				Name:    fmt.Sprintf("Large Item %d", i),
				Value:   i,
			})
		}

		require.NoError(t, testCtx.DB.Model(&BatchTestItem{}).WithContext(ctx).BatchCreate(seed))

		totalRequests := 140
		keys := make([]any, 0, totalRequests)
		for i := 0; i < totalRequests; i++ {
			keys = append(keys, tabletheory.NewKeyPair(baseID, fmt.Sprintf("item-%03d", i)))
		}

		opts := tabletheory.DefaultBatchGetOptions()
		opts.ChunkSize = 45
		opts.Parallel = true
		opts.MaxConcurrency = 3

		var progressCalls int
		var lastRetrieved int
		opts.ProgressCallback = func(retrieved, total int) {
			progressCalls++
			lastRetrieved = retrieved
			require.Equal(t, totalRequests, total)
		}

		var results []BatchTestItem
		err := testCtx.DB.Model(&BatchTestItem{}).WithContext(ctx).BatchGetWithOptions(keys, &results, opts)
		require.NoError(t, err)
		assert.Len(t, results, len(seed))
		assert.GreaterOrEqual(t, progressCalls, 3)
		assert.Equal(t, len(seed), lastRetrieved)
	})

	t.Run("BatchDelete", func(t *testing.T) {
		// First, clean up any existing items with the same keys
		cleanupItems := []BatchTestItem{
			{ID: "batch4", SKValue: "del1"},
			{ID: "batch4", SKValue: "del2"},
			{ID: "batch4", SKValue: "del3"},
			{ID: "batch4", SKValue: "keep1"},
		}

		for _, item := range cleanupItems {
			err := testCtx.DB.Model(&BatchTestItem{}).
				Where("ID", "=", item.ID).
				Where("SKValue", "=", item.SKValue).
				WithContext(ctx).
				Delete()
			require.NoError(t, err)
		}

		// Setup: Create items to delete
		setupItems := []BatchTestItem{
			{ID: "batch4", SKValue: "del1", Name: "Delete Item 1"},
			{ID: "batch4", SKValue: "del2", Name: "Delete Item 2"},
			{ID: "batch4", SKValue: "del3", Name: "Delete Item 3"},
			{ID: "batch4", SKValue: "keep1", Name: "Keep Item 1"},
		}

		for _, item := range setupItems {
			err := testCtx.DB.Model(&item).WithContext(ctx).Create()
			require.NoError(t, err)
		}

		// Delete specific items using BatchDelete
		deleteKeys := []any{
			BatchTestItem{ID: "batch4", SKValue: "del1"},
			BatchTestItem{ID: "batch4", SKValue: "del2"},
			BatchTestItem{ID: "batch4", SKValue: "del3"},
		}

		err := testCtx.DB.Model(&BatchTestItem{}).WithContext(ctx).BatchDelete(deleteKeys)
		require.NoError(t, err)

		// Verify items were deleted
		var remaining []BatchTestItem
		err = testCtx.DB.Model(&BatchTestItem{}).
			Where("ID", "=", "batch4").
			WithContext(ctx).
			All(&remaining)
		require.NoError(t, err)
		assert.Len(t, remaining, 1)
		assert.Equal(t, "keep1", remaining[0].SKValue)
	})

	t.Run("BatchWrite_Mixed", func(t *testing.T) {
		// First, clean up any existing items
		cleanupItems := []BatchTestItem{
			{ID: "batch5", SKValue: "put1"},
			{ID: "batch5", SKValue: "put2"},
			{ID: "batch5", SKValue: "del1"},
			{ID: "batch5", SKValue: "del2"},
		}

		for _, item := range cleanupItems {
			err := testCtx.DB.Model(&BatchTestItem{}).
				Where("ID", "=", item.ID).
				Where("SKValue", "=", item.SKValue).
				WithContext(ctx).
				Delete()
			require.NoError(t, err)
		}

		// Setup: Create items to be deleted
		setupItems := []BatchTestItem{
			{ID: "batch5", SKValue: "del1", Name: "To Delete 1"},
			{ID: "batch5", SKValue: "del2", Name: "To Delete 2"},
		}

		for _, item := range setupItems {
			err := testCtx.DB.Model(&item).WithContext(ctx).Create()
			require.NoError(t, err)
		}

		// Items to put
		putItems := []any{
			BatchTestItem{ID: "batch5", SKValue: "put1", Name: "New Put 1"},
			BatchTestItem{ID: "batch5", SKValue: "put2", Name: "New Put 2"},
		}

		// Keys to delete
		deleteKeys := []any{
			BatchTestItem{ID: "batch5", SKValue: "del1"},
			BatchTestItem{ID: "batch5", SKValue: "del2"},
		}

		// Execute mixed batch write
		err := testCtx.DB.Model(&BatchTestItem{}).WithContext(ctx).BatchWrite(putItems, deleteKeys)
		require.NoError(t, err)

		// Verify results
		var results []BatchTestItem
		err = testCtx.DB.Model(&BatchTestItem{}).
			Where("ID", "=", "batch5").
			WithContext(ctx).
			All(&results)
		require.NoError(t, err)
		assert.Len(t, results, 2) // Should only have the put items

		// Verify the put items exist
		for _, result := range results {
			assert.Contains(t, []string{"put1", "put2"}, result.SKValue)
			assert.Contains(t, []string{"New Put 1", "New Put 2"}, result.Name)
		}
	})

	t.Run("BatchOperations_WithOptions", func(t *testing.T) {
		// Create test items for update
		items := []BatchTestItem{
			{ID: "batch6", SKValue: "item1", Name: "Original 1", Value: 100},
			{ID: "batch6", SKValue: "item2", Name: "Original 2", Value: 200},
			{ID: "batch6", SKValue: "item3", Name: "Original 3", Value: 300},
		}

		// Create items first
		for _, item := range items {
			err := testCtx.DB.Model(&item).WithContext(ctx).Create()
			require.NoError(t, err)
		}

		// Update items with new values
		updateItems := []any{
			BatchTestItem{ID: "batch6", SKValue: "item1", Name: "Updated 1", Value: 150},
			BatchTestItem{ID: "batch6", SKValue: "item2", Name: "Updated 2", Value: 250},
			BatchTestItem{ID: "batch6", SKValue: "item3", Name: "Updated 3", Value: 350},
		}

		// Execute batch update with options
		err := testCtx.DB.Model(&BatchTestItem{}).WithContext(ctx).BatchUpdateWithOptions(
			updateItems,
			[]string{"Name", "Value"},
		)
		require.NoError(t, err)

		// Verify updates
		var results []BatchTestItem
		err = testCtx.DB.Model(&BatchTestItem{}).
			Where("ID", "=", "batch6").
			WithContext(ctx).
			All(&results)
		require.NoError(t, err)
		assert.Len(t, results, 3)

		// Check that values were updated
		for _, result := range results {
			switch result.SKValue {
			case "item1":
				assert.Equal(t, "Updated 1", result.Name)
				assert.Equal(t, 150, result.Value)
			case "item2":
				assert.Equal(t, "Updated 2", result.Name)
				assert.Equal(t, 250, result.Value)
			case "item3":
				assert.Equal(t, "Updated 3", result.Name)
				assert.Equal(t, 350, result.Value)
			}
		}
	})
}

// Helper functions

func TestBatchOperationsE2E(t *testing.T) {
	tests.RequireDynamoDBLocal(t)

	// Fixed initialization with session.Config
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

	db, err := tabletheory.New(sessionConfig)
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, db.Close())
	})

	// ... existing code ...
}
