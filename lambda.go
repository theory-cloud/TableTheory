// lambda.go
package theorydb

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/session"
	pkgTypes "github.com/theory-cloud/tabletheory/pkg/types"
)

var (
	// Global Lambda-optimized DB for connection reuse
	globalLambdaDB *LambdaDB
	lambdaOnce     sync.Once

	benchmarkLoadDefaultConfig = config.LoadDefaultConfig
	benchmarkNewDynamoDBClient = dynamodb.NewFromConfig
)

// Package theorydb provides Lambda-specific optimizations for DynamoDB operations.
//
// Best Practices for Lambda:
//
// 1. Initialize globally to reuse connections across invocations:
//    var db *theorydb.LambdaDB
//    func init() {
//        db, _ = theorydb.LambdaInit(&User{}, &Post{})
//    }
//
// 2. Use context with Lambda timeout:
//    func handler(ctx context.Context, event Event) error {
//        lambdaDB := db.WithLambdaTimeout(ctx)
//        // Use lambdaDB for all operations
//    }
//
// 3. Connection Pool Sizing:
//    - 128MB-512MB: 5 connections
//    - 512MB-1GB: 10 connections
//    - 1GB+: 20 connections
//
// 4. Cold Start Optimization:
//    - Pre-register all models in init()
//    - Use LambdaInit() helper
//    - Consider increasing Lambda memory for faster CPU
//
// 5. Monitoring:
//    - Use GetMemoryStats() to track memory usage
//    - Log cold start metrics in production
//    - Monitor DynamoDB throttling

// LambdaDB wraps DB with Lambda-specific optimizations
type LambdaDB struct {
	core.ExtendedDB
	db             *DB
	modelCache     *sync.Map
	lambdaMemoryMB int
	isLambda       bool
	xrayEnabled    bool
}

// NewLambdaOptimized creates a Lambda-optimized DB instance
func NewLambdaOptimized() (*LambdaDB, error) {
	// Use global instance if available (warm start)
	if globalLambdaDB != nil {
		return globalLambdaDB, nil
	}

	var err error
	lambdaOnce.Do(func() {
		globalLambdaDB, err = createLambdaDB()
	})

	return globalLambdaDB, err
}

// createLambdaDB creates the actual Lambda DB instance
func createLambdaDB() (*LambdaDB, error) {
	// Detect Lambda environment
	isLambda := IsLambdaEnvironment()
	memoryMB := GetLambdaMemoryMB()

	// Create optimized HTTP client for Lambda
	httpClient := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
			DisableKeepAlives:   false, // Keep connections alive for reuse
		},
	}

	// Load AWS config with Lambda optimizations
	awsConfigOptions := []func(*config.LoadOptions) error{
		config.WithRegion(getRegion()),
		config.WithHTTPClient(httpClient),
		config.WithRetryMode(aws.RetryModeAdaptive),
		config.WithRetryMaxAttempts(3),
	}

	// Enable X-Ray tracing automatically when running in Lambda. The AWS SDK picks up
	// X-Ray configuration from the environment, so no explicit setup is required here.

	cfg := session.Config{
		Region:           getRegion(),
		MaxRetries:       3,
		DefaultRCU:       5,
		DefaultWCU:       5,
		AutoMigrate:      false,
		EnableMetrics:    isLambda,
		AWSConfigOptions: awsConfigOptions,
	}

	// Optimize DynamoDB client options for Lambda
	if isLambda {
		cfg.DynamoDBOptions = append(cfg.DynamoDBOptions, func(o *dynamodb.Options) {
			// Lambda-specific optimizations
			o.RetryMode = aws.RetryModeAdaptive
		})
	}

	db, err := New(cfg)
	if err != nil {
		return nil, err
	}

	// Type assert to get the concrete DB
	concreteDB, ok := db.(*DB)
	if !ok {
		return nil, fmt.Errorf("failed to get concrete DB implementation")
	}

	ldb := &LambdaDB{
		ExtendedDB:     db,
		db:             concreteDB,
		modelCache:     &sync.Map{},
		isLambda:       isLambda,
		lambdaMemoryMB: memoryMB,
		xrayEnabled:    os.Getenv("_X_AMZN_TRACE_ID") != "",
	}

	return ldb, nil
}

// PreRegisterModels registers models at init time to reduce cold starts
func (ldb *LambdaDB) PreRegisterModels(models ...any) error {
	for _, model := range models {
		if err := ldb.db.registry.Register(model); err != nil {
			return err
		}
		// Cache the model type for fast lookup
		modelType := reflect.TypeOf(model)
		if modelType.Kind() == reflect.Ptr {
			modelType = modelType.Elem()
		}
		ldb.modelCache.Store(modelType, true)
	}
	return nil
}

// RegisterTypeConverter registers a custom converter on the underlying DB and
// clears any cached marshalers so the converter takes effect immediately.
func (ldb *LambdaDB) RegisterTypeConverter(typ reflect.Type, converter pkgTypes.CustomConverter) error {
	if ldb == nil || ldb.db == nil {
		return fmt.Errorf("lambda DB is not initialized")
	}
	return ldb.db.RegisterTypeConverter(typ, converter)
}

// IsModelRegistered checks if a model is already registered
func (ldb *LambdaDB) IsModelRegistered(model any) bool {
	modelType := reflect.TypeOf(model)
	if modelType.Kind() == reflect.Ptr {
		modelType = modelType.Elem()
	}
	_, ok := ldb.modelCache.Load(modelType)
	return ok
}

// WithLambdaTimeout creates a new DB instance with Lambda timeout handling
func (ldb *LambdaDB) WithLambdaTimeout(ctx context.Context) *LambdaDB {
	deadline, ok := ctx.Deadline()
	if !ok {
		return ldb
	}

	// Leave 1 second buffer for Lambda cleanup
	adjustedDeadline := deadline.Add(-1 * time.Second)

	newDB := &DB{
		session:        ldb.db.session,
		registry:       ldb.db.registry,
		converter:      ldb.db.converter,
		marshaler:      ldb.db.marshaler,
		ctx:            ctx,
		lambdaDeadline: adjustedDeadline,
	}

	return &LambdaDB{
		ExtendedDB:     newDB,
		db:             newDB,
		modelCache:     ldb.modelCache, // Share the same model cache pointer
		isLambda:       ldb.isLambda,
		lambdaMemoryMB: ldb.lambdaMemoryMB,
		xrayEnabled:    ldb.xrayEnabled,
	}
}

// OptimizeForMemory adjusts internal buffers based on available Lambda memory
func (ldb *LambdaDB) OptimizeForMemory() {
	// Adjust batch sizes based on memory
	memoryMB := ldb.lambdaMemoryMB
	if memoryMB == 0 {
		memoryMB = 512 // Default
	}

	// Scale timeout buffers with available memory
	buffer := 100 * time.Millisecond
	if memoryMB >= 2048 {
		buffer = 50 * time.Millisecond
	} else if memoryMB <= 512 {
		buffer = 200 * time.Millisecond
	}

	if ldb.db != nil {
		ldb.db.lambdaTimeoutBuffer = buffer
	}
}

// OptimizeForColdStart reduces Lambda cold start time
func (ldb *LambdaDB) OptimizeForColdStart() {
	// Pre-warm the connection pool
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("theorydb: lambda pre-warm encountered panic: %v", r)
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		// Perform a lightweight operation to establish connection
		client, err := ldb.db.session.Client()
		if err != nil {
			// Connection pre-warming failed, but we continue normally
			return
		}

		_, err = client.ListTables(ctx, &dynamodb.ListTablesInput{
			Limit: aws.Int32(1),
		})
		if err != nil {
			return
		}
	}()

	// Pre-compile common expressions if using a query builder
	if ldb.isLambda {
		// Initialize expression builder cache
		_ = ldb.Model(struct{}{})
	}
}

// GetMemoryStats returns current memory usage statistics
func (ldb *LambdaDB) GetMemoryStats() LambdaMemoryStats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return LambdaMemoryStats{
		Alloc:          m.Alloc,
		TotalAlloc:     m.TotalAlloc,
		Sys:            m.Sys,
		NumGC:          m.NumGC,
		AllocatedMB:    float64(m.Alloc) / 1024 / 1024,
		SystemMB:       float64(m.Sys) / 1024 / 1024,
		LambdaMemoryMB: ldb.lambdaMemoryMB,
		MemoryPercent:  (float64(m.Sys) / 1024 / 1024) / float64(ldb.lambdaMemoryMB) * 100,
	}
}

// LambdaMemoryStats contains memory usage information
type LambdaMemoryStats struct {
	Alloc          uint64  // Bytes allocated and still in use
	TotalAlloc     uint64  // Bytes allocated (even if freed)
	Sys            uint64  // Bytes obtained from system
	NumGC          uint32  // Number of GC cycles
	AllocatedMB    float64 // MB currently allocated
	SystemMB       float64 // MB obtained from system
	LambdaMemoryMB int     // Total Lambda memory allocation
	MemoryPercent  float64 // Percentage of Lambda memory used
}

// Lambda environment helper functions

// IsLambdaEnvironment detects if running in AWS Lambda
func IsLambdaEnvironment() bool {
	return os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != ""
}

// GetLambdaMemoryMB returns the allocated memory in MB
func GetLambdaMemoryMB() int {
	memStr := os.Getenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE")
	if memStr == "" {
		return 0
	}

	mem, err := strconv.Atoi(memStr)
	if err != nil {
		return 0
	}

	return mem
}

// EnableXRayTracing enables AWS X-Ray tracing for DynamoDB calls
func EnableXRayTracing() bool {
	return os.Getenv("_X_AMZN_TRACE_ID") != ""
}

// getRegion returns the AWS region from environment
func getRegion() string {
	if region := os.Getenv("AWS_REGION"); region != "" {
		return region
	}
	// Fallback to default region
	return "us-east-1"
}

// GetRemainingTimeMillis returns milliseconds until Lambda timeout
func GetRemainingTimeMillis(ctx context.Context) int64 {
	deadline, ok := ctx.Deadline()
	if !ok {
		return -1
	}

	remaining := time.Until(deadline)
	return remaining.Milliseconds()
}

// LambdaInit should be called in the init() function of your Lambda handler
// It performs one-time initialization to reduce cold start latency
func LambdaInit(models ...any) (*LambdaDB, error) {
	// Create Lambda-optimized DB
	db, err := NewLambdaOptimized()
	if err != nil {
		return nil, err
	}

	// Pre-register models
	if len(models) > 0 {
		if err := db.PreRegisterModels(models...); err != nil {
			return nil, err
		}
	}

	// Optimize for cold start
	db.OptimizeForColdStart()

	// Optimize based on Lambda memory
	db.OptimizeForMemory() // Uses auto-detected memory

	return db, nil
}

// BenchmarkColdStart measures cold start performance
func BenchmarkColdStart(models ...any) ColdStartMetrics {
	start := time.Now()

	// Track initialization phases
	phases := make(map[string]time.Duration)

	// Phase 1: AWS Config
	phaseStart := time.Now()
	cfg, err := benchmarkLoadDefaultConfig(context.Background())
	phases["aws_config"] = time.Since(phaseStart)
	if err != nil {
		// If config loading fails, still track it but with error
		phases["aws_config_error"] = time.Since(phaseStart)
		return ColdStartMetrics{
			TotalDuration: time.Since(start),
			Phases:        phases,
			MemoryMB:      GetLambdaMemoryMB(),
			IsLambda:      IsLambdaEnvironment(),
		}
	}

	// Phase 2: DynamoDB Client
	phaseStart = time.Now()
	client := benchmarkNewDynamoDBClient(cfg)
	phases["dynamodb_client"] = time.Since(phaseStart)

	// Phase 3: TableTheory Setup
	phaseStart = time.Now()
	db, err := NewLambdaOptimized()
	phases["theorydb_setup"] = time.Since(phaseStart)
	if err != nil {
		phases["theorydb_setup_error"] = phases["theorydb_setup"]
		return ColdStartMetrics{
			TotalDuration: time.Since(start),
			Phases:        phases,
			MemoryMB:      GetLambdaMemoryMB(),
			IsLambda:      IsLambdaEnvironment(),
		}
	}

	// Phase 4: Model Registration
	if len(models) > 0 {
		phaseStart = time.Now()
		if err := db.PreRegisterModels(models...); err != nil {
			// If model registration fails, still track it but with error
			phases["model_registration_error"] = time.Since(phaseStart)
			return ColdStartMetrics{
				TotalDuration: time.Since(start),
				Phases:        phases,
				MemoryMB:      GetLambdaMemoryMB(),
				IsLambda:      IsLambdaEnvironment(),
			}
		}
		phases["model_registration"] = time.Since(phaseStart)
	}

	// Phase 5: First Query (connection establishment)
	phaseStart = time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	if _, err := client.ListTables(ctx, &dynamodb.ListTablesInput{Limit: aws.Int32(1)}); err != nil {
		duration := time.Since(phaseStart)
		phases["first_connection_error"] = duration
		phases["first_connection"] = duration
	} else {
		phases["first_connection"] = time.Since(phaseStart)
	}

	totalDuration := time.Since(start)

	return ColdStartMetrics{
		TotalDuration: totalDuration,
		Phases:        phases,
		MemoryMB:      GetLambdaMemoryMB(),
		IsLambda:      IsLambdaEnvironment(),
	}
}

// ColdStartMetrics contains cold start performance data
type ColdStartMetrics struct {
	Phases        map[string]time.Duration
	TotalDuration time.Duration
	MemoryMB      int
	IsLambda      bool
}

// String returns a formatted string of the metrics
func (m ColdStartMetrics) String() string {
	var result strings.Builder
	result.WriteString(fmt.Sprintf("Cold Start Metrics (Total: %v)\n", m.TotalDuration))
	result.WriteString(fmt.Sprintf("Lambda Memory: %d MB\n", m.MemoryMB))
	result.WriteString("Phases:\n")

	// Sort phases for consistent output
	phases := make([]string, 0, len(m.Phases))
	for phase := range m.Phases {
		phases = append(phases, phase)
	}
	sort.Strings(phases)

	for _, phase := range phases {
		result.WriteString(fmt.Sprintf("  %s: %v\n", phase, m.Phases[phase]))
	}

	return result.String()
}
