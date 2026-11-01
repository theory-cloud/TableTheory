// Package main demonstrates proper TableTheory initialization patterns
// to avoid nil pointer dereference errors
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"github.com/theory-cloud/tabletheory/v6"
	"github.com/theory-cloud/tabletheory/v6/pkg/core"
	"github.com/theory-cloud/tabletheory/v6/pkg/session"
)

// Example model
type User struct {
	ID        string `theorydb:"pk"`
	Email     string `theorydb:"sk"`
	Name      string
	CreatedAt time.Time `theorydb:"created_at"`
	Active    bool
}

func main() {
	mode := flag.String(
		"mode",
		"all",
		"initialization path to run: all, local, aws, profile, minimal or debug",
	)
	flag.Parse()
	ctx := context.Background()

	// Demonstrate different initialization patterns. The local path runs
	// against DynamoDB Local and is the only path the examples harness runs.
	// The aws, profile, minimal and debug paths resolve the real AWS defaults;
	// they are L3 (named cloud gap: a real AWS account and credentials) and
	// never run under make examples-local.
	fmt.Println("TableTheory Initialization Examples")
	fmt.Println("================================")

	if *mode == "all" || *mode == "local" {
		fmt.Println("\n1. Local Development (DynamoDB Local)")
		if db, err := initializeLocal(ctx); err != nil {
			log.Printf("Local init failed: %v", err)
		} else {
			testDB(ctx, db, "local")
		}
	}

	if *mode == "all" || *mode == "aws" {
		fmt.Println("\n2. AWS Environment (with credentials)")
		if db, err := initializeAWS(ctx); err != nil {
			log.Printf("AWS init failed: %v", err)
		} else {
			testDB(ctx, db, "aws")
		}
	}

	if *mode == "all" || *mode == "profile" {
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
	}

	if *mode == "all" || *mode == "minimal" {
		fmt.Println("\n4. Minimal Configuration (may fail without AWS setup)")
		if db, err := initializeMinimal(ctx); err != nil {
			log.Printf("Minimal init failed: %v", err)
		} else {
			testDB(ctx, db, "minimal")
		}
	}

	if *mode == "all" || *mode == "debug" {
		fmt.Println("\n5. Debug: Test AWS SDK v2 Directly")
		debugAWSSDK(ctx)
	}
}

// localEndpoint honours the local harness endpoint while keeping the historical
// localhost:8000 default for a hand-started DynamoDB Local.
func localEndpoint() string {
	for _, key := range []string{"DYNAMODB_ENDPOINT", "AWS_ENDPOINT_URL_DYNAMODB"} {
		if endpoint := strings.TrimSpace(os.Getenv(key)); endpoint != "" {
			return endpoint
		}
	}
	return "http://localhost:8000"
}

// initializeLocal shows proper initialization for DynamoDB Local. The local
// path creates the example table so the real round-trip in testDB can run.
func initializeLocal(ctx context.Context) (core.ExtendedDB, error) {
	fmt.Println("   Initializing for DynamoDB Local...")

	sessionConfig := session.Config{
		Region:   "us-east-1",
		Endpoint: localEndpoint(),
		AWSConfigOptions: []func(*config.LoadOptions) error{
			// Critical: Must provide credentials for local DynamoDB
			config.WithCredentialsProvider(
				credentials.NewStaticCredentialsProvider("dummy", "dummy", ""),
			),
			config.WithRegion("us-east-1"),
		},
	}

	db, err := tabletheory.New(sessionConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create TableTheory: %w", err)
	}
	if err := db.EnsureTable(&User{}); err != nil {
		return nil, fmt.Errorf("failed to ensure the local table: %w", err)
	}

	fmt.Printf("   ✓ Initialized successfully against %s\n", localEndpoint())
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

	db, err := tabletheory.NewBasic(sessionConfig)
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

	db, err := tabletheory.NewBasic(sessionConfig)
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

	db, err := tabletheory.NewBasic(sessionConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create TableTheory: %w", err)
	}

	fmt.Println("   ✓ Initialized successfully")
	return db, nil
}

// testDB performs a real DynamoDB write and read so the reported success is an
// observed outcome rather than a constructed-client claim.
func testDB(ctx context.Context, db core.DB, testName string) {
	fmt.Printf("   Testing %s connection with a real write and read...\n", testName)

	user := &User{
		ID:        fmt.Sprintf("INIT#%s#%d", testName, time.Now().UnixNano()),
		Email:     "init-" + testName + "@example.test",
		Name:      "Initialization " + testName,
		CreatedAt: time.Now().UTC(),
		Active:    true,
	}
	if err := db.WithContext(ctx).Model(user).Create(); err != nil {
		fmt.Printf("   ✗ real write failed: %v\n", err)
		return
	}

	var got User
	if err := db.WithContext(ctx).Model(&User{}).
		Where("ID", "=", user.ID).
		First(&got); err != nil {
		fmt.Printf("   ✗ real read failed: %v\n", err)
		return
	}
	if got.ID != user.ID || got.Email != user.Email {
		fmt.Printf("   ✗ real read mismatch: got %s/%s, want %s/%s\n",
			got.ID, got.Email, user.ID, user.Email)
		return
	}

	fmt.Printf("   ✓ real round-trip: wrote and read %s/%s\n", got.ID, got.Email)
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
