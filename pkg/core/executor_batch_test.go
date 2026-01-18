package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testContextKey is a custom type for context keys to avoid collisions
type testContextKey string

type httpStubResponse struct {
	err      error
	validate func(*http.Request)
	target   string
	body     string
	status   int
}

type httpClientStub struct {
	t         *testing.T
	responses []httpStubResponse
	mu        sync.Mutex
}

func newHTTPClientStub(t *testing.T, responses []httpStubResponse) *httpClientStub {
	t.Helper()
	return &httpClientStub{t: t, responses: responses}
}

func (c *httpClientStub) Do(req *http.Request) (*http.Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	require.NotEmpty(c.t, c.responses, "unexpected request to %s", req.Header.Get("X-Amz-Target"))

	resp := c.responses[0]
	c.responses = c.responses[1:]

	if resp.target != "" {
		require.Equal(c.t, resp.target, req.Header.Get("X-Amz-Target"))
	}

	if resp.validate != nil {
		resp.validate(req)
	}

	if resp.err != nil {
		return nil, resp.err
	}

	status := resp.status
	if status == 0 {
		status = http.StatusOK
	}

	body := resp.body
	if body == "" {
		body = "{}"
	}

	return &http.Response{
		StatusCode:    status,
		Body:          io.NopCloser(strings.NewReader(body)),
		Header:        make(http.Header),
		ContentLength: int64(len(body)),
		Request:       req,
	}, nil
}

func (c *httpClientStub) AssertDrained(t *testing.T) {
	t.Helper()

	c.mu.Lock()
	defer c.mu.Unlock()

	require.Empty(t, c.responses, "expected all stub responses to be consumed")
}

func newStubbedDynamoClient(t *testing.T, stub *httpClientStub) *dynamodb.Client {
	t.Helper()

	cfg := aws.Config{
		Region:      "us-west-2",
		Credentials: aws.AnonymousCredentials{},
		HTTPClient:  stub,
		Retryer: func() aws.Retryer {
			return aws.NopRetryer{}
		},
	}

	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String("https://dynamodb.stub.local")
	})
}

func readRequestJSON(t *testing.T, req *http.Request) map[string]any {
	t.Helper()

	body, err := io.ReadAll(req.Body)
	require.NoError(t, err)

	require.NotEmpty(t, body, "expected request body to be populated")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))

	// Restore body so subsequent validations (if any) can re-read it.
	req.Body = io.NopCloser(bytes.NewReader(body))

	return payload
}

func marshalJSON(t *testing.T, v any) string {
	t.Helper()

	data, err := json.Marshal(v)
	require.NoError(t, err)

	return string(data)
}

func requireMapStringAny(t *testing.T, v any) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	require.True(t, ok, "expected map[string]any, got %T", v)
	return m
}

func requireSliceAny(t *testing.T, v any) []any {
	t.Helper()
	s, ok := v.([]any)
	require.True(t, ok, "expected []any, got %T", v)
	return s
}

func TestNewBatchWriteExecutor(t *testing.T) {
	stub := newHTTPClientStub(t, nil)
	client := newStubbedDynamoClient(t, stub)
	ctx := context.WithValue(context.Background(), testContextKey("test"), "marker")

	executor := NewBatchWriteExecutor(client, ctx)
	require.NotNil(t, executor)
	assert.Equal(t, client, executor.client)
	assert.Equal(t, ctx, executor.ctx)
}

func TestExecuteBatchWriteItem(t *testing.T) {
	ctx := context.Background()

	t.Run("empty requests", func(t *testing.T) {
		stub := newHTTPClientStub(t, nil)
		executor := NewBatchWriteExecutor(newStubbedDynamoClient(t, stub), ctx)

		result, err := executor.ExecuteBatchWriteItem("tbl", nil)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Empty(t, result.UnprocessedItems)
		assert.Empty(t, result.ConsumedCapacity)

		stub.AssertDrained(t)
	})

	t.Run("too many requests", func(t *testing.T) {
		stub := newHTTPClientStub(t, nil)
		executor := NewBatchWriteExecutor(newStubbedDynamoClient(t, stub), ctx)

		var requests []types.WriteRequest
		for i := 0; i < 26; i++ {
			requests = append(requests, types.WriteRequest{
				PutRequest: &types.PutRequest{
					Item: map[string]types.AttributeValue{
						"id": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", i)},
					},
				},
			})
		}

		_, err := executor.ExecuteBatchWriteItem("tbl", requests)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "supports maximum 25 items")
	})

	t.Run("success with consumed capacity", func(t *testing.T) {
		response := map[string]any{
			"ConsumedCapacity": []map[string]any{
				{
					"TableName":     "tbl",
					"CapacityUnits": 3.5,
				},
			},
		}

		stub := newHTTPClientStub(t, []httpStubResponse{
			{
				target: "DynamoDB_20120810.BatchWriteItem",
				body:   marshalJSON(t, response),
				validate: func(req *http.Request) {
					payload := readRequestJSON(t, req)
					assert.Equal(t, "TOTAL", payload["ReturnConsumedCapacity"])

					requestItems := requireMapStringAny(t, payload["RequestItems"])
					assert.Len(t, requestItems, 1)
					items := requireSliceAny(t, requestItems["tbl"])
					require.Len(t, items, 2)

					item0Request := requireMapStringAny(t, items[0])
					putRequest := requireMapStringAny(t, item0Request["PutRequest"])
					item0 := requireMapStringAny(t, putRequest["Item"])
					assert.Equal(t, "alpha", requireMapStringAny(t, item0["id"])["S"])

					item1Request := requireMapStringAny(t, items[1])
					deleteRequest := requireMapStringAny(t, item1Request["DeleteRequest"])
					item1 := requireMapStringAny(t, deleteRequest["Key"])
					assert.Equal(t, "bravo", requireMapStringAny(t, item1["pk"])["S"])
				},
			},
		})

		client := newStubbedDynamoClient(t, stub)
		executor := NewBatchWriteExecutor(client, ctx)

		putItem := types.WriteRequest{
			PutRequest: &types.PutRequest{
				Item: map[string]types.AttributeValue{
					"id": &types.AttributeValueMemberS{Value: "alpha"},
				},
			},
		}
		deleteItem := types.WriteRequest{
			DeleteRequest: &types.DeleteRequest{
				Key: map[string]types.AttributeValue{
					"pk": &types.AttributeValueMemberS{Value: "bravo"},
				},
			},
		}

		requests := []types.WriteRequest{putItem, deleteItem}
		before := append([]types.WriteRequest(nil), requests...)

		result, err := executor.ExecuteBatchWriteItem("tbl", requests)
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Len(t, result.ConsumedCapacity, 1)
		assert.Equal(t, "tbl", aws.ToString(result.ConsumedCapacity[0].TableName))
		assert.Equal(t, 3.5, aws.ToFloat64(result.ConsumedCapacity[0].CapacityUnits))
		assert.Empty(t, result.UnprocessedItems)
		assert.Equal(t, before, requests, "input slice should remain unchanged")

		stub.AssertDrained(t)
	})

	t.Run("handles unprocessed items", func(t *testing.T) {
		unprocessed := map[string]any{
			"tbl": []map[string]any{
				{
					"DeleteRequest": map[string]any{
						"Key": map[string]any{
							"pk": map[string]any{"S": "unprocessed"},
						},
					},
				},
			},
		}

		stub := newHTTPClientStub(t, []httpStubResponse{
			{
				target: "DynamoDB_20120810.BatchWriteItem",
				body: marshalJSON(t, map[string]any{
					"UnprocessedItems": unprocessed,
				}),
			},
		})

		executor := NewBatchWriteExecutor(newStubbedDynamoClient(t, stub), ctx)

		requests := []types.WriteRequest{
			{
				DeleteRequest: &types.DeleteRequest{
					Key: map[string]types.AttributeValue{
						"pk": &types.AttributeValueMemberS{Value: "value"},
					},
				},
			},
		}

		result, err := executor.ExecuteBatchWriteItem("tbl", requests)
		require.NoError(t, err)
		require.NotNil(t, result)

		require.Contains(t, result.UnprocessedItems, "tbl")
		require.Len(t, result.UnprocessedItems["tbl"], 1)
		pk, ok := result.UnprocessedItems["tbl"][0].DeleteRequest.Key["pk"].(*types.AttributeValueMemberS)
		require.True(t, ok)
		assert.Equal(t, "unprocessed", pk.Value)

		stub.AssertDrained(t)
	})

	t.Run("wraps client errors", func(t *testing.T) {
		stub := newHTTPClientStub(t, []httpStubResponse{
			{
				target: "DynamoDB_20120810.BatchWriteItem",
				err:    errors.New("network down"),
			},
		})

		executor := NewBatchWriteExecutor(newStubbedDynamoClient(t, stub), ctx)

		requests := []types.WriteRequest{
			{
				PutRequest: &types.PutRequest{
					Item: map[string]types.AttributeValue{
						"id": &types.AttributeValueMemberS{Value: "alpha"},
					},
				},
			},
		}

		_, err := executor.ExecuteBatchWriteItem("tbl", requests)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "batch write failed")
		assert.Contains(t, err.Error(), "network down")
	})
}

func TestBatchWriteExecutor_ExecuteQueryAndScan_ReturnErrors_COV6(t *testing.T) {
	executor := &BatchWriteExecutor{}

	err := executor.ExecuteQuery(&CompiledQuery{Operation: "Query"}, &struct{}{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not support ExecuteQuery")

	err = executor.ExecuteScan(&CompiledQuery{Operation: "Scan"}, &struct{}{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not support ExecuteScan")
}

func TestBatchDeleteWithResult(t *testing.T) {
	ctx := context.Background()

	t.Run("no keys", func(t *testing.T) {
		stub := newHTTPClientStub(t, nil)
		executor := NewBatchWriteExecutor(newStubbedDynamoClient(t, stub), ctx)

		result, err := executor.BatchDeleteWithResult("tbl", nil)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, 0, result.Succeeded)
		assert.Equal(t, 0, result.Failed)
		assert.Empty(t, result.Errors)
		assert.Empty(t, result.UnprocessedKeys)

		stub.AssertDrained(t)
	})

	t.Run("splits batches and tracks unprocessed", func(t *testing.T) {
		totalKeys := 30
		keys := make([]map[string]types.AttributeValue, 0, totalKeys)
		for i := 0; i < totalKeys; i++ {
			keys = append(keys, map[string]types.AttributeValue{
				"pk": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", i)},
			})
		}

		original := make([]map[string]types.AttributeValue, len(keys))
		for i := range keys {
			original[i] = map[string]types.AttributeValue{
				"pk": keys[i]["pk"],
			}
		}

		stub := newHTTPClientStub(t, []httpStubResponse{
			{
				target: "DynamoDB_20120810.BatchWriteItem",
				body: marshalJSON(t, map[string]any{
					"UnprocessedItems": map[string]any{
						"tbl": []map[string]any{
							{
								"DeleteRequest": map[string]any{
									"Key": map[string]any{
										"pk": map[string]any{"N": "3"},
									},
								},
							},
						},
					},
				}),
				validate: func(req *http.Request) {
					payload := readRequestJSON(t, req)
					assert.Equal(t, "TOTAL", payload["ReturnConsumedCapacity"])
					requestItems := requireMapStringAny(t, payload["RequestItems"])
					items := requireSliceAny(t, requestItems["tbl"])
					assert.Len(t, items, 25)
				},
			},
			{
				target: "DynamoDB_20120810.BatchWriteItem",
				body:   marshalJSON(t, map[string]any{}),
				validate: func(req *http.Request) {
					payload := readRequestJSON(t, req)
					requestItems := requireMapStringAny(t, payload["RequestItems"])
					items := requireSliceAny(t, requestItems["tbl"])
					assert.Len(t, items, totalKeys-25)
				},
			},
		})

		executor := NewBatchWriteExecutor(newStubbedDynamoClient(t, stub), ctx)

		result, err := executor.BatchDeleteWithResult("tbl", keys)
		require.NoError(t, err)

		assert.Equal(t, totalKeys-1, result.Succeeded)
		assert.Equal(t, 1, result.Failed)
		require.Len(t, result.UnprocessedKeys, 1)
		pk, ok := result.UnprocessedKeys[0]["pk"].(*types.AttributeValueMemberN)
		require.True(t, ok)
		assert.Equal(t, "3", pk.Value)
		assert.Empty(t, result.Errors)

		assert.Len(t, keys, len(original))
		for i := range keys {
			assert.Equal(t, original[i], keys[i], "input keys should not be mutated")
		}

		stub.AssertDrained(t)
	})

	t.Run("captures errors per batch", func(t *testing.T) {
		stub := newHTTPClientStub(t, []httpStubResponse{
			{
				target: "DynamoDB_20120810.BatchWriteItem",
				err:    errors.New("boom"),
			},
		})

		executor := NewBatchWriteExecutor(newStubbedDynamoClient(t, stub), ctx)

		keys := make([]map[string]types.AttributeValue, 3)
		for i := range keys {
			keys[i] = map[string]types.AttributeValue{
				"pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("id-%d", i)},
			}
		}

		result, err := executor.BatchDeleteWithResult("tbl", keys)
		require.NoError(t, err)

		assert.Equal(t, 0, result.Succeeded)
		assert.Equal(t, len(keys), result.Failed)
		require.Len(t, result.Errors, 1)
		assert.Contains(t, result.Errors[0].Error(), "batch write failed")

		stub.AssertDrained(t)
	})
}

func TestNewExecutorWithBatchSupport(t *testing.T) {
	stub := newHTTPClientStub(t, nil)
	client := newStubbedDynamoClient(t, stub)
	ctx := context.WithValue(context.Background(), testContextKey("test"), "ctx-marker")

	executor := NewExecutorWithBatchSupport(client, ctx)
	require.NotNil(t, executor)
	require.NotNil(t, executor.UpdateExecutor)
	require.NotNil(t, executor.BatchWriteExecutor)

	assert.Equal(t, client, executor.deleteClient)
	assert.Equal(t, ctx, executor.BatchWriteExecutor.ctx)
	assert.Equal(t, client, executor.UpdateExecutor.client)

	stub.AssertDrained(t)
}

func TestExecuteDeleteItem(t *testing.T) {
	type contextKey struct{}
	ctxKey := contextKey{}
	ctxValue := "context-value"
	ctx := context.WithValue(context.Background(), ctxKey, ctxValue)

	t.Run("with expressions", func(t *testing.T) {
		stub := newHTTPClientStub(t, []httpStubResponse{
			{
				target: "DynamoDB_20120810.DeleteItem",
				body:   marshalJSON(t, map[string]any{}),
				validate: func(req *http.Request) {
					assert.Equal(t, ctxValue, req.Context().Value(ctxKey))
					payload := readRequestJSON(t, req)

					assert.Equal(t, "tbl", payload["TableName"])
					key := requireMapStringAny(t, payload["Key"])
					assert.Equal(t, "123", requireMapStringAny(t, key["id"])["S"])

					assert.Equal(t, "attribute_exists(id)", payload["ConditionExpression"])

					names := requireMapStringAny(t, payload["ExpressionAttributeNames"])
					assert.Equal(t, "name", names["#n"])

					values := requireMapStringAny(t, payload["ExpressionAttributeValues"])
					assert.Equal(t, "John", requireMapStringAny(t, values[":name"])["S"])
				},
			},
		})

		client := newStubbedDynamoClient(t, stub)
		executor := NewExecutorWithBatchSupport(client, ctx)

		input := &CompiledQuery{
			TableName:           "tbl",
			ConditionExpression: "attribute_exists(id)",
			ExpressionAttributeNames: map[string]string{
				"#n": "name",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":name": &types.AttributeValueMemberS{Value: "John"},
			},
		}

		originalNames := map[string]string{}
		for k, v := range input.ExpressionAttributeNames {
			originalNames[k] = v
		}

		originalValues := map[string]types.AttributeValue{}
		for k, v := range input.ExpressionAttributeValues {
			originalValues[k] = v
		}

		key := map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: "123"},
		}

		err := executor.ExecuteDeleteItem(input, key)
		require.NoError(t, err)

		assert.Equal(t, originalNames, input.ExpressionAttributeNames)
		assert.Equal(t, originalValues, input.ExpressionAttributeValues)

		stub.AssertDrained(t)
	})

	t.Run("without expressions", func(t *testing.T) {
		stub := newHTTPClientStub(t, []httpStubResponse{
			{
				target: "DynamoDB_20120810.DeleteItem",
				body:   "{}",
				validate: func(req *http.Request) {
					payload := readRequestJSON(t, req)
					assert.Equal(t, "tbl", payload["TableName"])
					assert.NotContains(t, payload, "ConditionExpression")
					assert.NotContains(t, payload, "ExpressionAttributeNames")
					assert.NotContains(t, payload, "ExpressionAttributeValues")
				},
			},
		})

		executor := NewExecutorWithBatchSupport(newStubbedDynamoClient(t, stub), ctx)

		input := &CompiledQuery{TableName: "tbl"}
		key := map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: "123"},
		}

		err := executor.ExecuteDeleteItem(input, key)
		require.NoError(t, err)

		stub.AssertDrained(t)
	})

	t.Run("wraps errors", func(t *testing.T) {
		stub := newHTTPClientStub(t, []httpStubResponse{
			{
				target: "DynamoDB_20120810.DeleteItem",
				err:    errors.New("delete failed"),
			},
		})

		executor := NewExecutorWithBatchSupport(newStubbedDynamoClient(t, stub), ctx)

		input := &CompiledQuery{TableName: "tbl"}
		key := map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: "123"},
		}

		err := executor.ExecuteDeleteItem(input, key)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete item")
		assert.Contains(t, err.Error(), "delete failed")
	})
}

func TestExecutePutItem(t *testing.T) {
	ctx := context.Background()

	t.Run("with condition expression", func(t *testing.T) {
		stub := newHTTPClientStub(t, []httpStubResponse{
			{
				target: "DynamoDB_20120810.PutItem",
				body:   "{}",
				validate: func(req *http.Request) {
					payload := readRequestJSON(t, req)

					assert.Equal(t, "tbl", payload["TableName"])
					item := requireMapStringAny(t, payload["Item"])
					assert.Equal(t, "alpha", requireMapStringAny(t, item["id"])["S"])

					assert.Equal(t, "attribute_not_exists(id)", payload["ConditionExpression"])
					names := requireMapStringAny(t, payload["ExpressionAttributeNames"])
					assert.Equal(t, "status", names["#st"])

					values := requireMapStringAny(t, payload["ExpressionAttributeValues"])
					assert.Equal(t, "active", requireMapStringAny(t, values[":status"])["S"])
				},
			},
		})

		executor := NewExecutorWithBatchSupport(newStubbedDynamoClient(t, stub), ctx)

		input := &CompiledQuery{
			TableName:           "tbl",
			ConditionExpression: "attribute_not_exists(id)",
			ExpressionAttributeNames: map[string]string{
				"#st": "status",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":status": &types.AttributeValueMemberS{Value: "active"},
			},
		}

		item := map[string]types.AttributeValue{
			"id":    &types.AttributeValueMemberS{Value: "alpha"},
			"score": &types.AttributeValueMemberN{Value: "10"},
		}

		err := executor.ExecutePutItem(input, item)
		require.NoError(t, err)

		stub.AssertDrained(t)
	})

	t.Run("without expressions", func(t *testing.T) {
		stub := newHTTPClientStub(t, []httpStubResponse{
			{
				target: "DynamoDB_20120810.PutItem",
				body:   "{}",
				validate: func(req *http.Request) {
					payload := readRequestJSON(t, req)
					assert.NotContains(t, payload, "ConditionExpression")
					assert.NotContains(t, payload, "ExpressionAttributeNames")
					assert.NotContains(t, payload, "ExpressionAttributeValues")
				},
			},
		})

		executor := NewExecutorWithBatchSupport(newStubbedDynamoClient(t, stub), ctx)

		input := &CompiledQuery{TableName: "tbl"}
		item := map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: "alpha"},
		}

		err := executor.ExecutePutItem(input, item)
		require.NoError(t, err)

		stub.AssertDrained(t)
	})

	t.Run("wraps errors", func(t *testing.T) {
		stub := newHTTPClientStub(t, []httpStubResponse{
			{
				target: "DynamoDB_20120810.PutItem",
				err:    errors.New("put failed"),
			},
		})

		executor := NewExecutorWithBatchSupport(newStubbedDynamoClient(t, stub), ctx)

		input := &CompiledQuery{TableName: "tbl"}
		item := map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: "alpha"},
		}

		err := executor.ExecutePutItem(input, item)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to put item")
		assert.Contains(t, err.Error(), "put failed")
	})
}
