package schema

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/model"
	"github.com/theory-cloud/tabletheory/pkg/session"
	"github.com/theory-cloud/tabletheory/tests"
)

func deleteTableIfExists(t *testing.T, manager *Manager, tableName string) {
	t.Helper()
	if err := manager.DeleteTable(tableName); err != nil {
		var notFoundErr *types.ResourceNotFoundException
		if errors.As(err, &notFoundErr) {
			return
		}
		require.NoError(t, err)
	}
}

// Test models
type User struct {
	CreatedAt time.Time `theorydb:"created_at"`
	UpdatedAt time.Time `theorydb:"updated_at"`
	ID        string    `theorydb:"pk"`
	Email     string    `theorydb:"sk"`
	Name      string
	Age       int
	Balance   float64
	Version   int `theorydb:"version"`
}

type Product struct {
	UpdatedTime time.Time `theorydb:"lsi:updated-lsi,sk"`
	ID          string    `theorydb:"pk"`
	CategoryID  string    `theorydb:"sk"`
	Name        string    `theorydb:"index:name-index,pk"`
	Price       float64
	StockLevel  int
}

type Order struct {
	OrderDate  time.Time `theorydb:"index:customer-index,sk"`
	UpdatedAt  time.Time `theorydb:"index:status-index,sk"`
	OrderID    string    `theorydb:"pk"`
	CustomerID string    `theorydb:"index:customer-index,pk"`
	Status     string    `theorydb:"index:status-index,pk"`
	Total      float64
}

func TestCreateTable(t *testing.T) {
	// Skip if no test endpoint is set
	if testing.Short() {
		t.Skip("Skipping integration test")
	}
	tests.RequireDynamoDBLocal(t)

	// Create test session with dummy credentials for DynamoDB Local
	sess, err := session.NewSession(&session.Config{
		Region:   "us-east-1",
		Endpoint: "http://localhost:8000",
		CredentialsProvider: aws.CredentialsProviderFunc(
			func(ctx context.Context) (aws.Credentials, error) {
				return aws.Credentials{
					AccessKeyID:     "dummy",
					SecretAccessKey: "dummy",
				}, nil
			}),
	})
	require.NoError(t, err)

	// Create registry and register model
	registry := model.NewRegistry()
	err = registry.Register(&User{})
	require.NoError(t, err)

	// Create schema manager
	manager := NewManager(sess, registry)

	t.Run("CreateSimpleTable", func(t *testing.T) {
		// Delete table if exists
		deleteTableIfExists(t, manager, "Users")

		// Create table
		err := manager.CreateTable(&User{})
		assert.NoError(t, err)

		// Verify table exists
		exists, err := manager.TableExists("Users")
		assert.NoError(t, err)
		assert.True(t, exists)

		// Describe table
		desc, err := manager.DescribeTable(&User{})
		assert.NoError(t, err)
		assert.Equal(t, "Users", *desc.TableName)
		assert.Equal(t, types.TableStatusActive, desc.TableStatus)

		// Cleanup
		deleteTableIfExists(t, manager, "Users")
	})

	t.Run("CreateTableWithGSI", func(t *testing.T) {
		// Register model with GSI
		err := registry.Register(&Order{})
		require.NoError(t, err)

		// Delete table if exists
		deleteTableIfExists(t, manager, "Orders")

		// Create table
		err = manager.CreateTable(&Order{})
		assert.NoError(t, err)

		// Verify GSIs
		desc, err := manager.DescribeTable(&Order{})
		assert.NoError(t, err)
		assert.Len(t, desc.GlobalSecondaryIndexes, 2)

		// Check customer index
		var hasCustomerIndex, hasStatusIndex bool
		for _, gsi := range desc.GlobalSecondaryIndexes {
			if *gsi.IndexName == "customer-index" {
				hasCustomerIndex = true
				assert.Equal(t, "customerID", *gsi.KeySchema[0].AttributeName)
				assert.Equal(t, types.KeyTypeHash, gsi.KeySchema[0].KeyType)
				assert.Equal(t, "orderDate", *gsi.KeySchema[1].AttributeName)
				assert.Equal(t, types.KeyTypeRange, gsi.KeySchema[1].KeyType)
			}
			if *gsi.IndexName == "status-index" {
				hasStatusIndex = true
			}
		}
		assert.True(t, hasCustomerIndex)
		assert.True(t, hasStatusIndex)

		// Cleanup
		deleteTableIfExists(t, manager, "Orders")
	})

	t.Run("CreateTableWithLSI", func(t *testing.T) {
		// Register model with LSI
		err := registry.Register(&Product{})
		require.NoError(t, err)

		// Delete table if exists
		deleteTableIfExists(t, manager, "Products")

		// Create table
		err = manager.CreateTable(&Product{})
		assert.NoError(t, err)

		// Verify LSI
		desc, err := manager.DescribeTable(&Product{})
		assert.NoError(t, err)
		assert.Len(t, desc.LocalSecondaryIndexes, 1)
		assert.Equal(t, "updated-lsi", *desc.LocalSecondaryIndexes[0].IndexName)

		// Cleanup
		deleteTableIfExists(t, manager, "Products")
	})

	t.Run("CreateTableWithOptions", func(t *testing.T) {
		// Delete table if exists
		deleteTableIfExists(t, manager, "Users")

		// Create table with provisioned throughput
		err := manager.CreateTable(&User{},
			WithBillingMode(types.BillingModeProvisioned),
			WithThroughput(5, 5),
		)
		assert.NoError(t, err)

		// Verify billing mode
		desc, err := manager.DescribeTable(&User{})
		assert.NoError(t, err)
		if desc.BillingModeSummary != nil {
			assert.Equal(t, types.BillingModeProvisioned, desc.BillingModeSummary.BillingMode)
		}
		if desc.ProvisionedThroughput != nil {
			assert.Equal(t, int64(5), *desc.ProvisionedThroughput.ReadCapacityUnits)
			assert.Equal(t, int64(5), *desc.ProvisionedThroughput.WriteCapacityUnits)
		}

		// Cleanup
		deleteTableIfExists(t, manager, "Users")
	})
}

func TestTableExists(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}
	tests.RequireDynamoDBLocal(t)

	// Create test session with dummy credentials for DynamoDB Local
	sess, err := session.NewSession(&session.Config{
		Region:   "us-east-1",
		Endpoint: "http://localhost:8000",
		CredentialsProvider: aws.CredentialsProviderFunc(
			func(ctx context.Context) (aws.Credentials, error) {
				return aws.Credentials{
					AccessKeyID:     "dummy",
					SecretAccessKey: "dummy",
				}, nil
			}),
	})
	require.NoError(t, err)

	// Create registry and manager
	registry := model.NewRegistry()
	manager := NewManager(sess, registry)

	t.Run("NonExistentTable", func(t *testing.T) {
		exists, err := manager.TableExists("NonExistentTable")
		assert.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("ExistingTable", func(t *testing.T) {
		// Create a table first
		err := registry.Register(&User{})
		require.NoError(t, err)

		deleteTableIfExists(t, manager, "Users")
		err = manager.CreateTable(&User{})
		require.NoError(t, err)

		// Check existence
		exists, err := manager.TableExists("Users")
		assert.NoError(t, err)
		assert.True(t, exists)

		// Cleanup
		deleteTableIfExists(t, manager, "Users")
	})
}

func TestUpdateTable(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}
	tests.RequireDynamoDBLocal(t)

	// Create test session with dummy credentials for DynamoDB Local
	sess, err := session.NewSession(&session.Config{
		Region:   "us-east-1",
		Endpoint: "http://localhost:8000",
		CredentialsProvider: aws.CredentialsProviderFunc(
			func(ctx context.Context) (aws.Credentials, error) {
				return aws.Credentials{
					AccessKeyID:     "dummy",
					SecretAccessKey: "dummy",
				}, nil
			}),
	})
	require.NoError(t, err)

	// Create registry and manager
	registry := model.NewRegistry()
	err = registry.Register(&User{})
	require.NoError(t, err)
	manager := NewManager(sess, registry)

	t.Run("UpdateBillingMode", func(t *testing.T) {
		// Create table with on-demand billing
		deleteTableIfExists(t, manager, "Users")
		err := manager.CreateTable(&User{})
		require.NoError(t, err)

		// Update to provisioned billing
		err = manager.UpdateTable(&User{},
			WithBillingMode(types.BillingModeProvisioned),
			WithThroughput(10, 10),
		)
		assert.NoError(t, err)

		// Verify update
		desc, err := manager.DescribeTable(&User{})
		assert.NoError(t, err)
		assert.Equal(t, types.BillingModeProvisioned, desc.BillingModeSummary.BillingMode)

		// Cleanup
		deleteTableIfExists(t, manager, "Users")
	})
}

func TestBuildAttributeDefinitions(t *testing.T) {
	registry := model.NewRegistry()
	manager := &Manager{registry: registry}

	t.Run("SimpleTable", func(t *testing.T) {
		err := registry.Register(&User{})
		require.NoError(t, err)

		metadata, err := registry.GetMetadata(&User{})
		require.NoError(t, err)

		attrs := manager.buildAttributeDefinitions(metadata)
		assert.Len(t, attrs, 2) // ID and Email (PK and SK)

		// Check that we have the right attributes
		attrMap := make(map[string]types.ScalarAttributeType)
		for _, attr := range attrs {
			attrMap[*attr.AttributeName] = attr.AttributeType
		}

		assert.Equal(t, types.ScalarAttributeTypeS, attrMap["id"])
		assert.Equal(t, types.ScalarAttributeTypeS, attrMap["email"])
	})

	t.Run("TableWithIndexes", func(t *testing.T) {
		err := registry.Register(&Order{})
		require.NoError(t, err)

		metadata, err := registry.GetMetadata(&Order{})
		require.NoError(t, err)

		attrs := manager.buildAttributeDefinitions(metadata)

		// Should have OrderID, CustomerID, OrderDate, Status, UpdatedAt
		attrMap := make(map[string]types.ScalarAttributeType)
		for _, attr := range attrs {
			attrMap[*attr.AttributeName] = attr.AttributeType
		}

		assert.Contains(t, attrMap, "orderID")
		assert.Contains(t, attrMap, "customerID")
		assert.Contains(t, attrMap, "orderDate")
		assert.Contains(t, attrMap, "status")
		assert.Contains(t, attrMap, "updatedAt")
	})
}

func TestGetAttributeType(t *testing.T) {
	manager := &Manager{}

	tests := []struct {
		name     string
		expected types.ScalarAttributeType
		kind     reflect.Kind
	}{
		{"String", types.ScalarAttributeTypeS, reflect.String},
		{"Int", types.ScalarAttributeTypeN, reflect.Int},
		{"Int64", types.ScalarAttributeTypeN, reflect.Int64},
		{"Uint", types.ScalarAttributeTypeN, reflect.Uint},
		{"Slice", types.ScalarAttributeTypeB, reflect.Slice},
		{"Other", types.ScalarAttributeTypeS, reflect.Bool},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.getAttributeType(tt.kind)
			assert.Equal(t, tt.expected, result)
		})
	}
}
