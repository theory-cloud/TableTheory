package theorydb

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/session"
)

type cov4LambdaModel struct {
	ID string `theorydb:"pk,attr:id"`
}

func (cov4LambdaModel) TableName() string { return "cov4_lambda_models" }

type cov4LambdaConverter struct{}

func (cov4LambdaConverter) ToAttributeValue(value any) (types.AttributeValue, error) {
	_ = value
	return &types.AttributeValueMemberS{Value: "x"}, nil
}

func (cov4LambdaConverter) FromAttributeValue(av types.AttributeValue, target any) error {
	_ = av
	_ = target
	return nil
}

func TestLambdaDB_RegistrationAndOptimizers_COV4(t *testing.T) {
	httpClient := newCapturingHTTPClient(map[string]string{
		"DynamoDB_20120810.ListTables": `{"TableNames":[]}`,
	})

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	ldb := &LambdaDB{
		ExtendedDB:     db,
		db:             db,
		modelCache:     &sync.Map{},
		lambdaMemoryMB: 2048,
		isLambda:       true,
	}

	require.False(t, ldb.IsModelRegistered(cov4LambdaModel{}))
	require.NoError(t, ldb.PreRegisterModels(&cov4LambdaModel{}))
	require.True(t, ldb.IsModelRegistered(cov4LambdaModel{}))

	require.Error(t, (*LambdaDB)(nil).RegisterTypeConverter(reflect.TypeOf(""), cov4LambdaConverter{}))
	require.NoError(t, ldb.RegisterTypeConverter(reflect.TypeOf(""), cov4LambdaConverter{}))
	require.True(t, db.converter.HasCustomConverter(reflect.TypeOf("")))

	ldb.OptimizeForMemory()
	require.Equal(t, 50*time.Millisecond, db.lambdaTimeoutBuffer)

	ldb.lambdaMemoryMB = 1024
	ldb.OptimizeForMemory()
	require.Equal(t, 100*time.Millisecond, db.lambdaTimeoutBuffer)

	ldb.lambdaMemoryMB = 0
	ldb.OptimizeForMemory()
	require.Equal(t, 200*time.Millisecond, db.lambdaTimeoutBuffer)

	stats := ldb.GetMemoryStats()
	require.Equal(t, 0, stats.LambdaMemoryMB)

	ldb.lambdaMemoryMB = 512
	stats = ldb.GetMemoryStats()
	require.Equal(t, 512, stats.LambdaMemoryMB)
	require.GreaterOrEqual(t, stats.MemoryPercent, 0.0)

	ldb.OptimizeForColdStart()
	require.Eventually(t, func() bool {
		reqs := httpClient.Requests()
		return countRequestsByTarget(reqs, "DynamoDB_20120810.ListTables") > 0
	}, 500*time.Millisecond, 10*time.Millisecond)
}

func TestLambdaInitAndMetricsString_COV4(t *testing.T) {
	httpClient := newCapturingHTTPClient(map[string]string{
		"DynamoDB_20120810.ListTables": `{"TableNames":[]}`,
	})

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	globalLambdaDB = nil
	lambdaOnce = sync.Once{}
	t.Cleanup(func() {
		globalLambdaDB = nil
		lambdaOnce = sync.Once{}
	})

	db, err := LambdaInit(&cov4LambdaModel{})
	require.NoError(t, err)
	require.NotNil(t, db)

	metrics := ColdStartMetrics{
		TotalDuration: 10 * time.Millisecond,
		MemoryMB:      256,
		IsLambda:      true,
		Phases: map[string]time.Duration{
			"z": 2 * time.Millisecond,
			"a": 1 * time.Millisecond,
		},
	}
	out := metrics.String()
	require.Contains(t, out, "Cold Start Metrics")
	require.Less(t, strings.Index(out, "a:"), strings.Index(out, "z:"))
}
