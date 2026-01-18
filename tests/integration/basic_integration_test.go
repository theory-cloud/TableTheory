// Package integration contains integration tests for TableTheory
package integration

import (
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory"
	"github.com/theory-cloud/tabletheory/pkg/session"
)

// TestBasicOperations tests the core CRUD operations
func TestBasicOperations(t *testing.T) {
	// Initialize test context with automatic cleanup
	testCtx := InitTestDB(t)

	// Create table with automatic cleanup
	testCtx.CreateTableIfNotExists(t, &TestUser{})

	t.Run("Create", func(t *testing.T) {
		user := &TestUser{
			ID:     "test-user-1",
			Email:  "test@example.com",
			Name:   "Test User",
			Active: true,
		}

		err := testCtx.DB.Model(user).Create()
		assert.NoError(t, err)
		assert.NotZero(t, user.CreatedAt)
		assert.NotZero(t, user.UpdatedAt)
	})

	t.Run("Query", func(t *testing.T) {
		var user TestUser
		err := testCtx.DB.Model(&TestUser{}).
			Where("ID", "=", "test-user-1").
			First(&user)

		assert.NoError(t, err)
		assert.Equal(t, "test-user-1", user.ID)
		assert.Equal(t, "test@example.com", user.Email)
		assert.Equal(t, "Test User", user.Name)
		assert.True(t, user.Active)
	})

	t.Run("Update", func(t *testing.T) {
		user := &TestUser{
			ID:     "test-user-1",
			Name:   "Updated Name",
			Active: false,
		}

		err := testCtx.DB.Model(user).
			Where("ID", "=", "test-user-1").
			Update("Name", "Active")

		assert.NoError(t, err)

		// Verify update
		var updated TestUser
		err = testCtx.DB.Model(&TestUser{}).
			Where("ID", "=", "test-user-1").
			First(&updated)

		assert.NoError(t, err)
		assert.Equal(t, "Updated Name", updated.Name)
		assert.False(t, updated.Active)
		assert.Equal(t, "test@example.com", updated.Email) // Unchanged
	})

	t.Run("Delete", func(t *testing.T) {
		err := testCtx.DB.Model(&TestUser{}).
			Where("ID", "=", "test-user-1").
			Delete()

		assert.NoError(t, err)

		// Verify deletion
		var deleted TestUser
		err = testCtx.DB.Model(&TestUser{}).
			Where("ID", "=", "test-user-1").
			First(&deleted)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

// TestNilPointerScenarios specifically tests scenarios that might cause nil pointer dereference
func TestNilPointerScenarios(t *testing.T) {
	if os.Getenv("SKIP_INTEGRATION") == "true" {
		t.Skip("Integration tests disabled")
	}

	t.Run("MinimalConfig", func(t *testing.T) {
		// Test with minimal config that was causing issues
		sessionConfig := session.Config{
			Region: "us-east-1",
		}

		// This should fail without proper AWS setup
		_, err := theorydb.New(sessionConfig)
		// Should create DB without error (but operations will fail without AWS)
		assert.NoError(t, err)
	})

	t.Run("LocalConfig", func(t *testing.T) {
		// Test with local config
		sessionConfig := session.Config{
			Region:   "us-east-1",
			Endpoint: "http://localhost:8000",
			AWSConfigOptions: []func(*config.LoadOptions) error{
				config.WithCredentialsProvider(
					credentials.NewStaticCredentialsProvider("dummy", "dummy", ""),
				),
			},
		}

		db, err := theorydb.New(sessionConfig)
		require.NoError(t, err)
		assert.NotNil(t, db)

		// Create a query - this shouldn't panic
		query := db.Model(&TestUser{})
		assert.NotNil(t, query)
	})
}
