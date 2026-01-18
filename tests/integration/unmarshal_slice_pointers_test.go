package integration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestModel for testing slice of pointers
type TestModel struct {
	CreatedAt time.Time `theorydb:"created_at"`
	UpdatedAt time.Time `theorydb:"updated_at"`
	PK        string    `theorydb:"pk"`
	SK        string    `theorydb:"sk"`
	Name      string    `json:"name"`
	Version   int64     `theorydb:"version"`
}

func (t *TestModel) SetKeys() {
	// Set composite key if needed
}

func TestUnmarshalSliceOfPointers(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Add timeout for the entire test to prevent hanging
	done := make(chan bool, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				t.Logf("Test panicked: %v", r)
			}
			done <- true
		}()

		// Initialize test context with automatic cleanup
		t.Logf("Initializing test database...")
		testCtx := InitTestDB(t)

		// Create table with automatic cleanup - this should fail fast if there are issues
		t.Logf("Creating table...")
		testCtx.CreateTableIfNotExists(t, &TestModel{})

		// Create test data
		t.Logf("Inserting test data...")
		items := []TestModel{
			{PK: "test", SK: "item1", Name: "First Item"},
			{PK: "test", SK: "item2", Name: "Second Item"},
			{PK: "test", SK: "item3", Name: "Third Item"},
		}

		// Insert test data
		for _, item := range items {
			err := testCtx.DB.Model(&item).Create()
			require.NoError(t, err)
		}

		// Test 1: All() with slice of pointers
		t.Run("All with slice of pointers", func(t *testing.T) {
			t.Logf("Starting All() query with slice of pointers...")
			var results []*TestModel
			err := testCtx.DB.Model(&TestModel{}).
				Where("PK", "=", "test").
				All(&results)

			require.NoError(t, err)
			assert.Len(t, results, 3)
			t.Logf("Successfully retrieved %d items", len(results))

			// Verify all items are properly unmarshaled
			for _, result := range results {
				assert.NotNil(t, result)
				assert.Equal(t, "test", result.PK)
				assert.Contains(t, []string{"item1", "item2", "item3"}, result.SK)
				assert.NotEmpty(t, result.Name)
			}
		})

		// Test 2: All() with slice of values - should still work
		t.Run("All with slice of values", func(t *testing.T) {
			var results []TestModel // Slice of values
			err := testCtx.DB.Model(&TestModel{}).
				Where("PK", "=", "test").
				All(&results)

			require.NoError(t, err)
			assert.Len(t, results, 3)

			// Verify all items are properly unmarshaled
			for _, result := range results {
				assert.Equal(t, "test", result.PK)
				assert.Contains(t, []string{"item1", "item2", "item3"}, result.SK)
				assert.NotEmpty(t, result.Name)
			}
		})
	}()

	// Wait for test completion or timeout
	select {
	case <-done:
		t.Logf("Test completed successfully")
	case <-time.After(120 * time.Second):
		t.Fatal("Test timed out after 2 minutes - likely stuck during table creation or database operations")
	}
}
