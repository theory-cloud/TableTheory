package schema

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/model"
	"github.com/theory-cloud/tabletheory/pkg/session"
)

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
		"DynamoDB_20120810.BatchWriteItem": {{body: `{"UnprocessedItems":{}}`}},
		"DynamoDB_20120810.PutItem":        {{body: `{}`}},
		"DynamoDB_20120810.Scan":           {{body: `{"Items":[],"Count":0,"ScannedCount":0}`}},
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
	c.requests = append(c.requests, capturedRequest{Target: target, Payload: payload})
	callIndex := c.callCount[target]
	c.callCount[target] = callIndex + 1

	respSeq := c.responses[target]
	var stub stubbedResponse
	switch {
	case len(respSeq) == 0:
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

func (c *capturingHTTPClient) Requests() []capturedRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]capturedRequest, len(c.requests))
	copy(out, c.requests)
	return out
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

func newTestManager(t *testing.T, httpClient aws.HTTPClient) *Manager {
	t.Helper()

	sess, err := session.NewSession(&session.Config{
		Region:              "us-east-1",
		CredentialsProvider: credentials.NewStaticCredentialsProvider("test", "secret", "token"),
		AWSConfigOptions: []func(*config.LoadOptions) error{
			config.WithHTTPClient(httpClient),
			config.WithRetryer(func() aws.Retryer { return aws.NopRetryer{} }),
		},
	})
	require.NoError(t, err)

	return NewManager(sess, model.NewRegistry())
}

func TestManager_copyTableData_ScansAndWrites(t *testing.T) {
	httpClient := newCapturingHTTPClient(nil)
	httpClient.SetResponseSequence("DynamoDB_20120810.Scan", []stubbedResponse{
		{body: `{"Items":[{"id":{"S":"1"}},{"id":{"S":"2"}}],"Count":2,"ScannedCount":2,"LastEvaluatedKey":{"id":{"S":"2"}}}`},
		{body: `{"Items":[{"id":{"S":"3"}}],"Count":1,"ScannedCount":1}`},
	})

	mgr := newTestManager(t, httpClient)

	err := mgr.copyTableData(context.Background(), "source", "target", 2)
	require.NoError(t, err)

	reqs := httpClient.Requests()
	require.Equal(t, 2, countRequestsByTarget(reqs, "DynamoDB_20120810.Scan"))
	require.GreaterOrEqual(t, countRequestsByTarget(reqs, "DynamoDB_20120810.BatchWriteItem"), 2)
}

func TestWriteRequestsBatched_FallsBackToPutItem(t *testing.T) {
	httpClient := newCapturingHTTPClient(nil)
	httpClient.SetResponseSequence("DynamoDB_20120810.BatchWriteItem", []stubbedResponse{
		{body: `{"UnprocessedItems":{"target":[{"PutRequest":{"Item":{"id":{"S":"1"}}}}]}}`},
	})

	mgr := newTestManager(t, httpClient)
	client, err := mgr.session.Client()
	require.NoError(t, err)

	reqs := []types.WriteRequest{
		{PutRequest: &types.PutRequest{Item: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: "1"},
		}}},
	}

	require.NoError(t, writeRequestsBatched(context.Background(), client, "target", reqs, 25, 1))

	got := httpClient.Requests()
	require.Equal(t, 1, countRequestsByTarget(got, "DynamoDB_20120810.BatchWriteItem"))
	require.Equal(t, 1, countRequestsByTarget(got, "DynamoDB_20120810.PutItem"))
}

func TestBuildPutWriteRequestsWithTransform(t *testing.T) {
	items := []map[string]types.AttributeValue{
		{"id": &types.AttributeValueMemberS{Value: "1"}},
	}

	pk := &model.FieldMetadata{Name: "ID", DBName: "id"}
	meta := &model.Metadata{
		PrimaryKey: &model.KeySchema{PartitionKey: pk},
	}

	reqs, err := buildPutWriteRequestsWithTransform(items, nil, meta, meta)
	require.NoError(t, err)
	require.Len(t, reqs, 1)

	_, err = buildPutWriteRequestsWithTransform(items, func(map[string]types.AttributeValue) (map[string]types.AttributeValue, error) {
		return map[string]types.AttributeValue{}, nil
	}, meta, meta)
	require.Error(t, err)
}

func TestManager_UpdateTable_And_BatchUpdateTable(t *testing.T) {
	httpClient := newCapturingHTTPClient(map[string]string{
		"DynamoDB_20120810.DescribeTable": `{"Table":{"TableName":"tbl","TableStatus":"ACTIVE","BillingModeSummary":{"BillingMode":"PAY_PER_REQUEST"}}}`,
		"DynamoDB_20120810.UpdateTable":   `{}`,
	})

	mgr := newTestManager(t, httpClient)

	type testItem struct {
		ID string `theorydb:"pk"`
	}

	require.NoError(t, mgr.registry.Register(&testItem{}))

	require.NoError(t, mgr.UpdateTable(&testItem{}, WithThroughput(5, 5)))
	require.NoError(t, mgr.BatchUpdateTable(&testItem{}, []TableOption{WithBillingMode(types.BillingModePayPerRequest)}))

	reqs := httpClient.Requests()
	require.GreaterOrEqual(t, countRequestsByTarget(reqs, "DynamoDB_20120810.DescribeTable"), 2)
	require.Equal(t, 2, countRequestsByTarget(reqs, "DynamoDB_20120810.UpdateTable"))
}
