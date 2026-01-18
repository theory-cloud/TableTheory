package theorydb

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/model"
	queryPkg "github.com/theory-cloud/tabletheory/pkg/query"
	"github.com/theory-cloud/tabletheory/pkg/session"
)

type cov4RootItem struct {
	ID   string `theorydb:"pk,attr:id"`
	Name string `theorydb:"attr:name"`
}

func (cov4RootItem) TableName() string { return "cov4_items" }

type cov4CtxKey struct{}

type cov4MarshalModel struct {
	ID        string    `theorydb:"pk,attr:id"`
	Name      string    `theorydb:"attr:name,index:byName,pk"`
	CreatedAt time.Time `theorydb:"created_at"`
	UpdatedAt time.Time `theorydb:"updated_at"`
	Optional  string    `theorydb:"omitempty,attr:optional"`
	Tags      []string  `theorydb:"set,attr:tags"`
	Version   int64     `theorydb:"version,attr:version"`
	ExpiresAt int64     `theorydb:"ttl,attr:ttl"`
}

func (cov4MarshalModel) TableName() string { return "cov4_marshal_models" }

func TestDB_ContextAndTimeoutHelpers_COV4(t *testing.T) {
	httpClient := newCapturingHTTPClient(nil)

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	ctx := context.WithValue(context.Background(), cov4CtxKey{}, "v")
	ctxDBAny := db.WithContext(ctx)
	ctxDB := mustDB(t, ctxDBAny)
	require.NotSame(t, db, ctxDB)
	require.Equal(t, ctx, ctxDB.ctx)

	bufferedAny := ctxDB.WithLambdaTimeoutBuffer(50 * time.Millisecond)
	buffered := mustDB(t, bufferedAny)
	require.NotSame(t, ctxDB, buffered)
	require.Equal(t, 50*time.Millisecond, buffered.lambdaTimeoutBuffer)

	require.Same(t, buffered, mustDB(t, buffered.WithLambdaTimeout(context.Background())))

	deadlineCtx, cancel := context.WithDeadline(context.Background(), time.Now().Add(10*time.Millisecond))
	t.Cleanup(cancel)
	lambdaAny := buffered.WithLambdaTimeout(deadlineCtx)
	lambdaDB := mustDB(t, lambdaAny)
	require.NotSame(t, buffered, lambdaDB)
	require.False(t, lambdaDB.lambdaDeadline.IsZero())

	executor := &queryExecutor{db: lambdaDB}
	require.Error(t, executor.checkLambdaTimeout())
}

func TestQuery_GetItemWithProjectionAndConsistency_COV4(t *testing.T) {
	httpClient := newCapturingHTTPClient(map[string]string{
		"DynamoDB_20120810.GetItem": `{"Item":{"id":{"S":"u1"},"name":{"S":"alice"}}}`,
	})

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	var out cov4RootItem
	err = db.Model(&cov4RootItem{}).
		Select("Name").
		ConsistentRead().
		Where("ID", "=", "u1").
		First(&out)
	require.NoError(t, err)
	require.Equal(t, cov4RootItem{ID: "u1", Name: "alice"}, out)

	req := findRequestByTarget(httpClient.Requests(), "DynamoDB_20120810.GetItem")
	require.NotNil(t, req)
	require.Equal(t, true, req.Payload["ConsistentRead"])
	require.Contains(t, req.Payload, "ProjectionExpression")
}

func TestQuery_ConditionExpressionMerging_COV4(t *testing.T) {
	httpClient := newCapturingHTTPClient(nil)

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	require.NoError(t, db.Model(&cov4RootItem{ID: "u1", Name: "alice"}).
		WithConditionExpression("name = :raw", map[string]any{":raw": "alice"}).
		Create())

	putReq := findRequestByTarget(httpClient.Requests(), "DynamoDB_20120810.PutItem")
	require.NotNil(t, putReq)
	require.Equal(t, "name = :raw", putReq.Payload["ConditionExpression"])

	values, ok := putReq.Payload["ExpressionAttributeValues"].(map[string]any)
	require.True(t, ok)
	require.Contains(t, values, ":raw")

	require.NoError(t, db.Model(&cov4RootItem{ID: "u2", Name: "alice"}).
		IfExists().
		WithConditionExpression("name = :raw", map[string]any{":raw": "alice"}).
		Create())

	putReq = findRequestByTarget(httpClient.Requests(), "DynamoDB_20120810.PutItem")
	require.NotNil(t, putReq)
	condExpr, ok := putReq.Payload["ConditionExpression"].(string)
	require.True(t, ok)
	require.Contains(t, condExpr, ") AND (")

	values, ok = putReq.Payload["ExpressionAttributeValues"].(map[string]any)
	require.True(t, ok)
	require.Contains(t, values, ":raw")

	err = db.Model(&cov4RootItem{ID: "u3", Name: "alice"}).
		IfExists().
		WithConditionExpression("name = :raw", map[string]any{":raw": "alice"}).
		WithConditionExpression("other = :raw", map[string]any{":raw": "bob"}).
		Create()
	require.ErrorContains(t, err, "duplicate placeholder :raw")
}

func TestQuery_AllPaginated_QueryAndScanPaths_COV4(t *testing.T) {
	httpClient := newCapturingHTTPClient(map[string]string{
		"DynamoDB_20120810.Query": `{"Items":[{"id":{"S":"u1"},"name":{"S":"alice"}}],"Count":1,"ScannedCount":1,"LastEvaluatedKey":{"id":{"S":"u1"}}}`,
		"DynamoDB_20120810.Scan":  `{"Items":[{"id":{"S":"u2"},"name":{"S":"bob"}}],"Count":1,"ScannedCount":1}`,
	})

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	var out []cov4RootItem
	paginated, err := db.Model(&cov4RootItem{}).
		WithContext(context.Background()).
		Index("by-id").
		OrderBy("ID", "DESC").
		Limit(5).
		Where("ID", "=", "u1").
		AllPaginated(&out)
	require.NoError(t, err)
	require.True(t, paginated.HasMore)
	require.NotEmpty(t, paginated.NextCursor)
	require.Len(t, out, 1)
	require.Equal(t, cov4RootItem{ID: "u1", Name: "alice"}, out[0])

	var scanOut []cov4RootItem
	paginated, err = db.Model(&cov4RootItem{}).
		Index("by-id").
		Limit(5).
		Where("ID", "<", "u9").
		Where("Name", "=", "bob").
		AllPaginated(&scanOut)
	require.NoError(t, err)
	require.False(t, paginated.HasMore)
	require.Empty(t, paginated.NextCursor)
	require.Len(t, scanOut, 1)
	require.Equal(t, cov4RootItem{ID: "u2", Name: "bob"}, scanOut[0])

	reqs := httpClient.Requests()
	queryReq := findRequestByTarget(reqs, "DynamoDB_20120810.Query")
	require.NotNil(t, queryReq)
	require.Equal(t, "by-id", queryReq.Payload["IndexName"])
	require.Equal(t, false, queryReq.Payload["ScanIndexForward"])
	require.Equal(t, float64(5), queryReq.Payload["Limit"])

	scanReq := findRequestByTarget(reqs, "DynamoDB_20120810.Scan")
	require.NotNil(t, scanReq)
	require.Equal(t, "by-id", scanReq.Payload["IndexName"])
	require.Equal(t, float64(5), scanReq.Payload["Limit"])

	cursor, err := queryPkg.EncodeCursor(map[string]types.AttributeValue{
		"id": &types.AttributeValueMemberS{Value: "u1"},
	}, "", "")
	require.NoError(t, err)

	var cursorOut []cov4RootItem
	require.NoError(t, db.Model(&cov4RootItem{}).
		Cursor(cursor).
		Limit(1).
		Where("ID", "=", "u1").
		All(&cursorOut))

	cursorReq := findRequestByTarget(httpClient.Requests(), "DynamoDB_20120810.Query")
	require.NotNil(t, cursorReq)
	exclusiveStartKey, ok := cursorReq.Payload["ExclusiveStartKey"].(map[string]any)
	require.True(t, ok)
	idAttr, ok := exclusiveStartKey["id"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "u1", idAttr["S"])
}

type scanSegmentHTTPClient struct {
	requests []capturedRequest
	mu       sync.Mutex
}

func (c *scanSegmentHTTPClient) Do(req *http.Request) (*http.Response, error) {
	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	if err := req.Body.Close(); err != nil {
		return nil, err
	}

	payload := make(map[string]any)
	if len(bodyBytes) > 0 {
		if err := json.Unmarshal(bodyBytes, &payload); err != nil {
			return nil, err
		}
	}

	target := req.Header.Get("X-Amz-Target")

	c.mu.Lock()
	c.requests = append(c.requests, capturedRequest{Target: target, Payload: payload})
	c.mu.Unlock()

	if target != "DynamoDB_20120810.Scan" {
		return &http.Response{
			StatusCode:    http.StatusOK,
			Status:        fmt.Sprintf("%d %s", http.StatusOK, http.StatusText(http.StatusOK)),
			Header:        make(http.Header),
			ContentLength: int64(len("{}")),
			Body:          io.NopCloser(bytes.NewReader([]byte("{}"))),
			Request:       req,
		}, nil
	}

	segment := 0
	if raw, ok := payload["Segment"]; ok {
		if f, ok := raw.(float64); ok {
			segment = int(f)
		}
	}

	respBody := fmt.Sprintf(`{"Items":[{"id":{"S":"seg%d"},"name":{"S":"name%d"}}],"Count":1,"ScannedCount":1}`, segment, segment)
	resp := &http.Response{
		StatusCode:    http.StatusOK,
		Status:        fmt.Sprintf("%d %s", http.StatusOK, http.StatusText(http.StatusOK)),
		Header:        make(http.Header),
		ContentLength: int64(len(respBody)),
		Body:          io.NopCloser(bytes.NewReader([]byte(respBody))),
		Request:       req,
	}
	resp.Header.Set("Content-Type", "application/x-amz-json-1.0")
	return resp, nil
}

func TestQuery_ScanAllSegments_COV4(t *testing.T) {
	httpClient := &scanSegmentHTTPClient{}

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	var out []cov4RootItem
	qAny := db.Model(&cov4RootItem{}).
		WithContext(context.Background()).
		Index("by-id").
		Limit(9).
		Where("ID", "=", "u1").
		Where("Name", "=", "x").
		ParallelScan(0, 3)

	require.ErrorContains(t, qAny.ScanAllSegments(cov4RootItem{}, 3), "destination must be a pointer to slice")

	require.NoError(t, qAny.ScanAllSegments(&out, 3))
	require.Len(t, out, 3)

	gotIDs := make([]string, 0, len(out))
	for _, item := range out {
		gotIDs = append(gotIDs, item.ID)
	}
	sort.Strings(gotIDs)
	require.Equal(t, []string{"seg0", "seg1", "seg2"}, gotIDs)
}

func TestQuery_UpdateBuilderAndExecutor_COV4(t *testing.T) {
	httpClient := newCapturingHTTPClient(map[string]string{
		"DynamoDB_20120810.UpdateItem": `{"Attributes":{"id":{"S":"u1"},"name":{"S":"after"},"version":{"N":"1"}}}`,
	})

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	builder := db.Model(&cov4MarshalModel{}).Where("ID", "=", "u1").UpdateBuilder()
	require.NotNil(t, builder)

	var got cov4MarshalModel
	err = builder.
		Set("Name", "after").
		ConditionVersion(0).
		ReturnValues("ALL_NEW").
		ExecuteWithResult(&got)
	require.NoError(t, err)
	require.Equal(t, "u1", got.ID)
	require.Equal(t, "after", got.Name)
	require.Equal(t, int64(1), got.Version)

	adapter := &metadataAdapter{metadata: mustGetMetadata(t, db, &cov4MarshalModel{})}
	require.NotEmpty(t, adapter.Indexes())
	require.Equal(t, "version", adapter.VersionFieldName())

	badQ := &errorQuery{err: errors.New("boom")}
	errBuilder := badQ.UpdateBuilder()
	require.NotNil(t, errBuilder)
	require.Error(t, errBuilder.Set("x", "y").Execute())
	require.Error(t, errBuilder.ExecuteWithResult(&got))
}

func mustGetMetadata(t *testing.T, db *DB, model any) *model.Metadata {
	t.Helper()
	meta, err := db.registry.GetMetadata(model)
	require.NoError(t, err)
	return meta
}

func TestQuery_MarshalReflect_COV4(t *testing.T) {
	httpClient := newCapturingHTTPClient(map[string]string{
		"DynamoDB_20120810.PutItem": `{}`,
	})

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	db.marshaler = nil

	item := &cov4MarshalModel{
		ID:        "u1",
		Version:   0,
		ExpiresAt: 123,
		Tags:      []string{"a", "b"},
	}

	require.NoError(t, db.Model(item).Create())
	require.False(t, item.CreatedAt.IsZero())
	require.False(t, item.UpdatedAt.IsZero())

	req := findRequestByTarget(httpClient.Requests(), "DynamoDB_20120810.PutItem")
	require.NotNil(t, req)
	require.Contains(t, req.Payload, "Item")

	itemMap, ok := req.Payload["Item"].(map[string]any)
	require.True(t, ok)
	require.NotContains(t, itemMap, "optional")
}
