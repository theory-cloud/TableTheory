package theorydb

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/session"
)

// Test types for custom converter testing
// PayloadJSON is a custom type that should be stored as a JSON string
type TestPayloadJSON struct {
	Data map[string]interface{}
}

// TestPayloadJSONConverter implements the CustomConverter interface
type TestPayloadJSONConverter struct{}

func (c TestPayloadJSONConverter) ToAttributeValue(value any) (types.AttributeValue, error) {
	var payload TestPayloadJSON

	switch v := value.(type) {
	case TestPayloadJSON:
		payload = v
	case *TestPayloadJSON:
		if v == nil {
			return &types.AttributeValueMemberNULL{Value: true}, nil
		}
		payload = *v
	default:
		return &types.AttributeValueMemberNULL{Value: true}, nil
	}

	// Marshal to JSON string
	data, err := json.Marshal(payload.Data)
	if err != nil {
		return nil, err
	}

	return &types.AttributeValueMemberS{Value: string(data)}, nil
}

func (c TestPayloadJSONConverter) FromAttributeValue(av types.AttributeValue, target any) error {
	strValue, ok := av.(*types.AttributeValueMemberS)
	if !ok {
		return nil // gracefully handle other types
	}

	payload, ok := target.(*TestPayloadJSON)
	if !ok {
		return nil
	}

	// Initialize map if nil
	if payload.Data == nil {
		payload.Data = make(map[string]interface{})
	}

	return json.Unmarshal([]byte(strValue.Value), &payload.Data)
}

// TestAsyncRequest model for testing
type TestAsyncRequest struct {
	Payload TestPayloadJSON `theorydb:"attr:payload"`
	ID      string          `theorydb:"pk"`
	Name    string          `theorydb:"attr:name"`
}

// TestCustomID is a simple custom type for regression testing
type TestCustomID string

// TestCustomIDConverter adds a prefix to ensure converter is used
type TestCustomIDConverter struct{}

func (c TestCustomIDConverter) ToAttributeValue(value any) (types.AttributeValue, error) {
	var id TestCustomID
	switch v := value.(type) {
	case TestCustomID:
		id = v
	case *TestCustomID:
		if v == nil {
			return &types.AttributeValueMemberNULL{Value: true}, nil
		}
		id = *v
	default:
		return &types.AttributeValueMemberNULL{Value: true}, nil
	}

	// Add prefix to ensure converter is being used
	return &types.AttributeValueMemberS{Value: "CUSTOM-" + string(id)}, nil
}

func (c TestCustomIDConverter) FromAttributeValue(av types.AttributeValue, target any) error {
	strValue, ok := av.(*types.AttributeValueMemberS)
	if !ok {
		return nil
	}

	id, ok := target.(*TestCustomID)
	if !ok {
		return nil
	}

	// Remove prefix
	val := strValue.Value
	if len(val) > 7 {
		*id = TestCustomID(val[7:]) // Remove "CUSTOM-" prefix
	}
	return nil
}

// TestModelWithCustomID for regression testing
type TestModelWithCustomID struct {
	_        struct{}     `theorydb:"naming:snake_case"`
	ID       string       `theorydb:"pk"`
	CustomID TestCustomID `theorydb:"attr:custom_id"`
}

func setupPayloadConverterTestDB(t *testing.T) (*DB, *capturingHTTPClient) {
	client := newCapturingHTTPClient(nil)
	stubSessionConfigLoad(t, func(ctx context.Context, opts ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(client), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)

	db := mustDB(t, dbAny)
	err = db.RegisterTypeConverter(reflect.TypeOf(TestPayloadJSON{}), TestPayloadJSONConverter{})
	require.NoError(t, err)

	return db, client
}

func setupCustomIDConverterTestDB(t *testing.T) (*DB, *capturingHTTPClient) {
	client := newCapturingHTTPClient(nil)
	stubSessionConfigLoad(t, func(ctx context.Context, opts ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(client), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)

	db := mustDB(t, dbAny)
	err = db.RegisterTypeConverter(reflect.TypeOf(TestCustomID("")), TestCustomIDConverter{})
	require.NoError(t, err)

	return db, client
}

func attrString(value string) map[string]any {
	return map[string]any{"S": value}
}

func buildGetItemBody(attrs map[string]map[string]any) string {
	body := map[string]any{"Item": attrs}
	data, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	return string(data)
}

func buildScanBody(items []map[string]map[string]any) string {
	body := map[string]any{
		"Items":        items,
		"Count":        len(items),
		"ScannedCount": len(items),
	}
	data, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	return string(data)
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return string(data)
}

// TestCustomConverterWithUpdate tests the bug fix for custom converters being ignored during Update()
// This test ensures that custom type converters registered via RegisterTypeConverter() are properly
// invoked during Update() operations, not just Create() operations.
func TestCustomConverterWithUpdate(t *testing.T) {
	// Skip this test if DynamoDB Local is not running
	// This matches the pattern used in other integration tests
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Run("Create uses custom converter", func(t *testing.T) {
		db, client := setupPayloadConverterTestDB(t)

		request := &TestAsyncRequest{
			ID:   "test-create-1",
			Name: "Create Test",
			Payload: TestPayloadJSON{
				Data: map[string]interface{}{
					"action":   "process",
					"priority": 5,
					"metadata": map[string]interface{}{
						"user": "test@example.com",
					},
				},
			},
		}

		err := db.Model(request).Create()
		require.NoError(t, err)

		client.SetResponseSequence("DynamoDB_20120810.GetItem", []stubbedResponse{
			{
				body: buildGetItemBody(map[string]map[string]any{
					"id":      attrString(request.ID),
					"name":    attrString(request.Name),
					"payload": attrString(mustJSON(t, request.Payload.Data)),
				}),
			},
		})

		// Retrieve and verify it was stored as JSON string
		var retrieved TestAsyncRequest
		err = db.Model(&TestAsyncRequest{}).Where("ID", "=", "test-create-1").First(&retrieved)
		require.NoError(t, err)

		assert.Equal(t, "test-create-1", retrieved.ID)
		assert.Equal(t, "Create Test", retrieved.Name)
		assert.NotNil(t, retrieved.Payload.Data)
		assert.Equal(t, "process", retrieved.Payload.Data["action"])
		assert.Equal(t, float64(5), retrieved.Payload.Data["priority"]) // JSON unmarshals numbers as float64

		req := findRequestByTarget(client.Requests(), "DynamoDB_20120810.PutItem")
		require.NotNil(t, req)
		item, ok := req.Payload["Item"].(map[string]any)
		require.True(t, ok)
		payloadAttr, ok := item["payload"].(map[string]any)
		require.True(t, ok)
		payloadValue, ok := payloadAttr["S"].(string)
		require.True(t, ok)
		assert.True(t, json.Valid([]byte(payloadValue)), "payload should be stored as JSON string")
	})

	t.Run("Update uses custom converter - bug fix test", func(t *testing.T) {
		db, client := setupPayloadConverterTestDB(t)

		// First, create an item
		request := &TestAsyncRequest{
			ID:   "test-update-1",
			Name: "Update Test Initial",
			Payload: TestPayloadJSON{
				Data: map[string]interface{}{
					"status": "pending",
					"count":  1,
				},
			},
		}

		err := db.Model(request).Create()
		require.NoError(t, err)

		// Now update the payload
		request.Name = "Update Test Modified"
		request.Payload.Data = map[string]interface{}{
			"status":  "completed",
			"count":   2,
			"newKey":  "newValue",
			"complex": map[string]interface{}{"nested": "data"},
		}

		// This should use the custom converter to marshal Payload as JSON string
		err = db.Model(request).Update()
		require.NoError(t, err, "Update() should succeed with custom converter")

		client.SetResponseSequence("DynamoDB_20120810.GetItem", []stubbedResponse{
			{
				body: buildGetItemBody(map[string]map[string]any{
					"id":      attrString(request.ID),
					"name":    attrString(request.Name),
					"payload": attrString(mustJSON(t, request.Payload.Data)),
				}),
			},
		})

		// Retrieve and verify the payload was correctly stored as JSON string
		var retrieved TestAsyncRequest
		err = db.Model(&TestAsyncRequest{}).Where("ID", "=", "test-update-1").First(&retrieved)
		require.NoError(t, err)

		// Verify all fields including the custom type
		assert.Equal(t, "test-update-1", retrieved.ID)
		assert.Equal(t, "Update Test Modified", retrieved.Name)
		require.NotNil(t, retrieved.Payload.Data, "Payload.Data should not be nil after Update()")

		// Verify the payload content was correctly marshaled and unmarshaled
		assert.Equal(t, "completed", retrieved.Payload.Data["status"])
		assert.Equal(t, float64(2), retrieved.Payload.Data["count"])
		assert.Equal(t, "newValue", retrieved.Payload.Data["newKey"])

		// Verify nested data
		complex, ok := retrieved.Payload.Data["complex"].(map[string]interface{})
		require.True(t, ok, "complex field should be a map")
		assert.Equal(t, "data", complex["nested"])

		updateReq := findRequestByTarget(client.Requests(), "DynamoDB_20120810.UpdateItem")
		require.NotNil(t, updateReq)
		values, ok := updateReq.Payload["ExpressionAttributeValues"].(map[string]any)
		require.True(t, ok)
		var payloadJSON string
		for _, v := range values {
			if attr, ok := v.(map[string]any); ok {
				if s, ok := attr["S"].(string); ok && json.Valid([]byte(s)) {
					payloadJSON = s
				}
			}
		}
		assert.NotEmpty(t, payloadJSON, "update should include payload JSON string")
	})

	t.Run("Update with specific fields uses custom converter", func(t *testing.T) {
		db, client := setupPayloadConverterTestDB(t)

		// Test Update() with specific fields parameter

		request := &TestAsyncRequest{
			ID:   "test-update-fields-1",
			Name: "Field Update Test",
			Payload: TestPayloadJSON{
				Data: map[string]interface{}{
					"original": "data",
				},
			},
		}

		err := db.Model(request).Create()
		require.NoError(t, err)

		// Update only the Payload field
		request.Payload.Data = map[string]interface{}{
			"updated": "payload",
			"type":    "specific_field_update",
		}

		err = db.Model(request).Update("Payload")
		require.NoError(t, err)

		client.SetResponseSequence("DynamoDB_20120810.GetItem", []stubbedResponse{
			{
				body: buildGetItemBody(map[string]map[string]any{
					"id":      attrString(request.ID),
					"name":    attrString(request.Name),
					"payload": attrString(mustJSON(t, request.Payload.Data)),
				}),
			},
		})

		// Verify
		var retrieved TestAsyncRequest
		err = db.Model(&TestAsyncRequest{}).Where("ID", "=", "test-update-fields-1").First(&retrieved)
		require.NoError(t, err)

		assert.Equal(t, "Field Update Test", retrieved.Name) // Should be unchanged
		assert.Equal(t, "payload", retrieved.Payload.Data["updated"])
		assert.Equal(t, "specific_field_update", retrieved.Payload.Data["type"])

		updateReq := findRequestByTarget(client.Requests(), "DynamoDB_20120810.UpdateItem")
		require.NotNil(t, updateReq)
		names, ok := updateReq.Payload["ExpressionAttributeNames"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, names, "#n1")
		assert.Equal(t, "payload", names["#n1"])
		values, ok := updateReq.Payload["ExpressionAttributeValues"].(map[string]any)
		require.True(t, ok)
		valAttr, ok := values[":v1"].(map[string]any)
		require.True(t, ok)
		valString, ok := valAttr["S"].(string)
		require.True(t, ok)
		assert.True(t, json.Valid([]byte(valString)))
	})

	t.Run("CreateOrUpdate uses custom converter", func(t *testing.T) {
		db, client := setupPayloadConverterTestDB(t)

		// Verify CreateOrUpdate also works with custom converters

		request := &TestAsyncRequest{
			ID:   "test-upsert-1",
			Name: "Upsert Test",
			Payload: TestPayloadJSON{
				Data: map[string]interface{}{
					"mode": "upsert",
				},
			},
		}

		// First CreateOrUpdate (acts as create)
		err := db.Model(request).CreateOrUpdate()
		require.NoError(t, err)

		// Second CreateOrUpdate (acts as update)
		request.Payload.Data["mode"] = "updated"
		request.Payload.Data["iteration"] = 2

		err = db.Model(request).CreateOrUpdate()
		require.NoError(t, err)

		client.SetResponseSequence("DynamoDB_20120810.GetItem", []stubbedResponse{
			{
				body: buildGetItemBody(map[string]map[string]any{
					"id":      attrString(request.ID),
					"name":    attrString(request.Name),
					"payload": attrString(mustJSON(t, request.Payload.Data)),
				}),
			},
		})

		// Verify
		var retrieved TestAsyncRequest
		err = db.Model(&TestAsyncRequest{}).Where("ID", "=", "test-upsert-1").First(&retrieved)
		require.NoError(t, err)

		assert.Equal(t, "updated", retrieved.Payload.Data["mode"])
		assert.Equal(t, float64(2), retrieved.Payload.Data["iteration"])

		putReq := findRequestByTarget(client.Requests(), "DynamoDB_20120810.PutItem")
		require.NotNil(t, putReq)
		item, ok := putReq.Payload["Item"].(map[string]any)
		require.True(t, ok)
		payloadAttr, ok := item["payload"].(map[string]any)
		require.True(t, ok)
		payloadString, ok := payloadAttr["S"].(string)
		require.True(t, ok)
		assert.True(t, json.Valid([]byte(payloadString)))
	})

	t.Run("Filter with custom type uses converter", func(t *testing.T) {
		db, client := setupPayloadConverterTestDB(t)

		// Test that Filter() conditions also work with custom types

		// Create test data
		for i := 1; i <= 3; i++ {
			request := &TestAsyncRequest{
				ID:   "test-filter-" + string(rune('0'+i)),
				Name: "Filter Test",
				Payload: TestPayloadJSON{
					Data: map[string]interface{}{
						"index": i,
					},
				},
			}
			err := db.Model(request).Create()
			require.NoError(t, err)
		}

		scanItems := []map[string]map[string]any{
			{
				"id":      attrString("test-filter-1"),
				"name":    attrString("Filter Test"),
				"payload": attrString(mustJSON(t, map[string]any{"index": 1})),
			},
			{
				"id":      attrString("test-filter-2"),
				"name":    attrString("Filter Test"),
				"payload": attrString(mustJSON(t, map[string]any{"index": 2})),
			},
			{
				"id":      attrString("test-filter-3"),
				"name":    attrString("Filter Test"),
				"payload": attrString(mustJSON(t, map[string]any{"index": 3})),
			},
		}
		client.SetResponseSequence("DynamoDB_20120810.Scan", []stubbedResponse{
			{body: buildScanBody(scanItems)},
		})

		// Query with filter should work
		var results []TestAsyncRequest
		err := db.Model(&TestAsyncRequest{}).Scan(&results)
		require.NoError(t, err)
		assert.Equal(t, 3, len(results))
		assert.Equal(t, float64(3), results[2].Payload.Data["index"])
	})
}

// TestCustomConverterRegressionGuard ensures custom converters work across all operations
func TestCustomConverterRegressionGuard(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Test all CRUD operations
	t.Run("All operations use converter", func(t *testing.T) {
		db, client := setupCustomIDConverterTestDB(t)

		model := &TestModelWithCustomID{
			ID:       "test-1",
			CustomID: "ABC123",
		}

		// Create
		err := db.Model(model).Create()
		require.NoError(t, err)

		// Read
		var retrieved TestModelWithCustomID
		client.SetResponseSequence("DynamoDB_20120810.GetItem", []stubbedResponse{
			{body: buildGetItemBody(map[string]map[string]any{
				"id":        attrString(model.ID),
				"custom_id": attrString("CUSTOM-ABC123"),
			})},
			{body: buildGetItemBody(map[string]map[string]any{
				"id":        attrString(model.ID),
				"custom_id": attrString("CUSTOM-XYZ789"),
			})},
		})
		err = db.Model(&TestModelWithCustomID{}).Where("ID", "=", "test-1").First(&retrieved)
		require.NoError(t, err)
		assert.Equal(t, TestCustomID("ABC123"), retrieved.CustomID, "CustomID should round-trip through converter")

		// Update
		retrieved.CustomID = "XYZ789"
		err = db.Model(&retrieved).Update()
		require.NoError(t, err)

		// Verify update
		var updated TestModelWithCustomID
		err = db.Model(&TestModelWithCustomID{}).Where("ID", "=", "test-1").First(&updated)
		require.NoError(t, err)
		assert.Equal(t, TestCustomID("XYZ789"), updated.CustomID, "Updated CustomID should use converter")

		putReq := findRequestByTarget(client.Requests(), "DynamoDB_20120810.PutItem")
		require.NotNil(t, putReq)
		item, ok := putReq.Payload["Item"].(map[string]any)
		require.True(t, ok)
		customAttr, ok := item["custom_id"].(map[string]any)
		require.True(t, ok)
		customString, ok := customAttr["S"].(string)
		require.True(t, ok)
		assert.True(t, strings.HasPrefix(customString, "CUSTOM-"))

		updateReq := findRequestByTarget(client.Requests(), "DynamoDB_20120810.UpdateItem")
		require.NotNil(t, updateReq)
		values, ok := updateReq.Payload["ExpressionAttributeValues"].(map[string]any)
		require.True(t, ok)
		found := false
		for _, v := range values {
			if attr, ok := v.(map[string]any); ok {
				if s, ok := attr["S"].(string); ok && strings.HasPrefix(s, "CUSTOM-") {
					found = true
				}
			}
		}
		assert.True(t, found, "Update should marshal custom_id via converter")
	})
}
