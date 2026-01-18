package integration

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory"
	"github.com/theory-cloud/tabletheory/pkg/schema"
	"github.com/theory-cloud/tabletheory/tests"
)

// Migration test models
type UserV1 struct {
	ID       string `theorydb:"pk"`
	Email    string `theorydb:"sk"`
	Name     string `theorydb:"attr:fullName"`
	Status   string `theorydb:"attr:status"`
	Settings string `theorydb:"attr:settings"`
	Age      int    `theorydb:"attr:age"`
	Version  int64  `theorydb:"version"`
}

func (u *UserV1) TableName() string {
	return "users_v1"
}

type UserV2 struct {
	CreatedAt time.Time         `theorydb:"attr:createdAt"`
	Settings  map[string]string `theorydb:"attr:settings"`
	ID        string            `theorydb:"pk"`
	Email     string            `theorydb:"sk"`
	FirstName string            `theorydb:"attr:firstName"`
	LastName  string            `theorydb:"attr:lastName"`
	Age       int               `theorydb:"attr:age"`
	Version   int64             `theorydb:"version"`
	Active    bool              `theorydb:"attr:active"`
}

func (u *UserV2) TableName() string {
	return "users_v2"
}

type ProductV1 struct {
	ID          string  `theorydb:"pk"`
	Category    string  `theorydb:"sk"`
	Name        string  `theorydb:"attr:productName"`
	Description string  `theorydb:"attr:description"`
	Price       float64 `theorydb:"attr:price"`
	Version     int64   `theorydb:"version"`
}

func (p *ProductV1) TableName() string {
	return "products_v1"
}

type ProductV2 struct {
	UpdatedAt   time.Time         `theorydb:"attr:updatedAt"`
	Metadata    map[string]string `theorydb:"attr:metadata"`
	ID          string            `theorydb:"pk"`
	Category    string            `theorydb:"sk"`
	Name        string            `theorydb:"attr:productName"`
	Currency    string            `theorydb:"attr:currency"`
	Description string            `theorydb:"attr:description"`
	Tags        []string          `theorydb:"attr:tags"`
	Price       float64           `theorydb:"attr:price"`
	Version     int64             `theorydb:"version"`
}

func (p *ProductV2) TableName() string {
	return "products_v2"
}

func TestAutoMigrate(t *testing.T) {
	tests.RequireDynamoDBLocal(t)

	// Create DB with TestContext
	testCtx := InitTestDB(t)

	t.Run("BasicDataTransformation", func(t *testing.T) {
		// Create tables
		testCtx.CreateTableIfNotExists(t, &UserV1{})
		testCtx.CreateTableIfNotExists(t, &UserV2{})

		// Add test data
		users := []*UserV1{
			{
				ID:       "user-1",
				Email:    "john@example.com",
				Name:     "John Doe",
				Age:      30,
				Status:   "active",
				Settings: "theme=dark,lang=en",
				Version:  1,
			},
			{
				ID:       "user-2",
				Email:    "jane@example.com",
				Name:     "Jane Smith",
				Age:      25,
				Status:   "inactive",
				Settings: "theme=light,lang=es",
				Version:  1,
			},
		}

		for _, u := range users {
			err := testCtx.DB.Model(u).Create()
			require.NoError(t, err)
		}

		// Define transformation function
		transformFunc := func(old UserV1) UserV2 {
			// Split name into first and last
			var firstName, lastName string
			if old.Name != "" {
				parts := strings.Split(old.Name, " ")
				if len(parts) > 0 {
					firstName = parts[0]
				}
				if len(parts) > 1 {
					lastName = strings.Join(parts[1:], " ")
				}
			}

			// Parse settings string into map
			settings := make(map[string]string)
			if old.Settings != "" {
				pairs := strings.Split(old.Settings, ",")
				for _, pair := range pairs {
					if kv := strings.Split(pair, "="); len(kv) == 2 {
						settings[kv[0]] = kv[1]
					}
				}
			}

			return UserV2{
				ID:        old.ID,
				Email:     old.Email,
				FirstName: firstName,
				LastName:  lastName,
				Age:       old.Age,
				Active:    old.Status == "active",
				Settings:  settings,
				CreatedAt: time.Now(),
				Version:   1,
			}
		}

		// Migrate to V2 with transformation
		err := testCtx.DB.AutoMigrateWithOptions(&UserV1{},
			tabletheory.WithTargetModel(&UserV2{}),
			tabletheory.WithDataCopy(true),
			tabletheory.WithTransform(transformFunc),
		)
		require.NoError(t, err)

		// Verify migration results
		var migratedUsers []UserV2
		err = testCtx.DB.Model(&UserV2{}).All(&migratedUsers)
		require.NoError(t, err)
		assert.Len(t, migratedUsers, 2)

		// Check first user
		var user1 UserV2
		err = testCtx.DB.Model(&UserV2{}).
			Where("ID", "=", "user-1").
			Where("Email", "=", "john@example.com").
			First(&user1)
		require.NoError(t, err)

		assert.Equal(t, "user-1", user1.ID)
		assert.Equal(t, "john@example.com", user1.Email)
		assert.Equal(t, "John", user1.FirstName)
		assert.Equal(t, "Doe", user1.LastName)
		assert.Equal(t, 30, user1.Age)
		assert.True(t, user1.Active)
		assert.Equal(t, "dark", user1.Settings["theme"])
		assert.Equal(t, "en", user1.Settings["lang"])

		// Check second user
		var user2 UserV2
		err = testCtx.DB.Model(&UserV2{}).
			Where("ID", "=", "user-2").
			Where("Email", "=", "jane@example.com").
			First(&user2)
		require.NoError(t, err)

		assert.Equal(t, "user-2", user2.ID)
		assert.Equal(t, "jane@example.com", user2.Email)
		assert.Equal(t, "Jane", user2.FirstName)
		assert.Equal(t, "Smith", user2.LastName)
		assert.Equal(t, 25, user2.Age)
		assert.False(t, user2.Active)
		assert.Equal(t, "light", user2.Settings["theme"])
		assert.Equal(t, "es", user2.Settings["lang"])
	})

	t.Run("AttributeValueTransformation", func(t *testing.T) {
		// Create tables
		testCtx.CreateTableIfNotExists(t, &ProductV1{})
		testCtx.CreateTableIfNotExists(t, &ProductV2{})

		// Add test data
		products := []*ProductV1{
			{
				ID:          "prod-1",
				Category:    "electronics",
				Name:        "Laptop",
				Price:       999.99,
				Description: "High-performance laptop",
				Version:     1,
			},
			{
				ID:          "prod-2",
				Category:    "books",
				Name:        "Go Programming",
				Price:       39.99,
				Description: "Learn Go programming",
				Version:     1,
			},
		}

		for _, p := range products {
			err := testCtx.DB.Model(p).Create()
			require.NoError(t, err)
		}

		// Define AttributeValue transformation function
		var transformFunc schema.TransformFunc = func(source map[string]types.AttributeValue) (map[string]types.AttributeValue, error) {
			target := make(map[string]types.AttributeValue)

			// Copy all existing fields
			for k, v := range source {
				target[k] = v
			}

			// Add currency field based on price
			target["currency"] = &types.AttributeValueMemberS{Value: "USD"}

			// Add tags based on category
			var tags []types.AttributeValue
			if categoryAttr, exists := source["Category"]; exists {
				if categoryStr, ok := categoryAttr.(*types.AttributeValueMemberS); ok {
					switch categoryStr.Value {
					case "electronics":
						tags = []types.AttributeValue{
							&types.AttributeValueMemberS{Value: "tech"},
							&types.AttributeValueMemberS{Value: "gadget"},
						}
					case "books":
						tags = []types.AttributeValue{
							&types.AttributeValueMemberS{Value: "education"},
							&types.AttributeValueMemberS{Value: "reading"},
						}
					}
				}
			}
			if len(tags) > 0 {
				target["tags"] = &types.AttributeValueMemberL{Value: tags}
			}

			// Add metadata
			metadata := map[string]types.AttributeValue{
				"migrated_at": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
				"source":      &types.AttributeValueMemberS{Value: "v1"},
			}
			target["metadata"] = &types.AttributeValueMemberM{Value: metadata}

			// Add updated_at timestamp
			target["updated_at"] = &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)}

			return target, nil
		}

		// Migrate to V2 with transformation
		err := testCtx.DB.AutoMigrateWithOptions(&ProductV1{},
			tabletheory.WithTargetModel(&ProductV2{}),
			tabletheory.WithDataCopy(true),
			tabletheory.WithTransform(transformFunc),
		)
		require.NoError(t, err)

		// Verify migration results
		var migratedProducts []ProductV2
		err = testCtx.DB.Model(&ProductV2{}).All(&migratedProducts)
		require.NoError(t, err)
		assert.Len(t, migratedProducts, 2)

		// Check electronics product
		var laptop ProductV2
		err = testCtx.DB.Model(&ProductV2{}).
			Where("ID", "=", "prod-1").
			Where("Category", "=", "electronics").
			First(&laptop)
		require.NoError(t, err)

		assert.Equal(t, "prod-1", laptop.ID)
		assert.Equal(t, "electronics", laptop.Category)
		assert.Equal(t, "Laptop", laptop.Name)
		assert.Equal(t, 999.99, laptop.Price)
		assert.Equal(t, "USD", laptop.Currency)
		assert.Contains(t, laptop.Tags, "tech")
		assert.Contains(t, laptop.Tags, "gadget")
		assert.Equal(t, "v1", laptop.Metadata["source"])
		assert.NotEmpty(t, laptop.Metadata["migrated_at"])

		// Check books product
		var book ProductV2
		err = testCtx.DB.Model(&ProductV2{}).
			Where("ID", "=", "prod-2").
			Where("Category", "=", "books").
			First(&book)
		require.NoError(t, err)

		assert.Equal(t, "prod-2", book.ID)
		assert.Equal(t, "books", book.Category)
		assert.Equal(t, "Go Programming", book.Name)
		assert.Equal(t, 39.99, book.Price)
		assert.Equal(t, "USD", book.Currency)
		assert.Contains(t, book.Tags, "education")
		assert.Contains(t, book.Tags, "reading")
	})
}

func TestMigrationWithBackup(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	tests.RequireDynamoDBLocal(t)

	testCtx := InitTestDB(t)

	t.Run("MigrationWithBackup", func(t *testing.T) {
		// Create tables
		testCtx.CreateTableIfNotExists(t, &UserV1{})
		testCtx.CreateTableIfNotExists(t, &UserV2{})

		// Add test data
		user := &UserV1{
			ID:      "user-1",
			Email:   "test@example.com",
			Name:    "Test User",
			Age:     25,
			Status:  "active",
			Version: 1,
		}
		err := testCtx.DB.Model(user).Create()
		require.NoError(t, err)

		// Migrate with backup
		err = testCtx.DB.AutoMigrateWithOptions(&UserV1{},
			tabletheory.WithTargetModel(&UserV2{}),
			tabletheory.WithDataCopy(true),
			tabletheory.WithBackupTable("users_v1_backup"),
			tabletheory.WithTransform(func(old UserV1) UserV2 {
				return UserV2{
					ID:        old.ID,
					Email:     old.Email,
					FirstName: old.Name,
					Age:       old.Age,
					Active:    old.Status == "active",
					CreatedAt: time.Now(),
					Version:   1,
				}
			}),
		)
		require.NoError(t, err)

		// Verify target table has data
		var migratedUser UserV2
		err = testCtx.DB.Model(&UserV2{}).
			Where("ID", "=", "user-1").
			Where("Email", "=", "test@example.com").
			First(&migratedUser)
		require.NoError(t, err)
		assert.Equal(t, "Test User", migratedUser.FirstName)

		// Note: Backup verification would depend on the backup implementation
		// In a real scenario, you might check for backup table existence or backup metadata
	})
}

func TestMigrationBatchProcessing(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	tests.RequireDynamoDBLocal(t)

	// Create DB with TestContext
	testCtx := InitTestDB(t)

	t.Run("LargeBatchMigration", func(t *testing.T) {
		// Create tables
		testCtx.CreateTableIfNotExists(t, &UserV1{})
		testCtx.CreateTableIfNotExists(t, &UserV2{})

		// Create multiple users to test batch processing
		const numUsers = 50
		for i := 0; i < numUsers; i++ {
			status := "active"
			if i%2 == 1 {
				status = "inactive"
			}
			user := &UserV1{
				ID:      fmt.Sprintf("user-%d", i),
				Email:   fmt.Sprintf("user%d@example.com", i),
				Name:    fmt.Sprintf("User %d", i),
				Age:     20 + (i % 50),
				Status:  status,
				Version: 1,
			}
			err := testCtx.DB.Model(user).Create()
			require.NoError(t, err)
		}

		// Migrate with small batch size to test batching
		err := testCtx.DB.AutoMigrateWithOptions(&UserV1{},
			tabletheory.WithTargetModel(&UserV2{}),
			tabletheory.WithDataCopy(true),
			tabletheory.WithBatchSize(10), // Small batch size to test batching
			tabletheory.WithTransform(func(old UserV1) UserV2 {
				return UserV2{
					ID:        old.ID,
					Email:     old.Email,
					FirstName: old.Name,
					Age:       old.Age,
					Active:    old.Status == "active",
					CreatedAt: time.Now(),
					Version:   1,
				}
			}),
		)
		require.NoError(t, err)

		// Verify all users were migrated
		var migratedUsers []UserV2
		err = testCtx.DB.Model(&UserV2{}).All(&migratedUsers)
		require.NoError(t, err)
		assert.Len(t, migratedUsers, numUsers)

		// Verify a few specific users
		var user0 UserV2
		err = testCtx.DB.Model(&UserV2{}).
			Where("ID", "=", "user-0").
			Where("Email", "=", "user0@example.com").
			First(&user0)
		require.NoError(t, err)
		assert.Equal(t, "User 0", user0.FirstName)
		assert.True(t, user0.Active)

		var user1 UserV2
		err = testCtx.DB.Model(&UserV2{}).
			Where("ID", "=", "user-1").
			Where("Email", "=", "user1@example.com").
			First(&user1)
		require.NoError(t, err)
		assert.Equal(t, "User 1", user1.FirstName)
		assert.False(t, user1.Active)
	})
}

func TestMigrationErrorHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	tests.RequireDynamoDBLocal(t)

	// Create DB with TestContext
	testCtx := InitTestDB(t)

	t.Run("InvalidTransformFunction", func(t *testing.T) {
		// Create source table
		testCtx.CreateTableIfNotExists(t, &UserV1{})

		// Try to use an invalid transform function
		invalidTransform := "not a function"

		err := testCtx.DB.AutoMigrateWithOptions(&UserV1{},
			tabletheory.WithTargetModel(&UserV2{}),
			tabletheory.WithDataCopy(true),
			tabletheory.WithTransform(invalidTransform),
		)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "transform must be a function")
	})

	t.Run("TransformationError", func(t *testing.T) {
		// Create and populate source table
		testCtx.CreateTableIfNotExists(t, &UserV1{})

		user := &UserV1{
			ID:      "user-1",
			Email:   "test@example.com",
			Name:    "Test User",
			Version: 1,
		}
		err := testCtx.DB.Model(user).Create()
		require.NoError(t, err)

		// Define a transform that will fail
		var transformFunc schema.TransformFunc = func(source map[string]types.AttributeValue) (map[string]types.AttributeValue, error) {
			return nil, fmt.Errorf("intentional transform error")
		}

		// Migration should fail due to transform error
		err = testCtx.DB.AutoMigrateWithOptions(&UserV1{},
			tabletheory.WithTargetModel(&UserV2{}),
			tabletheory.WithDataCopy(true),
			tabletheory.WithTransform(transformFunc),
		)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "intentional transform error")
	})
}

func TestMigrationDataIntegrity(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	tests.RequireDynamoDBLocal(t)

	// Create DB with TestContext
	testCtx := InitTestDB(t)

	t.Run("DataIntegrityVerification", func(t *testing.T) {
		// Create and populate source table
		testCtx.CreateTableIfNotExists(t, &UserV1{})

		// Add test data with various data types
		users := []*UserV1{
			{
				ID:       "user-1",
				Email:    "john@example.com",
				Name:     "John Doe",
				Age:      30,
				Status:   "active",
				Settings: "theme=dark",
				Version:  1,
			},
			{
				ID:       "user-2",
				Email:    "jane@example.com",
				Name:     "Jane Smith",
				Age:      0, // Test zero value
				Status:   "",
				Settings: "",
				Version:  1,
			},
		}

		for _, u := range users {
			err := testCtx.DB.Model(u).Create()
			require.NoError(t, err)
		}

		// Transform with careful data handling
		transformFunc := func(old UserV1) UserV2 {
			var firstName, lastName string
			if old.Name != "" {
				parts := strings.Split(old.Name, " ")
				if len(parts) > 0 {
					firstName = parts[0]
				}
				if len(parts) > 1 {
					lastName = strings.Join(parts[1:], " ")
				}
			}

			settings := make(map[string]string)
			if old.Settings != "" {
				pairs := strings.Split(old.Settings, ",")
				for _, pair := range pairs {
					if kv := strings.Split(pair, "="); len(kv) == 2 {
						settings[kv[0]] = kv[1]
					}
				}
			}

			return UserV2{
				ID:        old.ID,
				Email:     old.Email,
				FirstName: firstName,
				LastName:  lastName,
				Age:       old.Age,
				Active:    old.Status == "active",
				Settings:  settings,
				CreatedAt: time.Now(),
				Version:   1,
			}
		}

		// Perform migration
		err := testCtx.DB.AutoMigrateWithOptions(&UserV1{},
			tabletheory.WithTargetModel(&UserV2{}),
			tabletheory.WithDataCopy(true),
			tabletheory.WithTransform(transformFunc),
		)
		require.NoError(t, err)

		// Verify data integrity
		var migratedUsers []UserV2
		err = testCtx.DB.Model(&UserV2{}).All(&migratedUsers)
		require.NoError(t, err)
		assert.Len(t, migratedUsers, 2)

		// Check user with full data
		var user1 UserV2
		err = testCtx.DB.Model(&UserV2{}).
			Where("ID", "=", "user-1").
			Where("Email", "=", "john@example.com").
			First(&user1)
		require.NoError(t, err)

		assert.Equal(t, "user-1", user1.ID)
		assert.Equal(t, "john@example.com", user1.Email)
		assert.Equal(t, "John", user1.FirstName)
		assert.Equal(t, "Doe", user1.LastName)
		assert.Equal(t, 30, user1.Age)
		assert.True(t, user1.Active)
		assert.Equal(t, "dark", user1.Settings["theme"])

		// Check user with minimal data (test zero values)
		var user2 UserV2
		err = testCtx.DB.Model(&UserV2{}).
			Where("ID", "=", "user-2").
			Where("Email", "=", "jane@example.com").
			First(&user2)
		require.NoError(t, err)

		assert.Equal(t, "user-2", user2.ID)
		assert.Equal(t, "jane@example.com", user2.Email)
		assert.Equal(t, "Jane", user2.FirstName)
		assert.Equal(t, "Smith", user2.LastName)
		assert.Equal(t, 0, user2.Age)   // Zero value preserved
		assert.False(t, user2.Active)   // Empty status -> inactive
		assert.Empty(t, user2.Settings) // Empty settings map
	})
}
