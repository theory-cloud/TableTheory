package theorydb

import (
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOrder represents a test model for stream processing
type TestOrder struct {
	PK         string   `theorydb:"PK" dynamodb:"PK"`
	SK         string   `theorydb:"SK" dynamodb:"SK"`
	OrderID    string   `theorydb:"order_id" dynamodb:"order_id"`
	CustomerID string   `theorydb:"customer_id" dynamodb:"customer_id"`
	Status     string   `theorydb:"status" dynamodb:"status"`
	Items      []string `theorydb:"items" dynamodb:"items"`
	Total      float64  `theorydb:"total" dynamodb:"total"`
}

func TestUnmarshalStreamImage(t *testing.T) {
	// Create a mock DynamoDB stream image
	streamImage := map[string]events.DynamoDBAttributeValue{
		"PK":          events.NewStringAttribute("ORDER#123"),
		"SK":          events.NewStringAttribute("METADATA"),
		"order_id":    events.NewStringAttribute("123"),
		"customer_id": events.NewStringAttribute("CUST456"),
		"total":       events.NewNumberAttribute("99.99"),
		"status":      events.NewStringAttribute("pending"),
		"items": events.NewListAttribute([]events.DynamoDBAttributeValue{
			events.NewStringAttribute("ITEM1"),
			events.NewStringAttribute("ITEM2"),
		}),
	}

	var order TestOrder
	err := UnmarshalStreamImage(streamImage, &order)
	require.NoError(t, err)

	assert.Equal(t, "ORDER#123", order.PK)
	assert.Equal(t, "METADATA", order.SK)
	assert.Equal(t, "123", order.OrderID)
	assert.Equal(t, "CUST456", order.CustomerID)
	assert.Equal(t, 99.99, order.Total)
	assert.Equal(t, "pending", order.Status)
	assert.Equal(t, []string{"ITEM1", "ITEM2"}, order.Items)
}

func TestUnmarshalStreamImage_ComplexTypes(t *testing.T) {
	// Test individual conversions to ensure all types are handled
	assert.NotNil(t, convertLambdaAttributeValue(events.NewStringAttribute("test")))
	assert.NotNil(t, convertLambdaAttributeValue(events.NewNumberAttribute("123")))
	assert.NotNil(t, convertLambdaAttributeValue(events.NewBooleanAttribute(true)))
	assert.NotNil(t, convertLambdaAttributeValue(events.NewNullAttribute()))
	assert.NotNil(t, convertLambdaAttributeValue(events.NewBinaryAttribute([]byte("data"))))

	// Test complex types
	listAttr := events.NewListAttribute([]events.DynamoDBAttributeValue{
		events.NewStringAttribute("item1"),
		events.NewNumberAttribute("42"),
	})
	assert.NotNil(t, convertLambdaAttributeValue(listAttr))

	mapAttr := events.NewMapAttribute(map[string]events.DynamoDBAttributeValue{
		"key": events.NewStringAttribute("value"),
	})
	assert.NotNil(t, convertLambdaAttributeValue(mapAttr))

	// Test set types
	assert.NotNil(t, convertLambdaAttributeValue(events.NewStringSetAttribute([]string{"a", "b"})))
	assert.NotNil(t, convertLambdaAttributeValue(events.NewNumberSetAttribute([]string{"1", "2"})))
	assert.NotNil(t, convertLambdaAttributeValue(events.NewBinarySetAttribute([][]byte{[]byte("data1"), []byte("data2")})))
}

func TestUnmarshalStreamImage_EmptyImage(t *testing.T) {
	streamImage := make(map[string]events.DynamoDBAttributeValue)

	var order TestOrder
	err := UnmarshalStreamImage(streamImage, &order)
	// Should not error on empty image
	assert.NoError(t, err)
}

func TestUnmarshalStreamImage_NilDestination(t *testing.T) {
	streamImage := map[string]events.DynamoDBAttributeValue{
		"PK": events.NewStringAttribute("TEST"),
	}

	err := UnmarshalStreamImage(streamImage, nil)
	assert.Error(t, err)
}

// TestUnmarshalStreamImage_JSONString tests unmarshaling JSON strings into structs
func TestUnmarshalStreamImage_JSONString(t *testing.T) {
	type Address struct {
		Street  string `json:"street"`
		City    string `json:"city"`
		Country string `json:"country"`
	}

	type Customer struct {
		PK      string   `theorydb:"PK"`
		SK      string   `theorydb:"SK"`
		Name    string   `theorydb:"name"`
		Address Address  `theorydb:"address"`
		Tags    []string `theorydb:"tags"`
	}

	// Create stream image with JSON string for struct field
	streamImage := map[string]events.DynamoDBAttributeValue{
		"PK":      events.NewStringAttribute("CUSTOMER#123"),
		"SK":      events.NewStringAttribute("PROFILE"),
		"name":    events.NewStringAttribute("John Doe"),
		"address": events.NewStringAttribute(`{"street":"123 Main St","city":"New York","country":"USA"}`),
		"tags":    events.NewStringAttribute(`["premium","verified"]`),
	}

	var customer Customer
	err := UnmarshalStreamImage(streamImage, &customer)
	require.NoError(t, err)

	assert.Equal(t, "CUSTOMER#123", customer.PK)
	assert.Equal(t, "PROFILE", customer.SK)
	assert.Equal(t, "John Doe", customer.Name)
	assert.Equal(t, "123 Main St", customer.Address.Street)
	assert.Equal(t, "New York", customer.Address.City)
	assert.Equal(t, "USA", customer.Address.Country)
	assert.Equal(t, []string{"premium", "verified"}, customer.Tags)
}

// TestUnmarshalStreamImage_TimeFields tests unmarshaling time fields
func TestUnmarshalStreamImage_TimeFields(t *testing.T) {
	type Event struct {
		CreatedAt time.Time
		UpdatedAt time.Time
		ExpiresAt time.Time
		PK        string `theorydb:"pk"`
		SK        string `theorydb:"sk"`
	}

	now := time.Now().UTC().Truncate(time.Second) // Truncate to match RFC3339 precision

	// Test various time formats
	streamImage := map[string]events.DynamoDBAttributeValue{
		"PK":        events.NewStringAttribute("EVENT#123"),
		"SK":        events.NewStringAttribute("METADATA"),
		"createdAt": events.NewStringAttribute(now.Format(time.RFC3339)),
		"updatedAt": events.NewStringAttribute(now.Add(time.Hour).Format(time.RFC3339Nano)),
		"expiresAt": events.NewStringAttribute(fmt.Sprintf("%d", now.Add(24*time.Hour).Unix())),
	}

	var event Event
	err := UnmarshalStreamImage(streamImage, &event)
	require.NoError(t, err)

	assert.Equal(t, "EVENT#123", event.PK)
	assert.Equal(t, "METADATA", event.SK)
	assert.Equal(t, now, event.CreatedAt)
	assert.Equal(t, now.Add(time.Hour).Truncate(time.Second), event.UpdatedAt.Truncate(time.Second))
	assert.Equal(t, now.Add(24*time.Hour).Unix(), event.ExpiresAt.Unix())
}

// TestUnmarshalStreamImage_MapInterface tests unmarshaling into map[string]interface{}
func TestUnmarshalStreamImage_MapInterface(t *testing.T) {
	type AsyncRequest struct {
		PK      string                 `theorydb:"PK"`
		SK      string                 `theorydb:"SK"`
		Action  string                 `theorydb:"action"`
		Payload map[string]interface{} `theorydb:"payload"`
		Status  string                 `theorydb:"status"`
	}

	// Create stream image with nested map structure
	streamImage := map[string]events.DynamoDBAttributeValue{
		"PK":     events.NewStringAttribute("REQ#123"),
		"SK":     events.NewStringAttribute("REQ#123"),
		"action": events.NewStringAttribute("knowledge_query"),
		"status": events.NewStringAttribute("PENDING"),
		"payload": events.NewMapAttribute(map[string]events.DynamoDBAttributeValue{
			"knowledge_base": events.NewStringAttribute("paytheory"),
			"query":          events.NewStringAttribute("What are the API authentication methods?"),
			"priority":       events.NewNumberAttribute("1"),
			"urgent":         events.NewBooleanAttribute(true),
			"metadata": events.NewMapAttribute(map[string]events.DynamoDBAttributeValue{
				"source":  events.NewStringAttribute("web"),
				"user_id": events.NewNumberAttribute("12345"),
			}),
			"tags": events.NewListAttribute([]events.DynamoDBAttributeValue{
				events.NewStringAttribute("api"),
				events.NewStringAttribute("authentication"),
			}),
		}),
	}

	var asyncReq AsyncRequest
	err := UnmarshalStreamImage(streamImage, &asyncReq)
	require.NoError(t, err)

	assert.Equal(t, "REQ#123", asyncReq.PK)
	assert.Equal(t, "REQ#123", asyncReq.SK)
	assert.Equal(t, "knowledge_query", asyncReq.Action)
	assert.Equal(t, "PENDING", asyncReq.Status)

	// Verify the payload map is properly unmarshaled
	assert.NotEmpty(t, asyncReq.Payload)
	assert.Equal(t, "paytheory", asyncReq.Payload["knowledge_base"])
	assert.Equal(t, "What are the API authentication methods?", asyncReq.Payload["query"])
	assert.Equal(t, int64(1), asyncReq.Payload["priority"])
	assert.Equal(t, true, asyncReq.Payload["urgent"])

	// Check nested map
	metadata, ok := asyncReq.Payload["metadata"].(map[string]interface{})
	require.True(t, ok, "metadata should be a map[string]interface{}")
	assert.Equal(t, "web", metadata["source"])
	assert.Equal(t, int64(12345), metadata["user_id"])

	// Check list
	tags, ok := asyncReq.Payload["tags"].([]interface{})
	require.True(t, ok, "tags should be a []interface{}")
	assert.Equal(t, "api", tags[0])
	assert.Equal(t, "authentication", tags[1])
}
