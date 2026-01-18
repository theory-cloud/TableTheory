package theorydb

import (
	"context"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/model"
	queryPkg "github.com/theory-cloud/tabletheory/pkg/query"
	"github.com/theory-cloud/tabletheory/pkg/session"
)

type batchWriteRoundTripper struct {
	mu    sync.Mutex
	calls int
}

func (rt *batchWriteRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.mu.Lock()
	rt.calls++
	call := rt.calls
	rt.mu.Unlock()

	if req.Header.Get("X-Amz-Target") != "DynamoDB_20120810.BatchWriteItem" {
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Status:     "400 Bad Request",
			Header:     http.Header{"Content-Type": []string{"application/x-amz-json-1.0"}},
			Body:       io.NopCloser(strings.NewReader(`{"__type":"UnknownOperationException","message":"unexpected operation"}`)),
			Request:    req,
		}, nil
	}

	body := `{"UnprocessedItems": {}}`
	if call == 1 {
		body = `{"UnprocessedItems":{"TestTable":[{"PutRequest":{"Item":{"id":{"S":"x"},"sk":{"S":"y"}}}}]}}`
	}

	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     http.Header{"Content-Type": []string{"application/x-amz-json-1.0"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}, nil
}

func TestQueryExecutor_ExecuteBatchWrite_UnprocessedLoop_COV6(t *testing.T) {
	rt := &batchWriteRoundTripper{}
	httpClient := &http.Client{Transport: rt}

	sess, err := session.NewSession(&session.Config{
		Region:              "us-east-1",
		Endpoint:            "http://example.com",
		CredentialsProvider: credentials.NewStaticCredentialsProvider("AKID", "SECRET", ""),
		DynamoDBOptions: []func(*dynamodb.Options){
			func(o *dynamodb.Options) {
				o.HTTPClient = httpClient
			},
		},
	})
	require.NoError(t, err)

	db := &DB{
		ctx:     context.Background(),
		session: sess,
	}
	qe := &queryExecutor{db: db}

	items := make([]map[string]types.AttributeValue, 26)
	for i := range items {
		items[i] = map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: "id"},
			"sk": &types.AttributeValueMemberS{Value: "sk"},
		}
	}

	err = qe.ExecuteBatchWrite(&queryPkg.CompiledBatchWrite{
		TableName: "TestTable",
		Items:     items,
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, rt.calls, 3)
}

func TestQueryExecutor_ExecuteBatchWriteItem_Branches_COV6(t *testing.T) {
	t.Run("EmptyRequests", func(t *testing.T) {
		qe := &queryExecutor{db: &DB{}}
		result, err := qe.ExecuteBatchWriteItem("TestTable", nil)
		require.NoError(t, err)
		require.NotNil(t, result)
	})

	t.Run("TooManyRequests", func(t *testing.T) {
		qe := &queryExecutor{db: &DB{}}
		tooMany := make([]types.WriteRequest, 26)
		_, err := qe.ExecuteBatchWriteItem("TestTable", tooMany)
		require.Error(t, err)
		require.Contains(t, err.Error(), "maximum 25 items")
	})

	t.Run("APIFailure", func(t *testing.T) {
		rt := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			body := `{"__type":"com.amazonaws.dynamodb.v20120810#ValidationException","message":"bad request"}`
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Status:     "400 Bad Request",
				Header:     http.Header{"Content-Type": []string{"application/x-amz-json-1.0"}},
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    req,
			}, nil
		})

		sess, err := session.NewSession(&session.Config{
			Region:              "us-east-1",
			Endpoint:            "http://example.com",
			CredentialsProvider: credentials.NewStaticCredentialsProvider("AKID", "SECRET", ""),
			DynamoDBOptions: []func(*dynamodb.Options){
				func(o *dynamodb.Options) {
					o.HTTPClient = &http.Client{Transport: rt}
				},
			},
		})
		require.NoError(t, err)

		qe := &queryExecutor{db: &DB{ctx: context.Background(), session: sess}}
		_, err = qe.ExecuteBatchWriteItem("TestTable", []types.WriteRequest{
			{PutRequest: &types.PutRequest{Item: map[string]types.AttributeValue{"id": &types.AttributeValueMemberS{Value: "1"}}}},
		})
		require.Error(t, err)
	})

	t.Run("EncryptedMetadata_SkipsEmptyPutRequests", func(t *testing.T) {
		rt := &batchWriteRoundTripper{}
		httpClient := &http.Client{Transport: rt}

		sess, err := session.NewSession(&session.Config{
			Region:              "us-east-1",
			Endpoint:            "http://example.com",
			CredentialsProvider: credentials.NewStaticCredentialsProvider("AKID", "SECRET", ""),
			KMSKeyARN:           "arn:aws:kms:us-east-1:000000000000:key/test",
			DynamoDBOptions: []func(*dynamodb.Options){
				func(o *dynamodb.Options) {
					o.HTTPClient = httpClient
				},
			},
		})
		require.NoError(t, err)

		meta := &model.Metadata{
			Type: reflect.TypeOf(struct{ Secret string }{}),
			Fields: map[string]*model.FieldMetadata{
				"Secret": {IsEncrypted: true},
			},
		}

		qe := &queryExecutor{
			db:       &DB{ctx: context.Background(), session: sess},
			metadata: meta,
		}

		_, err = qe.ExecuteBatchWriteItem("TestTable", []types.WriteRequest{
			{DeleteRequest: &types.DeleteRequest{Key: map[string]types.AttributeValue{"id": &types.AttributeValueMemberS{Value: "1"}}}},
			{PutRequest: &types.PutRequest{Item: map[string]types.AttributeValue{}}},
		})
		require.NoError(t, err)
	})
}

func TestCryptoFloat64_Range_COV6(t *testing.T) {
	val, err := cryptoFloat64()
	require.NoError(t, err)
	require.GreaterOrEqual(t, val, 0.0)
	require.Less(t, val, 1.0)
}

func TestCalculateBatchRetryDelay_COV6(t *testing.T) {
	require.Equal(t, time.Duration(0), calculateBatchRetryDelay(nil, 0))

	policy := &core.RetryPolicy{
		InitialDelay:  0,
		BackoffFactor: 2,
		MaxDelay:      0,
		Jitter:        0,
	}
	require.Equal(t, 50*time.Millisecond, calculateBatchRetryDelay(policy, 0))

	policy.InitialDelay = 100 * time.Millisecond
	policy.MaxDelay = 250 * time.Millisecond
	require.Equal(t, 100*time.Millisecond, calculateBatchRetryDelay(policy, 0))
	require.Equal(t, 200*time.Millisecond, calculateBatchRetryDelay(policy, 1))
	require.Equal(t, 250*time.Millisecond, calculateBatchRetryDelay(policy, 2))
}

func TestCalculateBatchRetryDelay_JitterRange_COV6(t *testing.T) {
	policy := &core.RetryPolicy{
		InitialDelay:  100 * time.Millisecond,
		BackoffFactor: 2,
		Jitter:        0.1,
	}

	delay := calculateBatchRetryDelay(policy, 0)
	require.GreaterOrEqual(t, delay, 85*time.Millisecond)
	require.LessOrEqual(t, delay, 115*time.Millisecond)
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (rt roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return rt(req)
}
