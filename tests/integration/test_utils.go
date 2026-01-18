package integration

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/session"
)

// TestContext holds test database and cleanup functions
type TestContext struct {
	DB             core.ExtendedDB
	DynamoDBClient *dynamodb.Client
	TablesCreated  []string
	cleanup        []func() error
}

// InitTestDB creates a test database instance with proper cleanup setup
func InitTestDB(t *testing.T) *TestContext {
	t.Helper()

	if testing.Short() {
		t.Skip("Skipping integration test in -short mode")
	}

	// Always check for DynamoDB Local availability first
	// This will skip the test with a clear message if DynamoDB Local is not running
	endpoint := os.Getenv("DYNAMODB_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:8000"
	}

	// Check if DynamoDB Local is running
	if !isDynamoDBLocalRunning(endpoint) {
		t.Skip(`DynamoDB Local is not running.

To run integration tests:
1. Install Docker: https://www.docker.com/
2. Start DynamoDB Local: ./tests/setup_test_env.sh
3. Run tests: go test ./tests/integration -v

Or skip integration tests: SKIP_INTEGRATION=true go test ./...`)
	}

	if os.Getenv("SKIP_INTEGRATION") == "true" {
		t.Skip("Integration tests disabled")
	}

	sessionConfig := session.Config{
		Region:   "us-east-1",
		Endpoint: endpoint,
		AWSConfigOptions: []func(*config.LoadOptions) error{
			config.WithCredentialsProvider(
				credentials.NewStaticCredentialsProvider("dummy", "dummy", ""),
			),
			config.WithRegion("us-east-1"),
		},
	}

	db, err := tabletheory.New(sessionConfig)
	require.NoError(t, err)
	require.NotNil(t, db)

	// Create DynamoDB client for direct operations
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("dummy", "dummy", ""),
		),
	)
	require.NoError(t, err)

	client := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = &endpoint
	})

	testCtx := &TestContext{
		DB:             db,
		DynamoDBClient: client,
		TablesCreated:  make([]string, 0),
		cleanup:        make([]func() error, 0),
	}

	// Register cleanup on test completion
	t.Cleanup(func() {
		if err := testCtx.Cleanup(); err != nil {
			t.Logf("Cleanup error: %v", err)
		}
	})

	return testCtx
}

// CreateTable creates a table and registers it for cleanup
func (tc *TestContext) CreateTable(t *testing.T, model any) {
	t.Helper()

	err := tc.DB.CreateTable(model)
	if err != nil && !isTableExistsError(err) {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Get table name for cleanup tracking
	tableName := getTableName(model)
	tc.TablesCreated = append(tc.TablesCreated, tableName)

	// Wait for table to be ready with timeout
	tc.WaitForTable(t, tableName)
}

// CreateTableIfNotExists creates a table only if it doesn't exist
func (tc *TestContext) CreateTableIfNotExists(t *testing.T, model any) {
	t.Helper()

	tableName := getTableName(model)

	// Set timeout for the entire operation
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Check if table exists
	_, err := tc.DynamoDBClient.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: &tableName,
	})

	if err != nil {
		// Table doesn't exist, create it
		t.Logf("Creating table %s", tableName)
		tc.CreateTable(t, model)
	} else {
		// CRITICAL FIX: Delete and recreate table to ensure schema changes take effect
		// This is essential for integration tests where model schemas may change
		t.Logf("Table %s already exists, deleting and recreating to ensure fresh schema", tableName)
		tc.DeleteTable(t, tableName)
		tc.CreateTable(t, model)
	}
}

// ClearTableData removes all items from a table
func (tc *TestContext) ClearTableData(t *testing.T, tableName string) {
	t.Helper()

	ctx := context.TODO()

	// Get table description to understand key schema
	descResp, err := tc.DynamoDBClient.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: &tableName,
	})
	if err != nil {
		t.Logf("Failed to describe table %s for cleanup: %v", tableName, err)
		return
	}

	table := descResp.Table
	if table == nil {
		return
	}

	// Extract key attributes
	var partitionKey, sortKey string
	for _, keyElement := range table.KeySchema {
		switch keyElement.KeyType {
		case types.KeyTypeHash:
			partitionKey = *keyElement.AttributeName
		case types.KeyTypeRange:
			sortKey = *keyElement.AttributeName
		}
	}

	// Scan and delete all items
	scanInput := &dynamodb.ScanInput{
		TableName: &tableName,
	}

	for {
		scanResp, err := tc.DynamoDBClient.Scan(ctx, scanInput)
		if err != nil {
			t.Logf("Failed to scan table %s for cleanup: %v", tableName, err)
			break
		}

		// Delete items in batches
		if len(scanResp.Items) > 0 {
			tc.batchDeleteItems(t, tableName, scanResp.Items, partitionKey, sortKey)
		}

		// Check for more items
		if scanResp.LastEvaluatedKey == nil {
			break
		}
		scanInput.ExclusiveStartKey = scanResp.LastEvaluatedKey
	}
}

// batchDeleteItems deletes items in batches
func (tc *TestContext) batchDeleteItems(t *testing.T, tableName string, items []map[string]types.AttributeValue, partitionKey, sortKey string) {
	t.Helper()

	const batchSize = 25 // DynamoDB limit
	ctx := context.TODO()

	for i := 0; i < len(items); i += batchSize {
		end := i + batchSize
		if end > len(items) {
			end = len(items)
		}

		writeRequests := make([]types.WriteRequest, 0, end-i)

		for j := i; j < end; j++ {
			item := items[j]
			key := make(map[string]types.AttributeValue)

			// Add partition key
			if pk, exists := item[partitionKey]; exists {
				key[partitionKey] = pk
			}

			// Add sort key if it exists
			if sortKey != "" {
				if sk, exists := item[sortKey]; exists {
					key[sortKey] = sk
				}
			}

			writeRequests = append(writeRequests, types.WriteRequest{
				DeleteRequest: &types.DeleteRequest{
					Key: key,
				},
			})
		}

		if len(writeRequests) > 0 {
			input := &dynamodb.BatchWriteItemInput{
				RequestItems: map[string][]types.WriteRequest{
					tableName: writeRequests,
				},
			}

			_, err := tc.DynamoDBClient.BatchWriteItem(ctx, input)
			if err != nil {
				t.Logf("Failed to batch delete items from %s: %v", tableName, err)
			}
		}
	}
}

// WaitForTable waits for a table to become active
func (tc *TestContext) WaitForTable(t *testing.T, tableName string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.TODO(), 30*time.Second)
	defer cancel()

	// Reduce attempts for faster feedback
	maxAttempts := 30 // 30 seconds

	for i := 0; i < maxAttempts; i++ {
		select {
		case <-ctx.Done():
			t.Fatalf("Table %s creation timed out after 30 seconds", tableName)
			return
		default:
		}

		resp, err := tc.DynamoDBClient.DescribeTable(ctx, &dynamodb.DescribeTableInput{
			TableName: &tableName,
		})

		if err == nil && resp.Table != nil && resp.Table.TableStatus == types.TableStatusActive {
			t.Logf("Table %s is now active", tableName)
			return
		}

		// Log status every 5 seconds for debugging
		if i > 0 && i%5 == 0 {
			if resp != nil && resp.Table != nil {
				t.Logf("Table %s status: %s (attempt %d/%d)", tableName, resp.Table.TableStatus, i+1, maxAttempts)
			} else if err != nil {
				t.Logf("Table %s describe error: %v (attempt %d/%d)", tableName, err, i+1, maxAttempts)
			}
		}

		time.Sleep(1 * time.Second)
	}

	t.Fatalf("Table %s did not become active after %d seconds", tableName, maxAttempts)
}

// DeleteTable deletes a table (alternative cleanup strategy)
func (tc *TestContext) DeleteTable(t *testing.T, tableName string) {
	t.Helper()

	ctx := context.TODO()
	_, err := tc.DynamoDBClient.DeleteTable(ctx, &dynamodb.DeleteTableInput{
		TableName: &tableName,
	})

	if err != nil {
		// Ignore ResourceNotFoundException
		if !strings.Contains(err.Error(), "ResourceNotFoundException") {
			t.Logf("Failed to delete table %s: %v", tableName, err)
		}
	}

	// Wait for table to be deleted
	for i := 0; i < 30; i++ {
		_, err := tc.DynamoDBClient.DescribeTable(ctx, &dynamodb.DescribeTableInput{
			TableName: &tableName,
		})

		if err != nil && strings.Contains(err.Error(), "ResourceNotFoundException") {
			return // Table successfully deleted
		}

		time.Sleep(1 * time.Second)
	}
}

// AddCleanupFunc adds a custom cleanup function
func (tc *TestContext) AddCleanupFunc(cleanup func() error) {
	tc.cleanup = append(tc.cleanup, cleanup)
}

// Cleanup performs all registered cleanup operations
func (tc *TestContext) Cleanup() error {
	var errors []string

	// Run custom cleanup functions first
	for _, cleanup := range tc.cleanup {
		if err := cleanup(); err != nil {
			errors = append(errors, err.Error())
		}
	}

	// Cleanup strategy: Clear data instead of deleting tables to be faster
	for _, tableName := range tc.TablesCreated {
		tc.ClearTableData(&testing.T{}, tableName)
	}

	// Close database connection
	if err := tc.DB.Close(); err != nil {
		errors = append(errors, fmt.Sprintf("failed to close DB: %v", err))
	}

	if len(errors) > 0 {
		return fmt.Errorf("cleanup errors: %s", strings.Join(errors, "; "))
	}

	return nil
}

// Test utility functions

func isTableExistsError(err error) bool {
	return err != nil &&
		(strings.Contains(err.Error(), "ResourceInUseException") ||
			strings.Contains(err.Error(), "already exists"))
}

var testModelTableNames = map[string]string{
	"TestUser":     "TestUsers",
	"TestOrder":    "TestOrders",
	"TestProduct":  "TestProducts",
	"TestAccount":  "TestAccounts",
	"TestBlogPost": "TestBlogPosts",
	"TestComment":  "TestComments",
	"TestNote":     "TestNotes",
	"TestContact":  "TestContacts",
}

func getTableName(model any) string {
	// First try to call TableName() method if it exists
	if tableNamer, ok := model.(interface{ TableName() string }); ok {
		return tableNamer.TableName()
	}

	baseName := extractBaseTypeName(model)
	if baseName == "" {
		return ""
	}

	if tableName, ok := testModelTableNames[baseName]; ok {
		return tableName
	}

	return pluralizeTableName(sanitizeTableName(baseName))
}

func extractBaseTypeName(model any) string {
	typ := reflect.TypeOf(model)
	if typ == nil {
		return ""
	}
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	baseName := typ.Name()
	if baseName != "" {
		return baseName
	}

	// Last resort: use full type name and clean it up
	fullName := typ.String()
	if lastDot := strings.LastIndex(fullName, "."); lastDot != -1 {
		return fullName[lastDot+1:]
	}
	return fullName
}

func sanitizeTableName(name string) string {
	name = strings.ReplaceAll(name, "*", "")
	name = strings.ReplaceAll(name, ".", "_")
	name = strings.ReplaceAll(name, "/", "_")
	return name
}

func pluralizeTableName(name string) string {
	if strings.HasSuffix(name, "s") {
		return name
	}
	return name + "s"
}

// Common test model definitions for reuse across tests

type TestUser struct {
	CreatedAt time.Time `theorydb:"created_at"`
	UpdatedAt time.Time `theorydb:"updated_at"`
	ID        string    `theorydb:"pk"`
	Email     string    `theorydb:"index:gsi-email"`
	Name      string
	Active    bool
}

type TestOrder struct {
	CreatedAt  time.Time `theorydb:"created_at"`
	OrderID    string    `theorydb:"pk"`
	CustomerID string    `theorydb:"sk"`
	Status     string
	Amount     float64
}

type TestProduct struct {
	UpdatedAt time.Time `theorydb:"updated_at"`
	ProductID string    `theorydb:"pk"`
	Name      string
	Category  string `theorydb:"index:gsi-category"`
	Price     float64
	InStock   bool
}

type TestAccount struct {
	AccountID string `theorydb:"pk"`
	UserID    string `theorydb:"sk"`
	Type      string
	Balance   float64
	Version   int64 `theorydb:"version"`
}

type TestBlogPost struct {
	PublishedAt time.Time
	CreatedAt   time.Time `theorydb:"created_at"`
	UpdatedAt   time.Time `theorydb:"updated_at"`
	PostID      string    `theorydb:"pk"`
	Title       string
	Content     string
	AuthorID    string   `theorydb:"index:gsi-author"`
	Tags        []string `theorydb:"set"`
}

type TestComment struct {
	CreatedAt time.Time `theorydb:"created_at"`
	CommentID string    `theorydb:"pk"`
	PostID    string    `theorydb:"sk"`
	AuthorID  string
	Content   string
}

type TestNote struct {
	CreatedAt time.Time `theorydb:"created_at"`
	UpdatedAt time.Time `theorydb:"updated_at"`
	ID        string    `theorydb:"pk"`
	Title     string
	Content   string
	Tags      []string
	Priority  int
	Archived  bool
}

type TestContact struct {
	ID      string `theorydb:"pk"`
	Name    string
	Email   string
	Phone   string
	Company string
	Active  bool
}

// isDynamoDBLocalRunning checks if DynamoDB Local is accessible
func isDynamoDBLocalRunning(endpoint string) bool {
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(aws.CredentialsProviderFunc(
			func(_ context.Context) (aws.Credentials, error) {
				return aws.Credentials{
					AccessKeyID:     "dummy",
					SecretAccessKey: "dummy",
				}, nil
			})),
	)
	if err != nil {
		return false
	}

	client := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err = client.ListTables(ctx, &dynamodb.ListTablesInput{
		Limit: aws.Int32(1),
	})

	return err == nil
}
