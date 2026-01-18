package benchmarks

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/theory-cloud/tabletheory"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/session"
	"github.com/theory-cloud/tabletheory/tests/models"
)

var (
	benchDB        core.ExtendedDB
	benchDynamoDB  *dynamodb.Client
	benchTableName = "BenchmarkTable"
)

func setupBenchDB(b *testing.B) (core.ExtendedDB, *dynamodb.Client) {
	if benchDB != nil && benchDynamoDB != nil {
		return benchDB, benchDynamoDB
	}

	// Skip if DynamoDB Local is not available
	// Note: For benchmarks, we assume DynamoDB Local is running

	// Create AWS config for DynamoDB client
	awsCfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("dummy", "dummy", ""),
		),
	)
	if err != nil {
		b.Fatal(err)
	}

	// Create DynamoDB client with endpoint override
	dynamoClient := dynamodb.NewFromConfig(awsCfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String("http://localhost:8000")
	})

	// Fixed initialization with session.Config for TableTheory
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
	if err != nil {
		b.Fatalf("Failed to create DB: %v", err)
	}

	benchDB = db
	benchDynamoDB = dynamoClient

	// Create test table
	createBenchTable(b)

	// Seed initial data
	seedBenchData(b)

	return benchDB, benchDynamoDB
}

func createBenchTable(b *testing.B) {
	ctx := context.TODO()

	// Delete existing table if it exists
	_, err := benchDynamoDB.DeleteTable(ctx, &dynamodb.DeleteTableInput{
		TableName: aws.String(benchTableName),
	})
	if err != nil {
		var notFound *types.ResourceNotFoundException
		if !errors.As(err, &notFound) {
			b.Fatalf("delete table %s: %v", benchTableName, err)
		}
	}

	// Wait a bit for deletion
	time.Sleep(2 * time.Second)

	// Create table
	input := &dynamodb.CreateTableInput{
		TableName: aws.String(benchTableName),
		KeySchema: []types.KeySchemaElement{
			{
				AttributeName: aws.String("ID"),
				KeyType:       types.KeyTypeHash,
			},
		},
		AttributeDefinitions: []types.AttributeDefinition{
			{
				AttributeName: aws.String("ID"),
				AttributeType: types.ScalarAttributeTypeS,
			},
			{
				AttributeName: aws.String("Email"),
				AttributeType: types.ScalarAttributeTypeS,
			},
			{
				AttributeName: aws.String("Status"),
				AttributeType: types.ScalarAttributeTypeS,
			},
		},
		BillingMode: types.BillingModePayPerRequest,
		GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{
			{
				IndexName: aws.String("gsi-email"),
				KeySchema: []types.KeySchemaElement{
					{
						AttributeName: aws.String("Email"),
						KeyType:       types.KeyTypeHash,
					},
				},
				Projection: &types.Projection{
					ProjectionType: types.ProjectionTypeAll,
				},
			},
			{
				IndexName: aws.String("gsi-status"),
				KeySchema: []types.KeySchemaElement{
					{
						AttributeName: aws.String("Status"),
						KeyType:       types.KeyTypeHash,
					},
				},
				Projection: &types.Projection{
					ProjectionType: types.ProjectionTypeAll,
				},
			},
		},
	}

	_, err = benchDynamoDB.CreateTable(ctx, input)
	if err != nil {
		b.Fatal(err)
	}

	// Wait for table to be active
	for i := 0; i < 30; i++ {
		desc, err := benchDynamoDB.DescribeTable(ctx, &dynamodb.DescribeTableInput{
			TableName: aws.String(benchTableName),
		})
		if err == nil && desc.Table.TableStatus == "ACTIVE" {
			// Check all GSIs are active
			allActive := true
			for _, gsi := range desc.Table.GlobalSecondaryIndexes {
				if gsi.IndexStatus != "ACTIVE" {
					allActive = false
					break
				}
			}
			if allActive {
				return
			}
		}
		time.Sleep(1 * time.Second)
	}
	b.Fatal("Table not active after 30 seconds")
}

func seedBenchData(b *testing.B) {
	ctx := context.TODO()

	// Seed 1000 items for benchmark
	for i := 0; i < 1000; i++ {
		item := map[string]types.AttributeValue{
			"ID":        &types.AttributeValueMemberS{Value: fmt.Sprintf("bench-user-%d", i)},
			"Email":     &types.AttributeValueMemberS{Value: fmt.Sprintf("bench%d@example.com", i)},
			"Status":    &types.AttributeValueMemberS{Value: "active"},
			"Age":       &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", 20+(i%50))},
			"Name":      &types.AttributeValueMemberS{Value: fmt.Sprintf("Bench User %d", i)},
			"CreatedAt": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
		}

		_, err := benchDynamoDB.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: aws.String(benchTableName),
			Item:      item,
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Benchmarks

func BenchmarkSimpleQuery(b *testing.B) {
	db, _ := setupBenchDB(b)
	user := &models.TestUser{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := db.Model(&models.TestUser{}).
			Where("ID", "=", "bench-user-100").
			First(user)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRawSDKQuery(b *testing.B) {
	_, client := setupBenchDB(b)
	ctx := context.TODO()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		output, err := client.GetItem(ctx, &dynamodb.GetItemInput{
			TableName: aws.String(benchTableName),
			Key: map[string]types.AttributeValue{
				"ID": &types.AttributeValueMemberS{Value: "bench-user-100"},
			},
		})
		if err != nil {
			b.Fatal(err)
		}
		if output.Item == nil {
			b.Fatal("Item not found")
		}
	}
}

func BenchmarkComplexQueryWithFilters(b *testing.B) {
	db, _ := setupBenchDB(b)
	var users []models.TestUser

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := db.Model(&models.TestUser{}).
			Where("Status", "=", "active").
			Filter("Age", ">", 25).
			Filter("Age", "<", 35).
			Limit(20).
			All(&users)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkIndexSelection(b *testing.B) {
	db, _ := setupBenchDB(b)

	// Pre-warm the registry with model metadata
	db.Model(&models.TestUser{})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Measure just the query building and index selection
		query := db.Model(&models.TestUser{}).
			Where("Email", "=", "bench100@example.com").
			Where("Status", "=", "active")

		// Force compilation without execution
		_ = query
	}
}

func BenchmarkExpressionBuilding(b *testing.B) {
	db, _ := setupBenchDB(b)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Build a complex query to measure expression building overhead
		query := db.Model(&models.TestUser{}).
			Where("Status", "=", "active").
			Filter("Age", ">", 25).
			OrderBy("CreatedAt", "desc").
			Select("ID", "Email", "Name", "Age").
			Limit(50)

		// Force compilation without execution
		_ = query
	}
}

func BenchmarkBatchGet(b *testing.B) {
	db, _ := setupBenchDB(b)

	// Prepare keys
	keys := make([]any, 20)
	for i := 0; i < 20; i++ {
		keys[i] = fmt.Sprintf("bench-user-%d", i*10)
	}

	var users []models.TestUser

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := db.Model(&models.TestUser{}).BatchGet(keys, &users)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkScanWithFilters(b *testing.B) {
	db, _ := setupBenchDB(b)
	var users []models.TestUser

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := db.Model(&models.TestUser{}).
			Filter("Age", ">", 30).
			Limit(50).
			Scan(&users)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Helper function to calculate overhead percentage
func calculateOverhead(theorydbTime, sdkTime time.Duration) float64 {
	return float64(theorydbTime-sdkTime) / float64(sdkTime) * 100
}

// Comparative benchmark to measure overhead
func BenchmarkOverheadComparison(b *testing.B) {
	db, client := setupBenchDB(b)
	ctx := context.TODO()

	// Benchmark TableTheory
	start := time.Now()
	for i := 0; i < 1000; i++ {
		user := &models.TestUser{}
		err := db.Model(&models.TestUser{}).
			Where("ID", "=", fmt.Sprintf("bench-user-%d", i%100)).
			First(user)
		if err != nil {
			b.Fatal(err)
		}
	}
	theorydbTime := time.Since(start)

	// Benchmark raw SDK
	start = time.Now()
	for i := 0; i < 1000; i++ {
		output, err := client.GetItem(ctx, &dynamodb.GetItemInput{
			TableName: aws.String(benchTableName),
			Key: map[string]types.AttributeValue{
				"ID": &types.AttributeValueMemberS{Value: fmt.Sprintf("bench-user-%d", i%100)},
			},
		})
		if err != nil {
			b.Fatal(err)
		}
		if output.Item == nil {
			b.Fatal("Item not found")
		}
	}
	sdkTime := time.Since(start)

	overhead := calculateOverhead(theorydbTime, sdkTime)
	b.Logf("TableTheory time: %v, SDK time: %v, Overhead: %.2f%%", theorydbTime, sdkTime, overhead)

	if overhead > 5.0 {
		b.Errorf("Overhead %.2f%% exceeds 5%% target", overhead)
	}
}
