// Package integration contains integration tests for TableTheory
package integration

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ModernUser uses default camelCase naming convention
type ModernUser struct {
	CreatedAt time.Time `theorydb:"created_at"`
	UpdatedAt time.Time `theorydb:"updated_at"`
	UserID    string    `theorydb:"pk"`
	Email     string    `theorydb:"sk"`
	FirstName string
	LastName  string
}

func (m ModernUser) TableName() string {
	return "modern_users"
}

// LegacyUser uses snake_case naming convention
type LegacyUser struct {
	_         struct{} `theorydb:"naming:snake_case"`
	CreatedAt time.Time
	UpdatedAt time.Time
	UserId    string `theorydb:"pk"`
	Email     string `theorydb:"sk"`
	FirstName string
	LastName  string
}

func (l LegacyUser) TableName() string {
	return "legacy_users"
}

// DynamORMUser preserves legacy DynamORM semantics:
// PK/SK stay uppercase, other attributes use camelCase.
type DynamORMUser struct {
	_         struct{} `theorydb:"naming:dynamorm"`
	CreatedAt time.Time
	UpdatedAt time.Time
	UserID    string `theorydb:"pk"`
	Entity    string `theorydb:"sk"`
	FirstName string
	LastName  string
}

func (d DynamORMUser) TableName() string {
	return "dynamorm_users"
}

type DynamORMAddress struct {
	PostalCode  string
	CountryCode string
}

type DynamORMProfile struct {
	DisplayName    string
	MailingAddress DynamORMAddress
}

type DynamORMNestedUser struct {
	_       struct{} `theorydb:"naming:dynamorm"`
	UserID  string   `theorydb:"pk"`
	Entity  string   `theorydb:"sk"`
	Profile DynamORMProfile
}

func (d DynamORMNestedUser) TableName() string {
	return "dynamorm_nested_users"
}

// TestNamingConventions tests both camelCase and snake_case naming conventions
func TestNamingConventions(t *testing.T) {
	testCtx := InitTestDB(t)

	t.Run("CamelCase_Modern", func(t *testing.T) {
		testCtx.CreateTableIfNotExists(t, &ModernUser{})

		// Create a user with camelCase attributes
		user := &ModernUser{
			UserID:    "user-001",
			Email:     "modern@example.com",
			FirstName: "John",
			LastName:  "Doe",
		}

		err := testCtx.DB.Model(user).Create()
		require.NoError(t, err)
		assert.NotZero(t, user.CreatedAt)

		// Query back and verify
		var retrieved ModernUser
		err = testCtx.DB.Model(&ModernUser{}).
			Where("UserID", "=", "user-001").
			Where("Email", "=", "modern@example.com").
			First(&retrieved)
		require.NoError(t, err)
		assert.Equal(t, "John", retrieved.FirstName)
		assert.Equal(t, "Doe", retrieved.LastName)
		assert.Equal(t, "user-001", retrieved.UserID)
		assert.Equal(t, "modern@example.com", retrieved.Email)
	})

	t.Run("SnakeCase_Legacy", func(t *testing.T) {
		testCtx.CreateTableIfNotExists(t, &LegacyUser{})

		// Create a user with snake_case attributes
		user := &LegacyUser{
			UserId:    "user-002",
			Email:     "legacy@example.com",
			FirstName: "Jane",
			LastName:  "Smith",
		}

		err := testCtx.DB.Model(user).Create()
		require.NoError(t, err)
		assert.NotZero(t, user.CreatedAt)

		// Query back and verify
		var retrieved LegacyUser
		err = testCtx.DB.Model(&LegacyUser{}).
			Where("UserId", "=", "user-002").
			Where("Email", "=", "legacy@example.com").
			First(&retrieved)
		require.NoError(t, err)
		assert.Equal(t, "Jane", retrieved.FirstName)
		assert.Equal(t, "Smith", retrieved.LastName)
		assert.Equal(t, "user-002", retrieved.UserId)
		assert.Equal(t, "legacy@example.com", retrieved.Email)
	})

	t.Run("BothConventions_SameDB", func(t *testing.T) {
		// Verify both models work in the same database instance

		// Create modern user
		modern := &ModernUser{
			UserID:    "user-003",
			Email:     "mixed-modern@example.com",
			FirstName: "Alice",
			LastName:  "Wonder",
		}
		err := testCtx.DB.Model(modern).Create()
		require.NoError(t, err)

		// Create legacy user
		legacy := &LegacyUser{
			UserId:    "user-004",
			Email:     "mixed-legacy@example.com",
			FirstName: "Bob",
			LastName:  "Builder",
		}
		err = testCtx.DB.Model(legacy).Create()
		require.NoError(t, err)

		// Retrieve both and verify
		var modernRetrieved ModernUser
		err = testCtx.DB.Model(&ModernUser{}).
			Where("UserID", "=", "user-003").
			Where("Email", "=", "mixed-modern@example.com").
			First(&modernRetrieved)
		require.NoError(t, err)
		assert.Equal(t, "Alice", modernRetrieved.FirstName)

		var legacyRetrieved LegacyUser
		err = testCtx.DB.Model(&LegacyUser{}).
			Where("UserId", "=", "user-004").
			Where("Email", "=", "mixed-legacy@example.com").
			First(&legacyRetrieved)
		require.NoError(t, err)
		assert.Equal(t, "Bob", legacyRetrieved.FirstName)
	})

	t.Run("DynamORM_LegacyPKSK", func(t *testing.T) {
		testCtx.CreateTableIfNotExists(t, &DynamORMUser{})

		user := &DynamORMUser{
			UserID:    "user-legacy-001",
			Entity:    "PROFILE",
			FirstName: "Dana",
			LastName:  "Legacy",
		}

		err := testCtx.DB.Model(user).Create()
		require.NoError(t, err)
		assert.NotZero(t, user.CreatedAt)

		var retrieved DynamORMUser
		err = testCtx.DB.Model(&DynamORMUser{}).
			Where("UserID", "=", "user-legacy-001").
			Where("Entity", "=", "PROFILE").
			First(&retrieved)
		require.NoError(t, err)
		assert.Equal(t, "Dana", retrieved.FirstName)
		assert.Equal(t, "Legacy", retrieved.LastName)
		assert.Equal(t, "user-legacy-001", retrieved.UserID)
		assert.Equal(t, "PROFILE", retrieved.Entity)
	})
}

// TestSnakeCaseUpdate tests update operations with snake_case naming
func TestSnakeCaseUpdate(t *testing.T) {
	testCtx := InitTestDB(t)
	testCtx.CreateTableIfNotExists(t, &LegacyUser{})

	// Create initial user
	user := &LegacyUser{
		UserId:    "user-005",
		Email:     "update-test@example.com",
		FirstName: "Original",
		LastName:  "Name",
	}
	err := testCtx.DB.Model(user).Create()
	require.NoError(t, err)

	// Update the user
	user.FirstName = "Updated"
	user.LastName = "Person"
	err = testCtx.DB.Model(user).
		Where("UserId", "=", "user-005").
		Where("Email", "=", "update-test@example.com").
		Update("FirstName", "LastName")
	require.NoError(t, err)

	// Verify update by querying back
	var retrieved LegacyUser
	err = testCtx.DB.Model(&LegacyUser{}).
		Where("UserId", "=", "user-005").
		Where("Email", "=", "update-test@example.com").
		First(&retrieved)
	require.NoError(t, err)
	assert.Equal(t, "Updated", retrieved.FirstName)
	assert.Equal(t, "Person", retrieved.LastName)
}

func TestDynamORMNestedStructNaming(t *testing.T) {
	testCtx := InitTestDB(t)
	testCtx.CreateTableIfNotExists(t, &DynamORMNestedUser{})

	user := &DynamORMNestedUser{
		UserID: "user-legacy-nested-001",
		Entity: "PROFILE",
		Profile: DynamORMProfile{
			DisplayName: "Dana Legacy",
			MailingAddress: DynamORMAddress{
				PostalCode:  "10001",
				CountryCode: "US",
			},
		},
	}

	err := testCtx.DB.Model(user).Create()
	require.NoError(t, err)

	item := mustGetDynamORMNestedUserItem(t, testCtx, user.UserID, user.Entity)
	assertNestedProfileCamelCase(t, item)

	user.Profile = DynamORMProfile{
		DisplayName: "Dana Updated",
		MailingAddress: DynamORMAddress{
			PostalCode:  "94107",
			CountryCode: "US",
		},
	}

	err = testCtx.DB.Model(user).
		Where("UserID", "=", user.UserID).
		Where("Entity", "=", user.Entity).
		Update("Profile")
	require.NoError(t, err)

	item = mustGetDynamORMNestedUserItem(t, testCtx, user.UserID, user.Entity)
	assertNestedProfileCamelCase(t, item)

	var retrieved DynamORMNestedUser
	err = testCtx.DB.Model(&DynamORMNestedUser{}).
		Where("UserID", "=", user.UserID).
		Where("Entity", "=", user.Entity).
		First(&retrieved)
	require.NoError(t, err)
	assert.Equal(t, "Dana Updated", retrieved.Profile.DisplayName)
	assert.Equal(t, "94107", retrieved.Profile.MailingAddress.PostalCode)
	assert.Equal(t, "US", retrieved.Profile.MailingAddress.CountryCode)
}

func mustGetDynamORMNestedUserItem(t *testing.T, testCtx *TestContext, userID, entity string) map[string]ddbtypes.AttributeValue {
	t.Helper()

	tableName := DynamORMNestedUser{}.TableName()
	resp, err := testCtx.DynamoDBClient.GetItem(context.Background(), &dynamodb.GetItemInput{
		TableName: &tableName,
		Key: map[string]ddbtypes.AttributeValue{
			"PK": &ddbtypes.AttributeValueMemberS{Value: userID},
			"SK": &ddbtypes.AttributeValueMemberS{Value: entity},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp.Item)
	return resp.Item
}

func assertNestedProfileCamelCase(t *testing.T, item map[string]ddbtypes.AttributeValue) {
	t.Helper()

	profileAV, ok := item["profile"].(*ddbtypes.AttributeValueMemberM)
	require.True(t, ok)
	require.Contains(t, profileAV.Value, "displayName")
	require.Contains(t, profileAV.Value, "mailingAddress")
	require.NotContains(t, profileAV.Value, "DisplayName")
	require.NotContains(t, profileAV.Value, "MailingAddress")

	addressAV, ok := profileAV.Value["mailingAddress"].(*ddbtypes.AttributeValueMemberM)
	require.True(t, ok)
	require.Contains(t, addressAV.Value, "postalCode")
	require.Contains(t, addressAV.Value, "countryCode")
	require.NotContains(t, addressAV.Value, "PostalCode")
	require.NotContains(t, addressAV.Value, "CountryCode")
}
