package schema_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory"
	"github.com/theory-cloud/tabletheory/pkg/session"
	"github.com/theory-cloud/tabletheory/tests"
)

// Test models
type UserV1 struct {
	ID    string `theorydb:"pk"`
	Email string `theorydb:""`
	Name  string `theorydb:""`
}

type UserV2 struct {
	UpdatedAt time.Time `theorydb:"updated_at"`
	ID        string    `theorydb:"pk"`
	Email     string    `theorydb:""`
	FirstName string    `theorydb:""`
	LastName  string    `theorydb:""`
}

type tableDeleter interface {
	DeleteTable(model any) error
}

func deleteTableIfExists(t *testing.T, db tableDeleter, model any) {
	t.Helper()
	if err := db.DeleteTable(model); err != nil {
		var notFoundErr *types.ResourceNotFoundException
		if errors.As(err, &notFoundErr) {
			return
		}
		require.NoError(t, err)
	}
}

func TestAutoMigrateWithOptions(t *testing.T) {
	tests.RequireDynamoDBLocal(t)
	// Skip if not running integration tests
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	config := session.Config{
		Region:              "us-east-1",
		Endpoint:            "http://localhost:8000", // DynamoDB Local
		CredentialsProvider: credentials.NewStaticCredentialsProvider("dummy", "dummy", "dummy"),
	}

	db, err := tabletheory.New(config)
	require.NoError(t, err)

	t.Run("SimpleTableCreation", func(t *testing.T) {
		// Clean up any existing table
		deleteTableIfExists(t, db, &UserV1{})

		// Simple auto-migrate should create table
		err := db.AutoMigrate(&UserV1{})
		assert.NoError(t, err)

		// Verify table exists
		desc, err := db.DescribeTable(&UserV1{})
		assert.NoError(t, err)
		assert.NotNil(t, desc)

		// Clean up
		deleteTableIfExists(t, db, &UserV1{})
	})

	t.Run("AutoMigrateWithBackup", func(t *testing.T) {
		t.Skip("Backup functionality not fully implemented")

		// Clean up any existing tables
		deleteTableIfExists(t, db, &UserV1{})
		deleteTableIfExists(t, db, "UserV1_backup")
	})

	t.Run("AutoMigrateWithTransform", func(t *testing.T) {
		// Clean up any existing tables
		deleteTableIfExists(t, db, &UserV1{})
		deleteTableIfExists(t, db, &UserV2{})

		// Create and populate V1 table
		err := db.CreateTable(&UserV1{})
		require.NoError(t, err)

		// Add test data
		users := []*UserV1{
			{ID: "1", Email: "john@example.com", Name: "John Doe"},
			{ID: "2", Email: "jane@example.com", Name: "Jane Smith"},
		}

		for _, u := range users {
			err = db.Model(u).Create()
			require.NoError(t, err)
		}

		// Define transformation function
		transformFunc := func(old UserV1) UserV2 {
			// Split name into first and last
			var firstName, lastName string
			if old.Name != "" {
				parts := split(old.Name, " ")
				if len(parts) > 0 {
					firstName = parts[0]
				}
				if len(parts) > 1 {
					lastName = parts[1]
				}
			}

			return UserV2{
				ID:        old.ID,
				Email:     old.Email,
				FirstName: firstName,
				LastName:  lastName,
				UpdatedAt: time.Now(),
			}
		}

		// Migrate to V2 with transformation
		err = db.AutoMigrateWithOptions(&UserV1{},
			tabletheory.WithTargetModel(&UserV2{}),
			tabletheory.WithDataCopy(true),
			tabletheory.WithTransform(transformFunc),
		)
		require.NoError(t, err)

		// Note: The transform function is not fully implemented in the current version
		// This test demonstrates the intended API

		// Clean up
		deleteTableIfExists(t, db, &UserV1{})
		deleteTableIfExists(t, db, &UserV2{})
	})

	t.Run("AutoMigrateIdempotent", func(t *testing.T) {
		// Clean up any existing table
		deleteTableIfExists(t, db, &UserV1{})

		// First auto-migrate
		err := db.AutoMigrate(&UserV1{})
		assert.NoError(t, err)

		// Second auto-migrate should be idempotent
		err = db.AutoMigrate(&UserV1{})
		assert.NoError(t, err)

		// Clean up
		deleteTableIfExists(t, db, &UserV1{})
	})

	t.Run("AutoMigrateWithBatchSize", func(t *testing.T) {
		// Create source table with data
		err := db.CreateTable(&UserV1{})
		require.NoError(t, err)

		// Add multiple items
		for i := 0; i < 50; i++ {
			user := &UserV1{
				ID:    fmt.Sprintf("user-%d", i),
				Email: fmt.Sprintf("user%d@example.com", i),
				Name:  fmt.Sprintf("User %d", i),
			}
			err = db.Model(user).Create()
			require.NoError(t, err)
		}

		// Migrate with custom batch size
		err = db.AutoMigrateWithOptions(&UserV1{},
			tabletheory.WithTargetModel(&UserV2{}),
			tabletheory.WithDataCopy(true),
			tabletheory.WithBatchSize(10), // Process 10 items at a time
		)
		require.NoError(t, err)

		// Clean up
		deleteTableIfExists(t, db, &UserV1{})
		deleteTableIfExists(t, db, &UserV2{})
	})
}

// Helper function to split strings
func split(s, sep string) []string {
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if i+len(sep) <= len(s) && s[i:i+len(sep)] == sep {
			result = append(result, s[start:i])
			start = i + len(sep)
			i += len(sep) - 1
		}
	}
	if start < len(s) {
		result = append(result, s[start:])
	}
	return result
}
