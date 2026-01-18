package integration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// UpdateProduct model for testing update operations
type UpdateProduct struct {
	LastModified time.Time `theorydb:"updated_at"`
	CreatedAt    time.Time `theorydb:"created_at"`
	Category     string    `theorydb:"sk"`
	Name         string    `theorydb:"attr:productName"`
	ID           string    `theorydb:"pk"`
	Description  string    `theorydb:"attr:description,omitempty"`
	Tags         []string  `theorydb:"attr:tags,set,omitempty"`
	Features     []string  `theorydb:"attr:features"`
	Ratings      []float64 `theorydb:"attr:ratings"`
	Price        float64   `theorydb:"attr:price"`
	Version      int64     `theorydb:"version"`
	Discount     float64   `theorydb:"attr:discount,omitempty"`
	Stock        int       `theorydb:"attr:stockCount"`
	Active       bool      `theorydb:"attr:isActive"`
}

// UserProfile model for testing complex updates
type UserProfile struct {
	LastLogin    time.Time         `theorydb:"attr:lastLogin"`
	Settings     map[string]string `theorydb:"attr:settings"`
	UserID       string            `theorydb:"pk"`
	Email        string            `theorydb:"sk"`
	Username     string            `theorydb:"attr:username"`
	FullName     string            `theorydb:"attr:fullName,omitempty"`
	Achievements []string          `theorydb:"attr:achievements"`
	Age          int               `theorydb:"attr:age,omitempty"`
	Score        float64           `theorydb:"attr:score"`
	LoginCount   int64             `theorydb:"attr:loginCount"`
	Version      int64             `theorydb:"version"`
}

func TestUpdateOperations_Set(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	testCtx := InitTestDB(t)

	// Create table with automatic cleanup
	testCtx.CreateTableIfNotExists(t, &UpdateProduct{})

	// Create initial product
	product := &UpdateProduct{
		ID:           "PROD-001",
		Category:     "Electronics",
		Name:         "Laptop",
		Price:        999.99,
		Stock:        10,
		Tags:         []string{"portable", "computer"},
		Active:       true,
		Version:      1,
		CreatedAt:    time.Now(),
		LastModified: time.Now(),
	}

	err := testCtx.DB.Model(product).Create()
	require.NoError(t, err)

	t.Run("Set single field", func(t *testing.T) {
		err := testCtx.DB.Model(&UpdateProduct{}).
			Where("ID", "=", "PROD-001").
			Where("Category", "=", "Electronics").
			UpdateBuilder().
			Set("Name", "Gaming Laptop").
			Execute()

		assert.NoError(t, err)

		// Verify update
		var updated UpdateProduct
		err = testCtx.DB.Model(&UpdateProduct{}).
			Where("ID", "=", "PROD-001").
			Where("Category", "=", "Electronics").
			First(&updated)

		assert.NoError(t, err)
		assert.Equal(t, "Gaming Laptop", updated.Name)
		assert.Equal(t, 999.99, updated.Price) // Unchanged
	})

	t.Run("Set multiple fields", func(t *testing.T) {
		err := testCtx.DB.Model(&UpdateProduct{}).
			Where("ID", "=", "PROD-001").
			Where("Category", "=", "Electronics").
			UpdateBuilder().
			Set("Price", 1299.99).
			Set("Description", "High-performance gaming laptop").
			Set("Discount", 0.15).
			Execute()

		assert.NoError(t, err)

		// Verify updates
		var updated UpdateProduct
		err = testCtx.DB.Model(&UpdateProduct{}).
			Where("ID", "=", "PROD-001").
			Where("Category", "=", "Electronics").
			First(&updated)

		assert.NoError(t, err)
		assert.Equal(t, 1299.99, updated.Price)
		assert.Equal(t, "High-performance gaming laptop", updated.Description)
		assert.Equal(t, 0.15, updated.Discount)
	})

	t.Run("Set with return values", func(t *testing.T) {
		var result UpdateProduct
		err := testCtx.DB.Model(&UpdateProduct{}).
			Where("ID", "=", "PROD-001").
			Where("Category", "=", "Electronics").
			UpdateBuilder().
			Set("Stock", 25).
			ReturnValues("ALL_NEW").
			ExecuteWithResult(&result)

		assert.NoError(t, err)
		assert.Equal(t, "PROD-001", result.ID)
		assert.Equal(t, 25, result.Stock)
		assert.Equal(t, "Gaming Laptop", result.Name)
	})
}

func TestUpdateOperations_AtomicCounters(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	testCtx := InitTestDB(t)

	// Create table with automatic cleanup
	testCtx.CreateTableIfNotExists(t, &UserProfile{})

	// Create initial user
	user := &UserProfile{
		UserID:     "USER-001",
		Email:      "test@example.com",
		Username:   "testuser",
		FullName:   "Test User",
		Age:        25,
		Score:      100.0,
		LoginCount: 5,
		Version:    1,
	}

	err := testCtx.DB.Model(user).Create()
	require.NoError(t, err)

	t.Run("Add to numeric fields", func(t *testing.T) {
		err := testCtx.DB.Model(&UserProfile{}).
			Where("UserID", "=", "USER-001").
			Where("Email", "=", "test@example.com").
			UpdateBuilder().
			Add("LoginCount", 1).
			Add("Score", 25.5).
			Set("LastLogin", time.Now()).
			Execute()

		assert.NoError(t, err)

		// Verify
		var updated UserProfile
		err = testCtx.DB.Model(&UserProfile{}).
			Where("UserID", "=", "USER-001").
			Where("Email", "=", "test@example.com").
			First(&updated)

		assert.NoError(t, err)
		assert.Equal(t, int64(6), updated.LoginCount)
		assert.Equal(t, 125.5, updated.Score)
	})

	t.Run("Increment and Decrement", func(t *testing.T) {
		err := testCtx.DB.Model(&UserProfile{}).
			Where("UserID", "=", "USER-001").
			Where("Email", "=", "test@example.com").
			UpdateBuilder().
			Increment("Age").
			Execute()

		assert.NoError(t, err)

		// Verify increment
		var updated UserProfile
		err = testCtx.DB.Model(&UserProfile{}).
			Where("UserID", "=", "USER-001").
			Where("Email", "=", "test@example.com").
			First(&updated)

		assert.NoError(t, err)
		assert.Equal(t, 26, updated.Age)

		// Test decrement
		err = testCtx.DB.Model(&UserProfile{}).
			Where("UserID", "=", "USER-001").
			Where("Email", "=", "test@example.com").
			UpdateBuilder().
			Add("Age", -2). // Decrement by 2
			Execute()

		assert.NoError(t, err)

		err = testCtx.DB.Model(&UserProfile{}).
			Where("UserID", "=", "USER-001").
			Where("Email", "=", "test@example.com").
			First(&updated)

		assert.NoError(t, err)
		assert.Equal(t, 24, updated.Age)
	})
}

func TestUpdateOperations_ListOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	testCtx := InitTestDB(t)

	// Create tables with automatic cleanup
	testCtx.CreateTableIfNotExists(t, &UpdateProduct{})
	testCtx.CreateTableIfNotExists(t, &UserProfile{})

	// Create product with initial lists
	product := &UpdateProduct{
		ID:       "PROD-002",
		Category: "Electronics",
		Name:     "Smartphone",
		Price:    699.99,
		Stock:    50,
		Features: []string{"5G", "128GB Storage"},
		Ratings:  []float64{4.5, 4.0, 5.0},
		Version:  1,
	}

	err := testCtx.DB.Model(product).Create()
	require.NoError(t, err)

	t.Run("AppendToList", func(t *testing.T) {
		err := testCtx.DB.Model(&UpdateProduct{}).
			Where("ID", "=", "PROD-002").
			Where("Category", "=", "Electronics").
			UpdateBuilder().
			AppendToList("Features", []string{"Wireless Charging", "Face ID"}).
			Execute()

		assert.NoError(t, err)

		// Verify
		var updated UpdateProduct
		err = testCtx.DB.Model(&UpdateProduct{}).
			Where("ID", "=", "PROD-002").
			Where("Category", "=", "Electronics").
			First(&updated)

		assert.NoError(t, err)
		assert.Equal(t, []string{"5G", "128GB Storage", "Wireless Charging", "Face ID"}, updated.Features)
	})

	t.Run("PrependToList", func(t *testing.T) {
		// Create user with achievements
		user := &UserProfile{
			UserID:       "USER-002",
			Email:        "gamer@example.com",
			Username:     "progamer",
			Achievements: []string{"First Win", "10 Wins"},
			Version:      1,
		}
		err := testCtx.DB.Model(user).Create()
		require.NoError(t, err)

		err = testCtx.DB.Model(&UserProfile{}).
			Where("UserID", "=", "USER-002").
			Where("Email", "=", "gamer@example.com").
			UpdateBuilder().
			PrependToList("Achievements", []string{"Tutorial Complete"}).
			Execute()

		assert.NoError(t, err)

		// Verify
		var updated UserProfile
		err = testCtx.DB.Model(&UserProfile{}).
			Where("UserID", "=", "USER-002").
			Where("Email", "=", "gamer@example.com").
			First(&updated)

		assert.NoError(t, err)
		assert.Equal(t, []string{"Tutorial Complete", "First Win", "10 Wins"}, updated.Achievements)
	})

	t.Run("RemoveFromListAt", func(t *testing.T) {
		// Remove rating at index 1 (4.0) from [4.5, 4.0, 5.0]
		err := testCtx.DB.Model(&UpdateProduct{}).
			Where("ID", "=", "PROD-002").
			Where("Category", "=", "Electronics").
			UpdateBuilder().
			RemoveFromListAt("Ratings", 1).
			Execute()

		assert.NoError(t, err)

		// Verify
		var updated UpdateProduct
		err = testCtx.DB.Model(&UpdateProduct{}).
			Where("ID", "=", "PROD-002").
			Where("Category", "=", "Electronics").
			First(&updated)

		assert.NoError(t, err)
		assert.Equal(t, []float64{4.5, 5.0}, updated.Ratings)
	})

	t.Run("SetListElement", func(t *testing.T) {
		// Update a specific feature at index 1
		err := testCtx.DB.Model(&UpdateProduct{}).
			Where("ID", "=", "PROD-002").
			Where("Category", "=", "Electronics").
			UpdateBuilder().
			SetListElement("Features", 1, "256GB Storage").
			Execute()

		assert.NoError(t, err)

		// Verify
		var updated UpdateProduct
		err = testCtx.DB.Model(&UpdateProduct{}).
			Where("ID", "=", "PROD-002").
			Where("Category", "=", "Electronics").
			First(&updated)

		assert.NoError(t, err)
		assert.Equal(t, "256GB Storage", updated.Features[1])
	})
}

func TestUpdateOperations_RemoveAndDelete(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	testCtx := InitTestDB(t)

	// Create table with automatic cleanup
	testCtx.CreateTableIfNotExists(t, &UpdateProduct{})

	// Create product with optional fields
	product := &UpdateProduct{
		ID:          "PROD-003",
		Category:    "Books",
		Name:        "Programming Guide",
		Price:       49.99,
		Stock:       100,
		Tags:        []string{"programming", "golang", "tutorial"},
		Description: "Complete Go programming guide",
		Discount:    0.20,
		Version:     1,
	}

	err := testCtx.DB.Model(product).Create()
	require.NoError(t, err)

	t.Run("Remove attributes", func(t *testing.T) {
		err := testCtx.DB.Model(&UpdateProduct{}).
			Where("ID", "=", "PROD-003").
			Where("Category", "=", "Books").
			UpdateBuilder().
			Remove("Description").
			Remove("Discount").
			Execute()

		assert.NoError(t, err)

		// Verify
		var updated UpdateProduct
		err = testCtx.DB.Model(&UpdateProduct{}).
			Where("ID", "=", "PROD-003").
			Where("Category", "=", "Books").
			First(&updated)

		assert.NoError(t, err)
		assert.Empty(t, updated.Description)
		assert.Equal(t, 0.0, updated.Discount)
	})

	t.Run("Delete from set", func(t *testing.T) {
		err := testCtx.DB.Model(&UpdateProduct{}).
			Where("ID", "=", "PROD-003").
			Where("Category", "=", "Books").
			UpdateBuilder().
			Delete("Tags", "tutorial"). // Delete single string from set
			Execute()

		assert.NoError(t, err)

		// Verify
		var updated UpdateProduct
		err = testCtx.DB.Model(&UpdateProduct{}).
			Where("ID", "=", "PROD-003").
			Where("Category", "=", "Books").
			First(&updated)

		assert.NoError(t, err)
		assert.ElementsMatch(t, []string{"programming", "golang"}, updated.Tags)
	})
}

func TestUpdateOperations_ConditionalUpdates(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	testCtx := InitTestDB(t)

	// Create table with automatic cleanup
	testCtx.CreateTableIfNotExists(t, &UpdateProduct{})

	// Create product
	product := &UpdateProduct{
		ID:       "PROD-004",
		Category: "Clothing",
		Name:     "T-Shirt",
		Price:    29.99,
		Stock:    5,
		Active:   true,
		Version:  1,
	}

	err := testCtx.DB.Model(product).Create()
	require.NoError(t, err)

	t.Run("Condition on field value", func(t *testing.T) {
		// Only update if stock is less than 10
		err := testCtx.DB.Model(&UpdateProduct{}).
			Where("ID", "=", "PROD-004").
			Where("Category", "=", "Clothing").
			UpdateBuilder().
			Set("Stock", 50).
			Condition("Stock", "<", 10).
			Execute()

		assert.NoError(t, err)

		// Verify update happened
		var updated UpdateProduct
		err = testCtx.DB.Model(&UpdateProduct{}).
			Where("ID", "=", "PROD-004").
			Where("Category", "=", "Clothing").
			First(&updated)

		assert.NoError(t, err)
		assert.Equal(t, 50, updated.Stock)
	})

	t.Run("Condition fails", func(t *testing.T) {
		// Try to update only if price > 100 (should fail)
		err := testCtx.DB.Model(&UpdateProduct{}).
			Where("ID", "=", "PROD-004").
			Where("Category", "=", "Clothing").
			UpdateBuilder().
			Set("Price", 19.99).
			Condition("Price", ">", 100).
			Execute()

		// Should get a conditional check failed error
		assert.Error(t, err)

		// Verify price unchanged
		var unchanged UpdateProduct
		err = testCtx.DB.Model(&UpdateProduct{}).
			Where("ID", "=", "PROD-004").
			Where("Category", "=", "Clothing").
			First(&unchanged)

		assert.NoError(t, err)
		assert.Equal(t, 29.99, unchanged.Price)
	})

	t.Run("ConditionExists", func(t *testing.T) {
		// Only update if Active field exists (it should since we created it with Active: true)
		err := testCtx.DB.Model(&UpdateProduct{}).
			Where("ID", "=", "PROD-004").
			Where("Category", "=", "Clothing").
			UpdateBuilder().
			Set("Active", false).
			ConditionExists("Active").
			Execute()

		assert.NoError(t, err)

		// Verify
		var updated UpdateProduct
		err = testCtx.DB.Model(&UpdateProduct{}).
			Where("ID", "=", "PROD-004").
			Where("Category", "=", "Clothing").
			First(&updated)

		assert.NoError(t, err)
		assert.False(t, updated.Active)
	})

	t.Run("ConditionNotExists", func(t *testing.T) {
		// Try to set Description only if it doesn't exist (it shouldn't exist initially)
		err := testCtx.DB.Model(&UpdateProduct{}).
			Where("ID", "=", "PROD-004").
			Where("Category", "=", "Clothing").
			UpdateBuilder().
			Set("Description", "Basic T-Shirt").
			ConditionNotExists("Description").
			Execute()

		assert.NoError(t, err)

		// Verify Description was set
		var updated UpdateProduct
		err = testCtx.DB.Model(&UpdateProduct{}).
			Where("ID", "=", "PROD-004").
			Where("Category", "=", "Clothing").
			First(&updated)
		assert.NoError(t, err)
		assert.Equal(t, "Basic T-Shirt", updated.Description)

		// Try again - should fail now because Description exists
		err = testCtx.DB.Model(&UpdateProduct{}).
			Where("ID", "=", "PROD-004").
			Where("Category", "=", "Clothing").
			UpdateBuilder().
			Set("Description", "Another description").
			ConditionNotExists("Description").
			Execute()

		assert.Error(t, err, "Should fail because Description now exists")
	})
}

func TestUpdateOperations_OptimisticLocking(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	testCtx := InitTestDB(t)

	// Create table with automatic cleanup
	testCtx.CreateTableIfNotExists(t, &UpdateProduct{})

	// Create product
	product := &UpdateProduct{
		ID:       "PROD-005",
		Category: "Sports",
		Name:     "Basketball",
		Price:    39.99,
		Stock:    20,
		Version:  1,
	}

	err := testCtx.DB.Model(product).Create()
	require.NoError(t, err)

	t.Run("Successful version update", func(t *testing.T) {
		err := testCtx.DB.Model(&UpdateProduct{}).
			Where("ID", "=", "PROD-005").
			Where("Category", "=", "Sports").
			UpdateBuilder().
			Set("Price", 44.99).
			Add("Version", 1).
			ConditionVersion(1).
			Execute()

		assert.NoError(t, err)

		// Verify version incremented
		var updated UpdateProduct
		err = testCtx.DB.Model(&UpdateProduct{}).
			Where("ID", "=", "PROD-005").
			Where("Category", "=", "Sports").
			First(&updated)

		assert.NoError(t, err)
		assert.Equal(t, int64(2), updated.Version)
		assert.Equal(t, 44.99, updated.Price)
	})

	t.Run("Version conflict", func(t *testing.T) {
		// Try to update with old version
		err := testCtx.DB.Model(&UpdateProduct{}).
			Where("ID", "=", "PROD-005").
			Where("Category", "=", "Sports").
			UpdateBuilder().
			Set("Price", 49.99).
			Add("Version", 1).
			ConditionVersion(1). // Old version
			Execute()

		// Should fail due to version mismatch
		assert.Error(t, err)

		// Verify price unchanged
		var unchanged UpdateProduct
		err = testCtx.DB.Model(&UpdateProduct{}).
			Where("ID", "=", "PROD-005").
			Where("Category", "=", "Sports").
			First(&unchanged)

		assert.NoError(t, err)
		assert.Equal(t, 44.99, unchanged.Price)
		assert.Equal(t, int64(2), unchanged.Version)
	})
}

func TestUpdateOperations_ReturnValues(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	testCtx := InitTestDB(t)

	// Create table with automatic cleanup
	testCtx.CreateTableIfNotExists(t, &UpdateProduct{})

	// Create product
	product := &UpdateProduct{
		ID:       "PROD-006",
		Category: "Food",
		Name:     "Coffee",
		Price:    12.99,
		Stock:    100,
		Version:  1,
	}

	err := testCtx.DB.Model(product).Create()
	require.NoError(t, err)

	t.Run("ALL_NEW return values", func(t *testing.T) {
		var result UpdateProduct
		err := testCtx.DB.Model(&UpdateProduct{}).
			Where("ID", "=", "PROD-006").
			Where("Category", "=", "Food").
			UpdateBuilder().
			Set("Price", 14.99).
			Set("Name", "Premium Coffee").
			ReturnValues("ALL_NEW").
			ExecuteWithResult(&result)

		assert.NoError(t, err)
		assert.Equal(t, "PROD-006", result.ID)
		assert.Equal(t, "Premium Coffee", result.Name)
		assert.Equal(t, 14.99, result.Price)
		assert.Equal(t, 100, result.Stock)
	})

	t.Run("ALL_OLD return values", func(t *testing.T) {
		var result UpdateProduct
		err := testCtx.DB.Model(&UpdateProduct{}).
			Where("ID", "=", "PROD-006").
			Where("Category", "=", "Food").
			UpdateBuilder().
			Set("Stock", 90).
			ReturnValues("ALL_OLD").
			ExecuteWithResult(&result)

		assert.NoError(t, err)
		assert.Equal(t, "PROD-006", result.ID)
		assert.Equal(t, "Premium Coffee", result.Name)
		assert.Equal(t, 100, result.Stock) // Old value
	})

	t.Run("UPDATED_NEW return values", func(t *testing.T) {
		// Use a struct instead of map to avoid unmarshaling issues
		var result UpdateProduct
		err := testCtx.DB.Model(&UpdateProduct{}).
			Where("ID", "=", "PROD-006").
			Where("Category", "=", "Food").
			UpdateBuilder().
			Set("Price", 13.99).
			ReturnValues("UPDATED_NEW").
			ExecuteWithResult(&result)

		assert.NoError(t, err)
		// With UPDATED_NEW, only the updated fields should be non-zero
		assert.Equal(t, 13.99, result.Price)
		// Other fields should be zero values since they weren't updated
		assert.Empty(t, result.ID) // Should be empty since not updated
	})
}

func TestUpdateOperations_ComplexScenarios(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	testCtx := InitTestDB(t)

	// Create table with automatic cleanup
	testCtx.CreateTableIfNotExists(t, &UserProfile{})

	// Create user with complex data
	user := &UserProfile{
		UserID:   "USER-003",
		Email:    "complex@example.com",
		Username: "complexuser",
		Settings: map[string]string{
			"theme":         "dark",
			"notifications": "enabled",
			"language":      "en",
		},
		Achievements: []string{"Beginner", "Explorer"},
		Score:        250.0,
		LoginCount:   10,
		Version:      1,
	}

	err := testCtx.DB.Model(user).Create()
	require.NoError(t, err)

	t.Run("Multiple operations in single update", func(t *testing.T) {
		var result UserProfile
		err := testCtx.DB.Model(&UserProfile{}).
			Where("UserID", "=", "USER-003").
			Where("Email", "=", "complex@example.com").
			UpdateBuilder().
			Set("Username", "poweruser").
			Add("Score", 50.5).
			Increment("LoginCount").
			AppendToList("Achievements", []string{"Advanced", "Expert"}).
			Set("LastLogin", time.Now()).
			Add("Version", 1).
			ConditionVersion(1).
			ReturnValues("ALL_NEW").
			ExecuteWithResult(&result)

		assert.NoError(t, err)
		assert.Equal(t, "poweruser", result.Username)
		assert.Equal(t, 300.5, result.Score)
		assert.Equal(t, int64(11), result.LoginCount)
		assert.Equal(t, []string{"Beginner", "Explorer", "Advanced", "Expert"}, result.Achievements)
		assert.Equal(t, int64(2), result.Version)
	})

	t.Run("SetIfNotExists with complex update", func(t *testing.T) {
		// Create a new user without FullName
		newUser := &UserProfile{
			UserID:   "USER-004",
			Email:    "new@example.com",
			Username: "newuser",
			Version:  1,
		}
		err := testCtx.DB.Model(newUser).Create()
		require.NoError(t, err)

		// Update with SetIfNotExists
		err = testCtx.DB.Model(&UserProfile{}).
			Where("UserID", "=", "USER-004").
			Where("Email", "=", "new@example.com").
			UpdateBuilder().
			SetIfNotExists("FullName", "", "Default Name").
			SetIfNotExists("Age", 0, 18).
			Set("Username", "updateduser").
			Execute()

		assert.NoError(t, err)

		// Verify
		var updated UserProfile
		err = testCtx.DB.Model(&UserProfile{}).
			Where("UserID", "=", "USER-004").
			Where("Email", "=", "new@example.com").
			First(&updated)

		assert.NoError(t, err)
		assert.Equal(t, "Default Name", updated.FullName)
		assert.Equal(t, 18, updated.Age)
		assert.Equal(t, "updateduser", updated.Username)
	})
}

func TestUpdateOperations_ErrorCases(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	testCtx := InitTestDB(t)

	// Create table with automatic cleanup
	testCtx.CreateTableIfNotExists(t, &UpdateProduct{})

	t.Run("Update non-existent item", func(t *testing.T) {
		err := testCtx.DB.Model(&UpdateProduct{}).
			Where("ID", "=", "NONEXISTENT").
			Where("Category", "=", "NONE").
			UpdateBuilder().
			Set("Name", "Should Fail").
			ConditionExists("ID"). // This will fail
			Execute()

		assert.Error(t, err)
	})

	t.Run("Invalid list index", func(t *testing.T) {
		// Create product with small list
		product := &UpdateProduct{
			ID:       "PROD-ERR",
			Category: "Test",
			Features: []string{"Feature1"},
			Version:  1,
		}
		err := testCtx.DB.Model(product).Create()
		require.NoError(t, err)

		// Try to update invalid index - DynamoDB extends lists automatically
		err = testCtx.DB.Model(&UpdateProduct{}).
			Where("ID", "=", "PROD-ERR").
			Where("Category", "=", "Test").
			UpdateBuilder().
			SetListElement("Features", 10, "Invalid"). // Index out of bounds
			Execute()

		// DynamoDB allows this and extends the list automatically
		assert.NoError(t, err)

		// Verify that DynamoDB extended the list (this is expected behavior)
		var result UpdateProduct
		err = testCtx.DB.Model(&UpdateProduct{}).
			Where("ID", "=", "PROD-ERR").
			Where("Category", "=", "Test").
			First(&result)
		assert.NoError(t, err)

		// DynamoDB extends the list, filling gaps with NULLs that get filtered out in Go
		// So we expect at least the original element plus the new one
		assert.GreaterOrEqual(t, len(result.Features), 2, "List should be extended")
		assert.Equal(t, "Feature1", result.Features[0], "Original element should remain")
		// The "Invalid" element should be somewhere in the extended list
		assert.Contains(t, result.Features, "Invalid", "New element should be added")
	})

	t.Run("Type mismatch in Add operation", func(t *testing.T) {
		product := &UpdateProduct{
			ID:       "PROD-TYPE",
			Category: "Test",
			Name:     "Type Test",
			Version:  1,
		}
		err := testCtx.DB.Model(product).Create()
		require.NoError(t, err)

		// Try to add to a string field (should fail)
		err = testCtx.DB.Model(&UpdateProduct{}).
			Where("ID", "=", "PROD-TYPE").
			Where("Category", "=", "Test").
			UpdateBuilder().
			Add("Name", 10). // Can't add number to string
			Execute()

		assert.Error(t, err)
	})
}

// TestUpdateOperations_ExecuteWithResultAutoReturnValues tests the bug fix where
// ExecuteWithResult should automatically set ReturnValues to ALL_NEW when not explicitly set
func TestUpdateOperations_ExecuteWithResultAutoReturnValues(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	testCtx := InitTestDB(t)

	// Create table with automatic cleanup
	testCtx.CreateTableIfNotExists(t, &UpdateProduct{})

	t.Run("ExecuteWithResult returns values after Add without explicit ReturnValues", func(t *testing.T) {
		// Create initial product
		product := &UpdateProduct{
			ID:       "PROD-ADD-TEST",
			Category: "TestCategory",
			Name:     "Test Product",
			Price:    10.0,
			Stock:    5,
			Version:  1,
		}
		err := testCtx.DB.Model(product).Create()
		require.NoError(t, err)

		// Update with Add operation and ExecuteWithResult WITHOUT setting ReturnValues
		var result UpdateProduct
		err = testCtx.DB.Model(&UpdateProduct{}).
			Where("ID", "=", "PROD-ADD-TEST").
			Where("Category", "=", "TestCategory").
			UpdateBuilder().
			Add("Stock", 3). // Atomic increment
			Set("LastModified", time.Now()).
			ExecuteWithResult(&result)

		// Should succeed
		require.NoError(t, err)

		// Verify the result contains updated values (not zero values)
		assert.Equal(t, "PROD-ADD-TEST", result.ID, "ID should be populated")
		assert.Equal(t, "TestCategory", result.Category, "Category should be populated")
		assert.Equal(t, "Test Product", result.Name, "Name should be populated")
		assert.Equal(t, 10.0, result.Price, "Price should be populated")
		assert.Equal(t, 8, result.Stock, "Stock should be 8 after adding 3 to 5")
		assert.Equal(t, int64(1), result.Version, "Version should be populated")
	})

	t.Run("ExecuteWithResult with multiple atomic operations", func(t *testing.T) {
		// Create user profile
		user := &UserProfile{
			UserID:     "USER-ATOMIC-TEST",
			Email:      "atomic@test.com",
			Username:   "atomicuser",
			Score:      100.0,
			LoginCount: 5,
			Version:    1,
		}
		testCtx.CreateTableIfNotExists(t, &UserProfile{})
		err := testCtx.DB.Model(user).Create()
		require.NoError(t, err)

		// Multiple atomic operations with ExecuteWithResult
		var result UserProfile
		err = testCtx.DB.Model(&UserProfile{}).
			Where("UserID", "=", "USER-ATOMIC-TEST").
			Where("Email", "=", "atomic@test.com").
			UpdateBuilder().
			Add("Score", 50.5).      // Add to score
			Increment("LoginCount"). // Increment login count
			Set("LastLogin", time.Now()).
			ExecuteWithResult(&result)

		// Should succeed
		require.NoError(t, err)

		// All fields should be populated
		assert.Equal(t, "USER-ATOMIC-TEST", result.UserID, "UserID should be populated")
		assert.Equal(t, "atomic@test.com", result.Email, "Email should be populated")
		assert.Equal(t, "atomicuser", result.Username, "Username should be populated")
		assert.Equal(t, 150.5, result.Score, "Score should be 150.5 after adding 50.5 to 100")
		assert.Equal(t, int64(6), result.LoginCount, "LoginCount should be 6 after increment")
		assert.NotZero(t, result.LastLogin, "LastLogin should be set")
	})

	t.Run("ExecuteWithResult with conditional Add operation", func(t *testing.T) {
		// Test with conditional update to ensure it works with conditions too
		product := &UpdateProduct{
			ID:       "PROD-COND-ADD",
			Category: "Conditional",
			Stock:    10,
			Version:  1,
		}
		err := testCtx.DB.Model(product).Create()
		require.NoError(t, err)

		var result UpdateProduct
		err = testCtx.DB.Model(&UpdateProduct{}).
			Where("ID", "=", "PROD-COND-ADD").
			Where("Category", "=", "Conditional").
			UpdateBuilder().
			Add("Stock", -5).            // Decrement stock
			Condition("Stock", ">=", 5). // Only if stock >= 5
			ExecuteWithResult(&result)

		require.NoError(t, err)
		assert.Equal(t, 5, result.Stock, "Stock should be 5 after subtracting 5 from 10")
		assert.Equal(t, "PROD-COND-ADD", result.ID, "ID should be populated")
	})
}
