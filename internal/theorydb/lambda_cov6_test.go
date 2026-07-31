package theorydb

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v3/pkg/session"
)

func TestBenchmarkColdStart_CoversSuccessAndErrorPaths_COV6(t *testing.T) {
	origLoad := benchmarkLoadDefaultConfig
	origNewClient := benchmarkNewDynamoDBClient

	t.Cleanup(func() {
		globalLambdaDB = nil
		globalLambdaDBErr = nil
		lambdaOnce = sync.Once{}
		benchmarkLoadDefaultConfig = origLoad
		benchmarkNewDynamoDBClient = origNewClient
	})

	resetGlobals := func() {
		globalLambdaDB = nil
		globalLambdaDBErr = nil
		lambdaOnce = sync.Once{}
	}

	t.Run("config load error", func(t *testing.T) {
		resetGlobals()

		benchmarkLoadDefaultConfig = func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
			return aws.Config{}, errors.New("boom")
		}

		metrics := BenchmarkColdStart()
		require.NotEmpty(t, metrics.Phases["aws_config_error"])
	})

	t.Run("theorydb setup error", func(t *testing.T) {
		resetGlobals()

		benchmarkLoadDefaultConfig = func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
			return minimalAWSConfig(nil), nil
		}

		stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
			return aws.Config{}, errors.New("session boom")
		})

		metrics := BenchmarkColdStart()
		require.NotEmpty(t, metrics.Phases["theorydb_setup_error"])
	})

	t.Run("model registration error", func(t *testing.T) {
		resetGlobals()

		benchmarkLoadDefaultConfig = func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
			return minimalAWSConfig(nil), nil
		}

		stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
			return minimalAWSConfig(nil), nil
		})

		metrics := BenchmarkColdStart(123)
		require.NotEmpty(t, metrics.Phases["model_registration_error"])
	})

	t.Run("success", func(t *testing.T) {
		resetGlobals()

		httpClient := newCapturingHTTPClient(map[string]string{
			"DynamoDB_20120810.ListTables": `{"TableNames":[]}`,
		})

		benchmarkLoadDefaultConfig = func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
			return minimalAWSConfig(httpClient), nil
		}
		benchmarkNewDynamoDBClient = dynamodb.NewFromConfig

		stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
			return minimalAWSConfig(httpClient), nil
		})

		metrics := BenchmarkColdStart(&cov4LambdaModel{})
		require.NotEmpty(t, metrics.Phases["aws_config"])
		require.NotEmpty(t, metrics.Phases["dynamodb_client"])
		require.NotEmpty(t, metrics.Phases["theorydb_setup"])
		require.NotEmpty(t, metrics.Phases["model_registration"])
		require.NotEmpty(t, metrics.Phases["first_connection"])
		require.Greater(t, metrics.TotalDuration, time.Duration(0))
	})
}

func TestNewLambdaOptimized_WarmStartReturnsGlobal_COV6(t *testing.T) {
	t.Cleanup(func() {
		globalLambdaDB = nil
		globalLambdaDBErr = nil
		lambdaOnce = sync.Once{}
	})

	globalLambdaDB = &LambdaDB{}
	globalLambdaDBErr = nil
	lambdaOnce = sync.Once{}

	got, err := NewLambdaOptimized()
	require.NoError(t, err)
	require.Same(t, globalLambdaDB, got)
}

func TestNewLambdaOptimized_ReturnsCachedInitErrorAfterFailedColdStart_COV6(t *testing.T) {
	t.Cleanup(func() {
		globalLambdaDB = nil
		globalLambdaDBErr = nil
		lambdaOnce = sync.Once{}
	})

	globalLambdaDB = nil
	globalLambdaDBErr = nil
	lambdaOnce = sync.Once{}

	initErr := errors.New("session boom")
	calls := 0
	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		calls++
		if calls == 1 {
			return aws.Config{}, initErr
		}
		return minimalAWSConfig(nil), nil
	})

	first, firstErr := NewLambdaOptimized()
	require.Nil(t, first)
	require.ErrorIs(t, firstErr, initErr)

	second, secondErr := NewLambdaOptimized()
	require.Nil(t, second)
	require.Error(t, secondErr)
	require.EqualError(t, secondErr, firstErr.Error())
	require.Equal(t, 1, calls, "failed Lambda init must be cached by sync.Once")
}

func TestNewLambdaOptimized_ReadsKMSKeyARNFromEnv_COV6(t *testing.T) {
	t.Cleanup(func() {
		globalLambdaDB = nil
		globalLambdaDBErr = nil
		lambdaOnce = sync.Once{}
	})

	globalLambdaDB = nil
	globalLambdaDBErr = nil
	lambdaOnce = sync.Once{}

	t.Setenv("AWS_LAMBDA_FUNCTION_NAME", "test-function")
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("KMS_KEY_ARN", "arn:aws:kms:us-east-1:111111111111:key/test")

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(nil), nil
	})

	got, err := NewLambdaOptimized()
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, got.db)
	require.NotNil(t, got.db.session)
	require.NotNil(t, got.db.session.Config())
	require.Equal(t, "arn:aws:kms:us-east-1:111111111111:key/test", got.db.session.Config().KMSKeyARN)
}

func TestLambdaDB_WithLambdaTimeout_NoDeadlineReturnsSame_COV6(t *testing.T) {
	ldb := &LambdaDB{db: &DB{}}
	require.Same(t, ldb, ldb.WithLambdaTimeout(context.Background()))
}

func TestLambdaDB_WithLambdaTimeout_SetsHardDeadlineAndEffectiveBuffer_COV6(t *testing.T) {
	httpClient := newCapturingHTTPClient(nil)

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	ldb := &LambdaDB{
		ExtendedDB: db,
		db:         db,
		modelCache: &sync.Map{},
	}

	deadline := time.Now().Add(5 * time.Second)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	t.Cleanup(cancel)

	newDB := ldb.WithLambdaTimeout(ctx)
	require.NotNil(t, newDB)
	require.Same(t, ldb.modelCache, newDB.modelCache)
	require.NotNil(t, newDB.db)
	require.Equal(t, ctx, newDB.db.ctx)
	require.WithinDuration(t, deadline, newDB.db.lambdaDeadline, 25*time.Millisecond)
	require.Equal(t, defaultLambdaDBTimeoutBuffer, newDB.db.lambdaTimeoutBuffer)
}

func TestLambdaDB_WithLambdaTimeoutConfig_PreservesConfiguredBuffer_COV6(t *testing.T) {
	httpClient := newCapturingHTTPClient(nil)

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	db := mustDB(t, dbAny)
	db.metadataCache.Store("cached", "value")

	ldb := &LambdaDB{
		ExtendedDB:     db,
		db:             db,
		modelCache:     &sync.Map{},
		lambdaMemoryMB: 1024,
		isLambda:       true,
		xrayEnabled:    true,
	}
	require.NoError(t, ldb.PreRegisterModels(&cov4LambdaModel{}))

	configured := ldb.WithLambdaTimeoutConfig(LambdaTimeoutConfig{Buffer: 500 * time.Millisecond})
	require.NotNil(t, configured)
	require.NotSame(t, ldb, configured)
	require.NotSame(t, db, configured.db)
	require.Equal(t, 500*time.Millisecond, configured.db.lambdaTimeoutBuffer)
	require.Same(t, ldb.modelCache, configured.modelCache)
	require.True(t, configured.IsModelRegistered(cov4LambdaModel{}))
	require.Same(t, db.session, configured.db.session)
	require.Same(t, db.registry, configured.db.registry)
	require.Same(t, db.converter, configured.db.converter)
	require.Same(t, db.marshaler, configured.db.marshaler)
	require.Equal(t, ldb.lambdaMemoryMB, configured.lambdaMemoryMB)
	require.Equal(t, ldb.isLambda, configured.isLambda)
	require.Equal(t, ldb.xrayEnabled, configured.xrayEnabled)
	cached, ok := configured.db.metadataCache.Load("cached")
	require.True(t, ok)
	require.Equal(t, "value", cached)

	deadline := time.Now().Add(5 * time.Second)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	t.Cleanup(cancel)

	timed := configured.WithLambdaTimeout(ctx)
	require.NotNil(t, timed)
	require.Equal(t, ctx, timed.db.ctx)
	require.Equal(t, 500*time.Millisecond, timed.db.lambdaTimeoutBuffer)
	require.WithinDuration(t, deadline, timed.db.lambdaDeadline, 25*time.Millisecond)
	require.Same(t, configured.modelCache, timed.modelCache)
	require.True(t, timed.IsModelRegistered(cov4LambdaModel{}))
	cached, ok = timed.db.metadataCache.Load("cached")
	require.True(t, ok)
	require.Equal(t, "value", cached)
}

func TestLambdaDB_WithLambdaTimeoutConfig_AppliesConfiguredBufferOnce_COV6(t *testing.T) {
	db := &DB{}
	ldb := &LambdaDB{ExtendedDB: db, db: db, modelCache: &sync.Map{}}
	configured := ldb.WithLambdaTimeoutConfig(LambdaTimeoutConfig{Buffer: 500 * time.Millisecond})

	deadline := time.Now().Add(900 * time.Millisecond)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	t.Cleanup(cancel)

	timed := configured.WithLambdaTimeout(ctx)
	require.NotNil(t, timed)
	require.Equal(t, 500*time.Millisecond, timed.db.lambdaTimeoutBuffer)
	require.NoError(t, (&queryExecutor{db: timed.db}).checkLambdaTimeout())

	soonDeadline := time.Now().Add(450 * time.Millisecond)
	soonCtx, soonCancel := context.WithDeadline(context.Background(), soonDeadline)
	t.Cleanup(soonCancel)

	soonTimed := configured.WithLambdaTimeout(soonCtx)
	require.NotNil(t, soonTimed)
	require.Error(t, (&queryExecutor{db: soonTimed.db}).checkLambdaTimeout())
}

func TestLambdaDB_OptimizeForMemorySynchronizesTimeoutBufferWithLambdaTimeout_COV6(t *testing.T) {
	db := &DB{}
	ldb := &LambdaDB{
		ExtendedDB:     db,
		db:             db,
		modelCache:     &sync.Map{},
		lambdaMemoryMB: 1024,
	}

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(5*time.Second))
	t.Cleanup(cancel)

	const workers = 8
	const iterations = 100

	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < iterations; j++ {
				ldb.OptimizeForMemory()
			}
		}()

		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < iterations; j++ {
				if got := ldb.WithLambdaTimeout(ctx); got == nil {
					t.Errorf("WithLambdaTimeout returned nil")
				}
			}
		}()
	}

	close(start)
	wg.Wait()
}

func TestLambdaDB_OptimizeForMemorySynchronizesTimeoutBufferWithDBTimeout_COV6(t *testing.T) {
	db := &DB{}
	ldb := &LambdaDB{
		ExtendedDB:     db,
		db:             db,
		modelCache:     &sync.Map{},
		lambdaMemoryMB: 1024,
	}

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(5*time.Second))
	t.Cleanup(cancel)

	const workers = 8
	const iterations = 100

	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < iterations; j++ {
				ldb.OptimizeForMemory()
			}
		}()

		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < iterations; j++ {
				gotAny := db.WithLambdaTimeout(ctx)
				got, ok := gotAny.(*DB)
				if !ok || got == nil {
					t.Errorf("WithLambdaTimeout returned %T", gotAny)
				}
			}
		}()
	}

	close(start)
	wg.Wait()
}

func TestLambdaDB_OptimizeForMemorySynchronizesTimeoutBufferWithQueryTimeoutCheck_COV6(t *testing.T) {
	db := &DB{lambdaDeadline: time.Now().Add(30 * time.Second)}
	ldb := &LambdaDB{
		ExtendedDB:     db,
		db:             db,
		modelCache:     &sync.Map{},
		lambdaMemoryMB: 1024,
	}

	const workers = 8
	const iterations = 100

	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < iterations; j++ {
				ldb.OptimizeForMemory()
			}
		}()

		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < iterations; j++ {
				if err := (&queryExecutor{db: db}).checkLambdaTimeout(); err != nil {
					t.Errorf("checkLambdaTimeout returned unexpected error: %v", err)
				}
			}
		}()
	}

	close(start)
	wg.Wait()
}

func TestLambdaDB_WithLambdaTimeoutConfig_NonPositiveBufferUsesDefault_COV6(t *testing.T) {
	db := &DB{}
	ldb := &LambdaDB{ExtendedDB: db, db: db, modelCache: &sync.Map{}}

	configured := ldb.WithLambdaTimeoutConfig(LambdaTimeoutConfig{Buffer: -1})
	require.NotNil(t, configured)
	require.Zero(t, configured.db.lambdaTimeoutBuffer)

	deadline := time.Now().Add(5 * time.Second)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	t.Cleanup(cancel)

	timed := configured.WithLambdaTimeout(ctx)
	require.NotNil(t, timed)
	require.Equal(t, defaultLambdaDBTimeoutBuffer, timed.db.lambdaTimeoutBuffer)
	require.WithinDuration(t, deadline, timed.db.lambdaDeadline, 25*time.Millisecond)
}

func TestGetLambdaMemoryMB_HandlesEmptyAndInvalidValues_COV6(t *testing.T) {
	t.Setenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE", "")
	require.Equal(t, 0, GetLambdaMemoryMB())

	t.Setenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE", "not-a-number")
	require.Equal(t, 0, GetLambdaMemoryMB())

	t.Setenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE", "256")
	require.Equal(t, 256, GetLambdaMemoryMB())
}
