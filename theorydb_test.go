package theorydb

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	_ "unsafe"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/core"
	customerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/marshal"
	"github.com/theory-cloud/tabletheory/pkg/model"
	queryPkg "github.com/theory-cloud/tabletheory/pkg/query"
	"github.com/theory-cloud/tabletheory/pkg/session"
	pkgTypes "github.com/theory-cloud/tabletheory/pkg/types"
)

//go:linkname sessionConfigLoadFunc github.com/theory-cloud/tabletheory/pkg/session.configLoadFunc
var sessionConfigLoadFunc func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error)

func stubSessionConfigLoad(t *testing.T, fn func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error)) {
	t.Helper()

	original := sessionConfigLoadFunc
	sessionConfigLoadFunc = fn

	t.Cleanup(func() {
		sessionConfigLoadFunc = original
	})
}

func minimalAWSConfig(httpClient aws.HTTPClient) aws.Config {
	cfg := aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider("test", "secret", "token"),
		Retryer: func() aws.Retryer {
			return aws.NopRetryer{}
		},
	}

	if httpClient != nil {
		cfg.HTTPClient = httpClient
	} else {
		cfg.HTTPClient = &http.Client{}
	}

	return cfg
}

func mustDB(t *testing.T, dbAny any) *DB {
	t.Helper()
	db, ok := dbAny.(*DB)
	require.True(t, ok, "expected *theorydb.DB, got %T", dbAny)
	return db
}

type capturedRequest struct {
	Payload map[string]any
	Target  string
}

type stubbedResponse struct {
	err     error
	headers map[string]string
	body    string
	status  int
}

type capturingHTTPClient struct {
	responses map[string][]stubbedResponse
	callCount map[string]int
	requests  []capturedRequest
	mu        sync.Mutex
}

func newCapturingHTTPClient(responses map[string]string) *capturingHTTPClient {
	defaults := map[string][]stubbedResponse{
		"DynamoDB_20120810.Query":   {{body: `{"Items":[],"Count":0,"ScannedCount":0}`}},
		"DynamoDB_20120810.Scan":    {{body: `{"Items":[],"Count":0,"ScannedCount":0}`}},
		"DynamoDB_20120810.GetItem": {{body: `{"Item":null}`}},
	}

	for k, v := range responses {
		defaults[k] = []stubbedResponse{{body: v}}
	}

	return &capturingHTTPClient{
		responses: defaults,
		callCount: make(map[string]int),
	}
}

func (c *capturingHTTPClient) Do(req *http.Request) (*http.Response, error) {
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
	c.requests = append(c.requests, capturedRequest{
		Target:  target,
		Payload: payload,
	})

	callIndex := c.callCount[target]
	c.callCount[target] = callIndex + 1

	respSeq, ok := c.responses[target]
	var stub stubbedResponse
	switch {
	case !ok || len(respSeq) == 0:
		stub = stubbedResponse{}
	case callIndex < len(respSeq):
		stub = respSeq[callIndex]
	default:
		stub = respSeq[len(respSeq)-1]
	}
	c.mu.Unlock()

	if stub.err != nil {
		return nil, stub.err
	}

	status := stub.status
	if status == 0 {
		status = http.StatusOK
	}

	body := stub.body
	if body == "" {
		body = "{}"
	}

	resp := &http.Response{
		StatusCode:    status,
		Status:        fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:        make(http.Header),
		ContentLength: int64(len(body)),
		Body:          io.NopCloser(bytes.NewReader([]byte(body))),
		Request:       req,
	}

	resp.Header.Set("Content-Type", "application/x-amz-json-1.0")
	for k, v := range stub.headers {
		resp.Header.Set(k, v)
	}

	return resp, nil
}

func (c *capturingHTTPClient) SetResponseSequence(target string, sequence []stubbedResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.responses[target] = append([]stubbedResponse(nil), sequence...)
	delete(c.callCount, target)
}

func (c *capturingHTTPClient) AppendResponse(target string, resp stubbedResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.responses[target] = append(c.responses[target], resp)
}

func (c *capturingHTTPClient) Requests() []capturedRequest {
	c.mu.Lock()
	defer c.mu.Unlock()

	out := make([]capturedRequest, len(c.requests))
	copy(out, c.requests)
	return out
}

func findRequestByTarget(reqs []capturedRequest, target string) *capturedRequest {
	for i := len(reqs) - 1; i >= 0; i-- {
		if reqs[i].Target == target {
			return &reqs[i]
		}
	}
	return nil
}

func countRequestsByTarget(reqs []capturedRequest, target string) int {
	count := 0
	for _, r := range reqs {
		if r.Target == target {
			count++
		}
	}
	return count
}

type testOrderModel struct {
	TenantID  string `theorydb:"pk,attr:tenantId"`
	CreatedAt string `theorydb:"sk,attr:createdAt"`
	Status    string `theorydb:"attr:status"`
}

type auditOrderModel struct {
	CreatedAt time.Time `theorydb:"created_at,attr:createdAt"`
	UpdatedAt time.Time `theorydb:"updated_at,attr:updatedAt"`
	TenantID  string    `theorydb:"pk,attr:tenantId"`
	OrderID   string    `theorydb:"sk,attr:orderId"`
	Status    string    `theorydb:"attr:status"`
}

func (auditOrderModel) TableName() string {
	return "orders_test"
}

func (testOrderModel) TableName() string {
	return "orders_test"
}

func newBareDB() *DB {
	converter := pkgTypes.NewConverter()
	return &DB{
		session:   &session.Session{},
		registry:  model.NewRegistry(),
		converter: converter,
		marshaler: marshal.New(converter),
		ctx:       context.Background(),
	}
}

func TestNewAndNewBasicSuccess(t *testing.T) {
	client := newCapturingHTTPClient(nil)
	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(client), nil
	})

	cfg := session.Config{Region: "us-east-1"}

	extended, err := New(cfg)
	require.NoError(t, err)
	require.NotNil(t, extended)

	db, ok := extended.(*DB)
	require.True(t, ok)
	require.NotNil(t, db.session)
	require.NotNil(t, db.registry)
	require.NotNil(t, db.converter)
	require.NotNil(t, db.marshaler)
	require.NotNil(t, db.ctx)

	basic, err := NewBasic(cfg)
	require.NoError(t, err)
	require.NotNil(t, basic)
}

func TestNewErrorWhenSessionFails(t *testing.T) {
	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return aws.Config{}, errors.New("forced failure")
	})

	_, err := New(session.Config{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to create session")

	_, err = NewBasic(session.Config{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to create session")
}

func TestDBModelCachesMetadata(t *testing.T) {
	db := newBareDB()

	q := db.Model(&testOrderModel{})
	_, ok := q.(*queryPkg.Query)
	require.True(t, ok)

	typ := reflect.TypeOf(testOrderModel{})
	metaValue, ok := db.metadataCache.Load(typ)
	require.True(t, ok)
	cachedMeta, isMeta := metaValue.(*model.Metadata)
	require.True(t, isMeta)

	meta, err := db.registry.GetMetadata(&testOrderModel{})
	require.NoError(t, err)
	require.Equal(t, meta, cachedMeta)
}

func TestDBModelRegistrationFailure(t *testing.T) {
	db := newBareDB()

	q := db.Model(123)
	require.IsType(t, &errorQuery{}, q)

	err := q.First(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to register model int")
}

func TestQueryBuilderAllBuildsExpressions(t *testing.T) {
	client := newCapturingHTTPClient(nil)
	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(client), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)

	db := mustDB(t, dbAny)

	q := db.Model(&testOrderModel{}).
		Where("TenantID", "=", "tenant#123").
		Where("CreatedAt", "=", "2024-03-01T00:00:00Z").
		Filter("Status", "=", "ACTIVE").
		OrderBy("CreatedAt", "DESC").
		Limit(5)

	internalQuery, ok := q.(*queryPkg.Query)
	require.True(t, ok)

	compiled, err := internalQuery.Compile()
	require.NoError(t, err)
	require.NotEmpty(t, compiled.FilterExpression)
	resolvedFilter := compiled.FilterExpression
	for placeholder, actual := range compiled.ExpressionAttributeNames {
		resolvedFilter = strings.ReplaceAll(resolvedFilter, placeholder, actual)
	}
	require.Contains(t, strings.ToLower(resolvedFilter), "status")
	require.NotEmpty(t, compiled.ExpressionAttributeValues)

	var results []testOrderModel
	err = q.All(&results)
	require.NoError(t, err)
	require.Empty(t, results)

	reqs := client.Requests()
	require.NotEmpty(t, reqs)

	var queryReq capturedRequest
	for _, r := range reqs {
		if r.Target == "DynamoDB_20120810.Query" {
			queryReq = r
			break
		}
	}
	require.NotNil(t, queryReq.Payload)
	require.Equal(t, "orders_test", queryReq.Payload["TableName"])

	limitRaw, ok := queryReq.Payload["Limit"]
	require.True(t, ok)
	require.Equal(t, float64(5), limitRaw)

	scanForward, ok := queryReq.Payload["ScanIndexForward"]
	require.True(t, ok)
	require.Equal(t, false, scanForward)

	keyExpr, ok := queryReq.Payload["KeyConditionExpression"].(string)
	require.True(t, ok)
	require.NotEmpty(t, keyExpr)

	filterExpr, ok := queryReq.Payload["FilterExpression"].(string)
	require.True(t, ok)
	require.NotEmpty(t, filterExpr)

	namesMapRaw, ok := queryReq.Payload["ExpressionAttributeNames"].(map[string]any)
	require.True(t, ok)

	valuesMapRaw, ok := queryReq.Payload["ExpressionAttributeValues"].(map[string]any)
	require.True(t, ok)

	meta, err := db.registry.GetMetadata(&testOrderModel{})
	require.NoError(t, err)

	var resolvedKeyExpr = keyExpr
	for placeholder, actual := range namesMapRaw {
		name, isString := actual.(string)
		require.True(t, isString)
		resolvedKeyExpr = strings.ReplaceAll(resolvedKeyExpr, placeholder, name)
	}
	require.Contains(t, resolvedKeyExpr, meta.PrimaryKey.PartitionKey.DBName)
	require.Contains(t, resolvedKeyExpr, meta.PrimaryKey.SortKey.DBName)

	resolvedFilterExpr := filterExpr
	for placeholder, actual := range namesMapRaw {
		name, isString := actual.(string)
		require.True(t, isString)
		resolvedFilterExpr = strings.ReplaceAll(resolvedFilterExpr, placeholder, name)
	}
	require.Contains(t, strings.ToLower(resolvedFilterExpr), "status")

	var sawTenantValue bool
	var sawStatusValue bool
	for _, v := range valuesMapRaw {
		if attr, ok := v.(map[string]any); ok {
			s, ok := attr["S"].(string)
			if !ok {
				continue
			}
			if s == "tenant#123" || s == "2024-03-01T00:00:00Z" {
				sawTenantValue = true
			}
			if s == "ACTIVE" {
				sawStatusValue = true
			}
		}
	}
	require.True(t, sawTenantValue)
	require.True(t, sawStatusValue)
}

func TestQueryBuilderFirstBuildsGetItemRequest(t *testing.T) {
	client := newCapturingHTTPClient(nil)
	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(client), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)

	db := mustDB(t, dbAny)

	q := db.Model(&testOrderModel{}).
		Where("TenantID", "=", "tenant#999").
		Where("CreatedAt", "=", "2024-03-02T10:00:00Z")

	var single testOrderModel
	err = q.First(&single)
	require.ErrorIs(t, err, customerrors.ErrItemNotFound)

	reqs := client.Requests()
	require.NotEmpty(t, reqs)

	var getReq capturedRequest
	for _, r := range reqs {
		if r.Target == "DynamoDB_20120810.GetItem" {
			getReq = r
			break
		}
	}
	require.NotNil(t, getReq.Payload)

	keyRaw, ok := getReq.Payload["Key"].(map[string]any)
	require.True(t, ok)

	meta, err := db.registry.GetMetadata(&testOrderModel{})
	require.NoError(t, err)

	pkValue, ok := keyRaw[meta.PrimaryKey.PartitionKey.DBName].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "tenant#999", pkValue["S"])

	skValue, ok := keyRaw[meta.PrimaryKey.SortKey.DBName].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "2024-03-02T10:00:00Z", skValue["S"])
}

func TestQueryCreatePopulatesTimestampsAndCondition(t *testing.T) {
	client := newCapturingHTTPClient(nil)
	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(client), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	order := &auditOrderModel{
		TenantID: "tenant#create",
		OrderID:  "order#1",
		Status:   "PENDING",
	}

	start := time.Now()
	err = db.Model(order).Create()
	require.NoError(t, err)

	require.False(t, order.CreatedAt.IsZero())
	require.False(t, order.UpdatedAt.IsZero())
	require.True(t, order.CreatedAt.Equal(order.UpdatedAt))
	require.False(t, order.CreatedAt.Before(start))
	require.Equal(t, "PENDING", order.Status)

	req := findRequestByTarget(client.Requests(), "DynamoDB_20120810.PutItem")
	require.NotNil(t, req)

	payload := req.Payload

	_, hasCond := payload["ConditionExpression"]
	require.False(t, hasCond, "Create should not add conditions unless requested")

	item, ok := payload["Item"].(map[string]any)
	require.True(t, ok)

	createdAttr, ok := item["createdAt"].(map[string]any)
	require.True(t, ok)
	createdStr, ok := createdAttr["S"].(string)
	require.True(t, ok)
	_, err = time.Parse(time.RFC3339Nano, createdStr)
	require.NoError(t, err)

	updatedAttr, ok := item["updatedAt"].(map[string]any)
	require.True(t, ok)
	updatedStr, ok := updatedAttr["S"].(string)
	require.True(t, ok)
	_, err = time.Parse(time.RFC3339Nano, updatedStr)
	require.NoError(t, err)

	// IfNotExists should add the guard when explicitly requested
	guarded := &auditOrderModel{
		TenantID: "tenant#guard",
		OrderID:  "order#guard",
		Status:   "PENDING",
	}
	err = db.Model(guarded).IfNotExists().Create()
	require.NoError(t, err)

	req = findRequestByTarget(client.Requests(), "DynamoDB_20120810.PutItem")
	require.NotNil(t, req)
	payload = req.Payload

	condExpr, hasCond := payload["ConditionExpression"].(string)
	require.True(t, hasCond, "IfNotExists should add a conditional guard")
	require.Contains(t, condExpr, "attribute_not_exists")

	namesMap, hasNames := payload["ExpressionAttributeNames"].(map[string]any)
	require.True(t, hasNames)
	foundTenant := false
	for _, rawName := range namesMap {
		if name, ok := rawName.(string); ok && name == "tenantId" {
			foundTenant = true
		}
	}
	require.True(t, foundTenant, "guard should reference the partition key attribute")
}

func TestQueryCreateConditionalFailure(t *testing.T) {
	client := newCapturingHTTPClient(nil)
	client.SetResponseSequence("DynamoDB_20120810.PutItem", []stubbedResponse{
		{
			status: http.StatusBadRequest,
			body:   `{"__type":"com.amazonaws.dynamodb.v20120810#ConditionalCheckFailedException","message":"Conditional request failed"}`,
			headers: map[string]string{
				"x-amzn-errortype": "ConditionalCheckFailedException",
			},
		},
	})
	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(client), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	order := &auditOrderModel{
		TenantID: "tenant#create",
		OrderID:  "order#exists",
		Status:   "PENDING",
	}

	err = db.Model(order).Create()
	require.Error(t, err)
	require.True(t, errors.Is(err, customerrors.ErrConditionFailed))
	require.Contains(t, err.Error(), "already exists")
	require.True(t, order.CreatedAt.IsZero())
	require.True(t, order.UpdatedAt.IsZero())
}

func TestCreateOrUpdateAllowsOverwrite(t *testing.T) {
	client := newCapturingHTTPClient(nil)
	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(client), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	order := &auditOrderModel{
		TenantID: "tenant#upsert",
		OrderID:  "order#1",
		Status:   "PROCESSING",
	}

	err = db.Model(order).CreateOrUpdate()
	require.NoError(t, err)

	req := findRequestByTarget(client.Requests(), "DynamoDB_20120810.PutItem")
	require.NotNil(t, req)

	_, hasCond := req.Payload["ConditionExpression"]
	require.False(t, hasCond)

	item, ok := req.Payload["Item"].(map[string]any)
	require.True(t, ok)

	createdAttr, ok := item["createdAt"].(map[string]any)
	require.True(t, ok)
	createdStr, isString := createdAttr["S"].(string)
	require.True(t, isString)
	_, err = time.Parse(time.RFC3339Nano, createdStr)
	require.NoError(t, err)

	updatedAttr, ok := item["updatedAt"].(map[string]any)
	require.True(t, ok)
	updatedStr, isString := updatedAttr["S"].(string)
	require.True(t, isString)
	_, err = time.Parse(time.RFC3339Nano, updatedStr)
	require.NoError(t, err)

	require.False(t, order.CreatedAt.IsZero())
	require.False(t, order.UpdatedAt.IsZero())
	require.True(t, order.CreatedAt.Equal(order.UpdatedAt))
	require.Equal(t, "PROCESSING", order.Status)
}

func TestQueryUpdateBuildsExpression(t *testing.T) {
	client := newCapturingHTTPClient(nil)
	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(client), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	order := &auditOrderModel{
		TenantID: "tenant#update",
		OrderID:  "order#1",
		Status:   "SHIPPED",
	}

	err = db.Model(order).
		Where("TenantID", "=", order.TenantID).
		Where("OrderID", "=", order.OrderID).
		Update("Status")
	require.NoError(t, err)
	require.Equal(t, "SHIPPED", order.Status)

	req := findRequestByTarget(client.Requests(), "DynamoDB_20120810.UpdateItem")
	require.NotNil(t, req)

	payload := req.Payload

	updateExpr, ok := payload["UpdateExpression"].(string)
	require.True(t, ok)
	require.Contains(t, strings.ToUpper(updateExpr), "SET")

	key, ok := payload["Key"].(map[string]any)
	require.True(t, ok)
	require.NotEmpty(t, key)

	names, ok := payload["ExpressionAttributeNames"].(map[string]any)
	require.True(t, ok)

	var sawStatusName bool
	var sawUpdatedAtName bool
	for _, raw := range names {
		if name, valid := raw.(string); valid {
			if name == "status" {
				sawStatusName = true
			}
			if name == "updatedAt" {
				sawUpdatedAtName = true
			}
		}
	}
	require.True(t, sawStatusName)
	require.True(t, sawUpdatedAtName)

	values, ok := payload["ExpressionAttributeValues"].(map[string]any)
	require.True(t, ok)
	require.GreaterOrEqual(t, len(values), 2)

	var sawStatusValue bool
	var sawUpdatedAtValue bool
	for _, raw := range values {
		attr, isMap := raw.(map[string]any)
		require.True(t, isMap)
		if s, exists := attr["S"].(string); exists {
			switch {
			case s == "SHIPPED":
				sawStatusValue = true
			case isRFC3339(s):
				sawUpdatedAtValue = true
			}
		}
	}
	require.True(t, sawStatusValue)
	require.True(t, sawUpdatedAtValue)
}

func TestQueryUpdateConditionalFailure(t *testing.T) {
	client := newCapturingHTTPClient(nil)
	client.SetResponseSequence("DynamoDB_20120810.UpdateItem", []stubbedResponse{
		{
			status: http.StatusBadRequest,
			body:   `{"__type":"com.amazonaws.dynamodb.v20120810#ConditionalCheckFailedException","message":"Conditional request failed"}`,
			headers: map[string]string{
				"x-amzn-errortype": "ConditionalCheckFailedException",
			},
		},
	})
	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(client), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	order := &auditOrderModel{
		TenantID: "tenant#update",
		OrderID:  "order#concurrency",
		Status:   "SHIPPED",
	}

	err = db.Model(order).
		Where("TenantID", "=", order.TenantID).
		Where("OrderID", "=", order.OrderID).
		Update("Status")
	require.Error(t, err)
	require.True(t, errors.Is(err, customerrors.ErrConditionFailed))
}

func TestQueryDeleteBuildsKey(t *testing.T) {
	client := newCapturingHTTPClient(nil)
	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(client), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	order := &auditOrderModel{
		TenantID: "tenant#delete",
		OrderID:  "order#1",
	}

	err = db.Model(order).
		Where("TenantID", "=", order.TenantID).
		Where("OrderID", "=", order.OrderID).
		Delete()
	require.NoError(t, err)

	req := findRequestByTarget(client.Requests(), "DynamoDB_20120810.DeleteItem")
	require.NotNil(t, req)

	key, ok := req.Payload["Key"].(map[string]any)
	require.True(t, ok)
	require.Len(t, key, 2)

	pk, pkOK := key["tenantId"].(map[string]any)
	require.True(t, pkOK)
	require.Equal(t, order.TenantID, pk["S"])

	sk, skOK := key["orderId"].(map[string]any)
	require.True(t, skOK)
	require.Equal(t, order.OrderID, sk["S"])
}

func TestQueryDeleteConditionalFailure(t *testing.T) {
	client := newCapturingHTTPClient(nil)
	client.SetResponseSequence("DynamoDB_20120810.DeleteItem", []stubbedResponse{
		{
			status: http.StatusBadRequest,
			body:   `{"__type":"com.amazonaws.dynamodb.v20120810#ConditionalCheckFailedException","message":"Conditional request failed"}`,
			headers: map[string]string{
				"x-amzn-errortype": "ConditionalCheckFailedException",
			},
		},
	})
	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(client), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	order := &auditOrderModel{
		TenantID: "tenant#delete",
		OrderID:  "order#lock",
	}

	err = db.Model(order).
		Where("TenantID", "=", order.TenantID).
		Where("OrderID", "=", order.OrderID).
		Delete()
	require.Error(t, err)
	require.True(t, errors.Is(err, customerrors.ErrConditionFailed))
}

func TestQueryAllAggregatesAcrossPages(t *testing.T) {
	page1 := `{
		"Items": [
			{
				"tenantId": {"S": "tenant#multi"},
				"createdAt": {"S": "2024-03-01T00:00:00Z"},
				"status": {"S": "PAID"}
			}
		],
		"Count": 1,
		"ScannedCount": 1,
		"LastEvaluatedKey": {
			"tenantId": {"S": "tenant#multi"},
			"createdAt": {"S": "2024-03-01T00:00:00Z"}
		}
	}`
	page2 := `{
		"Items": [
			{
				"tenantId": {"S": "tenant#multi"},
				"createdAt": {"S": "2024-03-02T00:00:00Z"},
				"status": {"S": "SHIPPED"}
			}
		],
		"Count": 1,
		"ScannedCount": 1
	}`

	client := newCapturingHTTPClient(nil)
	client.SetResponseSequence("DynamoDB_20120810.Query", []stubbedResponse{
		{body: page1},
		{body: page2},
	})

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(client), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	var results []testOrderModel
	err = db.Model(&testOrderModel{}).
		Where("TenantID", "=", "tenant#multi").
		FilterGroup(func(q core.Query) {
			q.Filter("Status", "=", "PAID")
			q.OrFilter("Status", "=", "SHIPPED")
		}).
		All(&results)
	require.NoError(t, err)
	require.Len(t, results, 2)

	reqs := client.Requests()
	require.Equal(t, 2, countRequestsByTarget(reqs, "DynamoDB_20120810.Query"))

	firstReq := findRequestByTarget(reqs, "DynamoDB_20120810.Query")
	require.NotNil(t, firstReq)
	filterExpr, ok := firstReq.Payload["FilterExpression"].(string)
	require.True(t, ok)
	require.Contains(t, strings.ToLower(filterExpr), "status")
	require.Contains(t, strings.ToLower(filterExpr), "or")

	require.Equal(t, "PAID", results[0].Status)
	require.Equal(t, "SHIPPED", results[1].Status)
}

func TestQueryAllRespectsLimit(t *testing.T) {
	page1 := `{
		"Items": [
			{
				"tenantId": {"S": "tenant#limit"},
				"createdAt": {"S": "2024-03-01T00:00:00Z"},
				"status": {"S": "PAID"}
			}
		],
		"Count": 1,
		"ScannedCount": 1,
		"LastEvaluatedKey": {
			"tenantId": {"S": "tenant#limit"},
			"createdAt": {"S": "2024-03-01T00:00:00Z"}
		}
	}`
	page2 := `{
		"Items": [
			{
				"tenantId": {"S": "tenant#limit"},
				"createdAt": {"S": "2024-03-02T00:00:00Z"},
				"status": {"S": "SHIPPED"}
			}
		],
		"Count": 1,
		"ScannedCount": 1
	}`

	client := newCapturingHTTPClient(nil)
	client.SetResponseSequence("DynamoDB_20120810.Query", []stubbedResponse{
		{body: page1},
		{body: page2},
	})

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(client), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	var results []testOrderModel
	err = db.Model(&testOrderModel{}).
		Where("TenantID", "=", "tenant#limit").
		Limit(1).
		All(&results)
	require.NoError(t, err)
	require.Len(t, results, 1)

	reqs := client.Requests()
	require.Equal(t, 1, countRequestsByTarget(reqs, "DynamoDB_20120810.Query"))
}

func TestScanAllAppliesFilters(t *testing.T) {
	scanResp := `{
		"Items": [
			{
				"tenantId": {"S": "tenant#scan"},
				"createdAt": {"S": "2024-03-05T00:00:00Z"},
				"status": {"S": "ARCHIVED"}
			},
			{
				"tenantId": {"S": "tenant#scan"},
				"createdAt": {"S": "2024-03-06T00:00:00Z"},
				"status": {"S": "ARCHIVED"}
			}
		],
		"Count": 2,
		"ScannedCount": 2
	}`

	client := newCapturingHTTPClient(nil)
	client.SetResponseSequence("DynamoDB_20120810.Scan", []stubbedResponse{{body: scanResp}})

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(client), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	var results []testOrderModel
	err = db.Model(&testOrderModel{}).
		Filter("Status", "=", "ARCHIVED").
		Filter("TenantID", "=", "tenant#scan").
		All(&results)
	require.NoError(t, err)
	require.Len(t, results, 2)

	reqs := client.Requests()
	require.Equal(t, 1, countRequestsByTarget(reqs, "DynamoDB_20120810.Scan"))

	scanReq := findRequestByTarget(reqs, "DynamoDB_20120810.Scan")
	require.NotNil(t, scanReq)
	filterExpr, ok := scanReq.Payload["FilterExpression"].(string)
	require.True(t, ok)
	require.Contains(t, strings.ToLower(filterExpr), "status")
}

func TestQueryCountAggregatesPages(t *testing.T) {
	page1 := `{
		"Count": 2,
		"ScannedCount": 2,
		"LastEvaluatedKey": {
			"tenantId": {"S": "tenant#count"},
			"createdAt": {"S": "2024-03-01T00:00:00Z"}
		}
	}`
	page2 := `{"Count":1,"ScannedCount":1}`

	client := newCapturingHTTPClient(nil)
	client.SetResponseSequence("DynamoDB_20120810.Query", []stubbedResponse{
		{body: page1},
		{body: page2},
	})

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(client), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	count, err := db.Model(&testOrderModel{}).
		Where("TenantID", "=", "tenant#count").
		Count()
	require.NoError(t, err)
	require.Equal(t, int64(3), count)

	reqs := client.Requests()
	require.Equal(t, 2, countRequestsByTarget(reqs, "DynamoDB_20120810.Query"))

	firstReq := findRequestByTarget(reqs, "DynamoDB_20120810.Query")
	require.NotNil(t, firstReq)
	selectValue, ok := firstReq.Payload["Select"].(string)
	require.True(t, ok)
	require.Equal(t, "COUNT", selectValue)
}

func TestScanCountReturnsTotal(t *testing.T) {
	client := newCapturingHTTPClient(nil)
	client.SetResponseSequence("DynamoDB_20120810.Scan", []stubbedResponse{
		{body: `{"Count":5,"ScannedCount":5}`},
	})

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(client), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	count, err := db.Model(&testOrderModel{}).Count()
	require.NoError(t, err)
	require.Equal(t, int64(5), count)

	reqs := client.Requests()
	require.Equal(t, 1, countRequestsByTarget(reqs, "DynamoDB_20120810.Scan"))

	scanReq := findRequestByTarget(reqs, "DynamoDB_20120810.Scan")
	require.NotNil(t, scanReq)
	selectValue, hasSelect := scanReq.Payload["Select"].(string)
	require.True(t, hasSelect)
	require.Equal(t, "COUNT", selectValue)
}

func TestQueryFirstReturnsItem(t *testing.T) {
	getResp := `{
		"Item": {
			"tenantId": {"S": "tenant#first"},
			"createdAt": {"S": "2024-03-01T00:00:00Z"},
			"status": {"S": "PAID"}
		}
	}`

	client := newCapturingHTTPClient(nil)
	client.SetResponseSequence("DynamoDB_20120810.GetItem", []stubbedResponse{{body: getResp}})

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(client), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	var result testOrderModel
	err = db.Model(&testOrderModel{}).
		Where("TenantID", "=", "tenant#first").
		Where("CreatedAt", "=", "2024-03-01T00:00:00Z").
		First(&result)
	require.NoError(t, err)
	require.Equal(t, "tenant#first", result.TenantID)
	require.Equal(t, "2024-03-01T00:00:00Z", result.CreatedAt)
	require.Equal(t, "PAID", result.Status)

	reqs := client.Requests()
	require.Equal(t, 1, countRequestsByTarget(reqs, "DynamoDB_20120810.GetItem"))
}

func isRFC3339(value string) bool {
	_, err := time.Parse(time.RFC3339Nano, value)
	return err == nil
}
