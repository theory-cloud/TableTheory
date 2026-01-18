package tests

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// TestConfig holds test environment configuration
type TestConfig struct {
	Endpoint        string
	Region          string
	SkipIntegration bool
}

// GetTestConfig returns the test configuration
func GetTestConfig() *TestConfig {
	endpoint := os.Getenv("DYNAMODB_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:8000"
	}

	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-1"
	}

	// Check if we should skip integration tests
	skipIntegration := os.Getenv("SKIP_INTEGRATION") == "true"

	return &TestConfig{
		Endpoint:        endpoint,
		Region:          region,
		SkipIntegration: skipIntegration,
	}
}

// RequireDynamoDBLocal skips the test if DynamoDB Local is not available
func RequireDynamoDBLocal(t *testing.T) {
	t.Helper()

	if testing.Short() {
		t.Skip("Skipping integration test in -short mode")
	}

	config := GetTestConfig()
	if config.SkipIntegration {
		t.Skip("Skipping integration test (SKIP_INTEGRATION=true)")
	}

	// Check if DynamoDB Local is running
	if !isDynamoDBLocalRunning(config.Endpoint) {
		t.Skip("DynamoDB Local is not running. Run ./tests/setup_test_env.sh to start it.")
	}
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

// CleanupTestTables removes all test tables
func CleanupTestTables(t *testing.T, client *dynamodb.Client) {
	t.Helper()

	ctx := context.TODO()

	// List all tables
	resp, err := client.ListTables(ctx, &dynamodb.ListTablesInput{})
	if err != nil {
		t.Logf("Failed to list tables: %v", err)
		return
	}

	// Delete each table
	for _, tableName := range resp.TableNames {
		_, err := client.DeleteTable(ctx, &dynamodb.DeleteTableInput{
			TableName: aws.String(tableName),
		})
		if err != nil {
			t.Logf("Failed to delete table %s: %v", tableName, err)
		} else {
			t.Logf("Deleted table: %s", tableName)
		}
	}

	// Wait for tables to be deleted
	time.Sleep(2 * time.Second)
}

// CreateTestTable creates a simple test table
func CreateTestTable(t *testing.T, client *dynamodb.Client, tableName string) {
	t.Helper()

	input := &dynamodb.CreateTableInput{
		TableName: aws.String(tableName),
		AttributeDefinitions: []types.AttributeDefinition{
			{
				AttributeName: aws.String("ID"),
				AttributeType: types.ScalarAttributeTypeS,
			},
		},
		KeySchema: []types.KeySchemaElement{
			{
				AttributeName: aws.String("ID"),
				KeyType:       types.KeyTypeHash,
			},
		},
		BillingMode: types.BillingModePayPerRequest,
	}

	ctx := context.TODO()
	_, err := client.CreateTable(ctx, input)
	if err != nil {
		t.Fatalf("Failed to create table %s: %v", tableName, err)
	}

	// Wait for table to be active
	waiter := dynamodb.NewTableExistsWaiter(client)
	err = waiter.Wait(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	}, 30*time.Second)
	if err != nil {
		t.Fatalf("Failed waiting for table %s: %v", tableName, err)
	}
}

// WaitForTable waits for a table to become active
func WaitForTable(t *testing.T, client *dynamodb.Client, tableName string) {
	t.Helper()

	ctx := context.TODO()
	maxAttempts := 30

	for i := 0; i < maxAttempts; i++ {
		resp, err := client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
			TableName: aws.String(tableName),
		})

		if err == nil && resp.Table.TableStatus == types.TableStatusActive {
			return
		}

		time.Sleep(1 * time.Second)
	}

	t.Fatalf("Table %s did not become active", tableName)
}
