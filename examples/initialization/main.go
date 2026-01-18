// Package main demonstrates proper TableTheory initialization patterns
// to avoid nil pointer dereference errors
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"github.com/theory-cloud/tabletheory"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/session"
)

// Example model
type User struct {
	ID        string `theorydb:"pk"`
	Email     string `theorydb:"sk"`
	Name      string
	CreatedAt string `theorydb:"created_at"`
	Active    bool
}

func main() {
	ctx := context.Background()

	// Demonstrate different initialization patterns
	fmt.Println("TableTheory Initialization Examples")
	fmt.Println("================================")

	// Example 1: Local Development
	fmt.Println("\n1. Local Development (DynamoDB Local)")
	if db, err := initializeLocal(ctx); err != nil {
		log.Printf("Local init failed: %v", err)
	} else {
		testDB(ctx, db, "local")
	}

	// Example 2: AWS Environment
	fmt.Println("\n2. AWS Environment (with credentials)")
	if db, err := initializeAWS(ctx); err != nil {
		log.Printf("AWS init failed: %v", err)
	} else {
		testDB(ctx, db, "aws")
	}

	// Example 3: Custom Profile
	fmt.Println("\n3. Custom AWS Profile")
	if profile := os.Getenv("AWS_PROFILE"); profile != "" {
		if db, err := initializeWithProfile(ctx, profile); err != nil {
			log.Printf("Profile init failed: %v", err)
		} else {
			testDB(ctx, db, "profile")
		}
	} else {
		fmt.Println("   Skipped: AWS_PROFILE not set")
	}

	// Example 4: Minimal Configuration
	fmt.Println("\n4. Minimal Configuration (may fail without AWS setup)")
	if db, err := initializeMinimal(ctx); err != nil {
		log.Printf("Minimal init failed: %v", err)
	} else {
		testDB(ctx, db, "minimal")
	}

	// Example 5: Debug AWS SDK directly
	fmt.Println("\n5. Debug: Test AWS SDK v2 Directly")
	debugAWSSDK(ctx)
}

// initializeLocal shows proper initialization for DynamoDB Local
func initializeLocal(ctx context.Context) (core.DB, error) {
	fmt.Println("   Initializing for DynamoDB Local...")

	sessionConfig := session.Config{
		Region:   "us-east-1",
		Endpoint: "http://localhost:8000",
		AWSConfigOptions: []func(*config.LoadOptions) error{
			// Critical: Must provide credentials for local DynamoDB
			config.WithCredentialsProvider(
				credentials.NewStaticCredentialsProvider("dummy", "dummy", ""),
			),
			config.WithRegion("us-east-1"),
		},
	}

	db, err := theorydb.NewBasic(sessionConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create TableTheory: %w", err)
	}

	fmt.Println("   ✓ Initialized successfully")
	return db, nil
}

// initializeAWS shows proper initialization for AWS environments
func initializeAWS(ctx context.Context) (core.DB, error) {
	fmt.Println("   Loading AWS configuration...")

	// First, ensure AWS config can be loaded
	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("us-east-1"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	fmt.Printf("   AWS Region: %s\n", awsCfg.Region)

	sessionConfig := session.Config{
		Region: awsCfg.Region,
		AWSConfigOptions: []func(*config.LoadOptions) error{
			config.WithRegion(awsCfg.Region),
		},
	}

	db, err := theorydb.NewBasic(sessionConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create TableTheory: %w", err)
	}

	fmt.Println("   ✓ Initialized successfully")
	return db, nil
}

// initializeWithProfile shows initialization with a specific AWS profile
func initializeWithProfile(ctx context.Context, profile string) (core.DB, error) {
	fmt.Printf("   Using AWS profile: %s\n", profile)

	sessionConfig := session.Config{
		Region: "us-east-1",
		AWSConfigOptions: []func(*config.LoadOptions) error{
			config.WithSharedConfigProfile(profile),
			config.WithRegion("us-east-1"),
		},
	}

	db, err := theorydb.NewBasic(sessionConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create TableTheory: %w", err)
	}

	fmt.Println("   ✓ Initialized successfully")
	return db, nil
}

// initializeMinimal shows the bare minimum configuration
func initializeMinimal(ctx context.Context) (core.DB, error) {
	fmt.Println("   Using minimal configuration...")

	// This is the minimum required - just a region
	sessionConfig := session.Config{
		Region: "us-east-1",
	}

	db, err := theorydb.NewBasic(sessionConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create TableTheory: %w", err)
	}

	fmt.Println("   ✓ Initialized successfully")
	return db, nil
}

// testDB performs a simple operation to verify the DB works
func testDB(ctx context.Context, db core.DB, testName string) {
	fmt.Printf("   Testing %s connection...\n", testName)

	// Try to create a query (this won't hit DynamoDB yet)
	query := db.Model(&User{})
	if query == nil {
		fmt.Println("   ✗ Failed: query is nil")
		return
	}

	fmt.Println("   ✓ DB instance working")

	// Note: Actual DynamoDB operations would fail here without proper setup
	// This is just to show the initialization succeeded
}

// debugAWSSDK tests AWS SDK v2 directly to help diagnose issues
func debugAWSSDK(ctx context.Context) {
	fmt.Println("   Testing AWS SDK v2 directly...")

	// Try to load default config
	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		fmt.Printf("   ✗ Failed to load AWS config: %v\n", err)
		return
	}

	fmt.Printf("   ✓ AWS config loaded (Region: %s)\n", awsCfg.Region)

	// Try to create DynamoDB client
	client := dynamodb.NewFromConfig(awsCfg)
	if client == nil {
		fmt.Println("   ✗ Failed: DynamoDB client is nil")
		return
	}

	fmt.Println("   ✓ DynamoDB client created")

	// Try to list tables (this will fail without credentials)
	limit := int32(1)
	_, err = client.ListTables(ctx, &dynamodb.ListTablesInput{
		Limit: &limit,
	})

	if err != nil {
		fmt.Printf("   ℹ ListTables failed (expected without credentials): %v\n", err)
	} else {
		fmt.Println("   ✓ ListTables succeeded - AWS credentials are configured")
	}
}
