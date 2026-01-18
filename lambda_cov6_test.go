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

	"github.com/theory-cloud/tabletheory/pkg/session"
)

func TestBenchmarkColdStart_CoversSuccessAndErrorPaths_COV6(t *testing.T) {
	origLoad := benchmarkLoadDefaultConfig
	origNewClient := benchmarkNewDynamoDBClient

	t.Cleanup(func() {
		globalLambdaDB = nil
		lambdaOnce = sync.Once{}
		benchmarkLoadDefaultConfig = origLoad
		benchmarkNewDynamoDBClient = origNewClient
	})

	resetGlobals := func() {
		globalLambdaDB = nil
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
		lambdaOnce = sync.Once{}
	})

	globalLambdaDB = &LambdaDB{}
	lambdaOnce = sync.Once{}

	got, err := NewLambdaOptimized()
	require.NoError(t, err)
	require.Same(t, globalLambdaDB, got)
}

func TestLambdaDB_WithLambdaTimeout_NoDeadlineReturnsSame_COV6(t *testing.T) {
	ldb := &LambdaDB{db: &DB{}}
	require.Same(t, ldb, ldb.WithLambdaTimeout(context.Background()))
}

func TestLambdaDB_WithLambdaTimeout_SetsAdjustedDeadline_COV6(t *testing.T) {
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
	require.WithinDuration(t, deadline.Add(-1*time.Second), newDB.db.lambdaDeadline, 25*time.Millisecond)
}

func TestGetLambdaMemoryMB_HandlesEmptyAndInvalidValues_COV6(t *testing.T) {
	t.Setenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE", "")
	require.Equal(t, 0, GetLambdaMemoryMB())

	t.Setenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE", "not-a-number")
	require.Equal(t, 0, GetLambdaMemoryMB())

	t.Setenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE", "256")
	require.Equal(t, 256, GetLambdaMemoryMB())
}
