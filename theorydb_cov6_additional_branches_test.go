package theorydb

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/marshal"
	"github.com/theory-cloud/tabletheory/pkg/model"
	queryPkg "github.com/theory-cloud/tabletheory/pkg/query"
	"github.com/theory-cloud/tabletheory/pkg/session"
	pkgTypes "github.com/theory-cloud/tabletheory/pkg/types"
)

type cov6SelectModel struct {
	ID   string `theorydb:"pk,attr:id"`
	Name string `theorydb:"attr:name"`
}

func (cov6SelectModel) TableName() string { return "cov6_select_models" }

type cov6CtxKey struct{}

func TestQuery_Select_Branches_COV6(t *testing.T) {
	db := newBareDB()
	qAny := db.Model(&cov6SelectModel{}).Select("Name").Select()
	q, ok := qAny.(*queryPkg.Query)
	require.True(t, ok)

	compiled, err := q.Compile()
	require.NoError(t, err)
	require.Empty(t, compiled.ProjectionExpression)
}

func TestDB_ContextHelpers_CopyMetadataCache_And_DefaultLambdaBuffer_COV6(t *testing.T) {
	converter := pkgTypes.NewConverter()
	db := &DB{
		registry:  model.NewRegistry(),
		converter: converter,
		marshaler: marshal.New(converter),
		ctx:       context.Background(),
	}

	db.Model(&cov6SelectModel{})

	typ := reflect.TypeOf(&cov6SelectModel{})
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	_, ok := db.metadataCache.Load(typ)
	require.True(t, ok, "expected model metadata to be cached")

	ctxDBAny := db.WithContext(context.WithValue(context.Background(), cov6CtxKey{}, "v"))
	ctxDB, ok := ctxDBAny.(*DB)
	require.True(t, ok)
	_, ok = ctxDB.metadataCache.Load(typ)
	require.True(t, ok)

	bufferedAny := db.WithLambdaTimeoutBuffer(123 * time.Millisecond)
	bufferedDB, ok := bufferedAny.(*DB)
	require.True(t, ok)
	_, ok = bufferedDB.metadataCache.Load(typ)
	require.True(t, ok)

	deadline := time.Now().Add(5 * time.Second)
	deadlineCtx, cancel := context.WithDeadline(context.Background(), deadline)
	t.Cleanup(cancel)

	lambdaAny := db.WithLambdaTimeout(deadlineCtx)
	lambdaDB, ok := lambdaAny.(*DB)
	require.True(t, ok)
	require.Equal(t, deadline.Add(-500*time.Millisecond), lambdaDB.lambdaDeadline)
	_, ok = lambdaDB.metadataCache.Load(typ)
	require.True(t, ok)
}

type cov6RetryModel struct {
	ID string `theorydb:"pk,attr:id"`
}

func (cov6RetryModel) TableName() string { return "cov6_retry_models" }

func TestQuery_AllWithRetry_RetriesOnEmptyResults_COV6(t *testing.T) {
	httpClient := newCapturingHTTPClient(nil)
	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	var out []cov6RetryModel
	err = db.Model(&cov6RetryModel{}).WithRetry(1, 0).All(&out)
	require.NoError(t, err)
	require.Empty(t, out)
	require.Equal(t, 2, countRequestsByTarget(httpClient.Requests(), "DynamoDB_20120810.Scan"))
}

func TestQuery_AllWithRetry_RetriesOnErrors_COV6(t *testing.T) {
	httpClient := newCapturingHTTPClient(nil)
	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	db.lambdaDeadline = time.Now().Add(-time.Second)

	var out []cov6RetryModel
	err = db.Model(&cov6RetryModel{}).WithRetry(1, 0).All(&out)
	require.Error(t, err)
	require.Equal(t, 0, countRequestsByTarget(httpClient.Requests(), "DynamoDB_20120810.Scan"))
}
