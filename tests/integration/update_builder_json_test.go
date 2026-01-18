package integration

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type IdempotencyKey struct {
	UpdatedAt      time.Time `theorydb:""`
	PK             string    `theorydb:"pk"`
	SK             string    `theorydb:"sk"`
	ResponseBody   string    `theorydb:""`
	Status         string    `theorydb:""`
	ResponseStatus int       `theorydb:""`
}

func (IdempotencyKey) TableName() string {
	return "IdempotencyKeys"
}

func TestUpdateBuilderWithJSONString(t *testing.T) {
	testCtx := InitTestDB(t)
	testCtx.CreateTable(t, &IdempotencyKey{})
	db := testCtx.DB

	t.Run("UpdateBuilder with JSON string content", func(t *testing.T) {
		// Create initial item
		item := &IdempotencyKey{
			PK:             "IDEMPOTENCY#acct_test#key123",
			SK:             "METADATA",
			ResponseBody:   "",
			ResponseStatus: 0,
			Status:         "pending",
			UpdatedAt:      time.Now(),
		}

		// Create the item first
		err := db.Model(item).Create()
		require.NoError(t, err)

		// Prepare JSON response body
		responseData := map[string]interface{}{
			"id":     "pi_123",
			"status": "succeeded",
			"amount": 1000,
			"nested": map[string]interface{}{
				"field1": "value1",
				"field2": 123,
			},
		}
		jsonBytes, err := json.Marshal(responseData)
		require.NoError(t, err)
		jsonString := string(jsonBytes)

		// Update using UpdateBuilder
		builder := db.Model(&IdempotencyKey{}).
			Where("PK", "=", "IDEMPOTENCY#acct_test#key123").
			Where("SK", "=", "METADATA").
			UpdateBuilder()

		builder.Set("ResponseBody", jsonString).
			Set("ResponseStatus", 200).
			Set("Status", "completed").
			Set("UpdatedAt", time.Now())

		err = builder.Execute()
		require.NoError(t, err)

		// Query the item back
		var result IdempotencyKey
		err = db.Model(&IdempotencyKey{}).
			Where("PK", "=", "IDEMPOTENCY#acct_test#key123").
			Where("SK", "=", "METADATA").
			First(&result)
		require.NoError(t, err)

		// Verify all fields were updated
		assert.Equal(t, "IDEMPOTENCY#acct_test#key123", result.PK)
		assert.Equal(t, "METADATA", result.SK)
		assert.Equal(t, jsonString, result.ResponseBody, "ResponseBody should contain the JSON string")
		assert.Equal(t, 200, result.ResponseStatus)
		assert.Equal(t, "completed", result.Status)
		assert.False(t, result.UpdatedAt.IsZero())

		// Verify the JSON can be unmarshaled back
		var unmarshaledData map[string]interface{}
		err = json.Unmarshal([]byte(result.ResponseBody), &unmarshaledData)
		require.NoError(t, err)
		assert.Equal(t, "pi_123", unmarshaledData["id"])
		assert.Equal(t, "succeeded", unmarshaledData["status"])
	})

	t.Run("UpdateBuilder with ExecuteWithResult", func(t *testing.T) {
		// Create initial item
		item := &IdempotencyKey{
			PK:             "IDEMPOTENCY#acct_test#key456",
			SK:             "METADATA",
			ResponseBody:   "",
			ResponseStatus: 0,
			Status:         "pending",
			UpdatedAt:      time.Now(),
		}

		err := db.Model(item).Create()
		require.NoError(t, err)

		// Update and get result
		builder := db.Model(&IdempotencyKey{}).
			Where("PK", "=", "IDEMPOTENCY#acct_test#key456").
			Where("SK", "=", "METADATA").
			UpdateBuilder()

		jsonResponse := `{"transaction_id": "txn_789", "result": "processed"}`
		builder.Set("ResponseBody", jsonResponse).
			Set("ResponseStatus", 201).
			Set("Status", "processed")

		var updatedItem IdempotencyKey
		err = builder.ExecuteWithResult(&updatedItem)
		require.NoError(t, err)

		// Verify the returned item has all updated fields
		assert.Equal(t, jsonResponse, updatedItem.ResponseBody)
		assert.Equal(t, 201, updatedItem.ResponseStatus)
		assert.Equal(t, "processed", updatedItem.Status)
	})

	t.Run("Compare with Update method", func(t *testing.T) {
		// Create initial item
		item := &IdempotencyKey{
			PK:             "IDEMPOTENCY#acct_test#key789",
			SK:             "METADATA",
			ResponseBody:   "",
			ResponseStatus: 0,
			Status:         "pending",
			UpdatedAt:      time.Now(),
		}

		err := db.Model(item).Create()
		require.NoError(t, err)

		// Update using full Update method
		item.ResponseBody = `{"order_id": "ord_123", "status": "complete"}`
		item.ResponseStatus = 200
		item.Status = "complete"
		item.UpdatedAt = time.Now()

		err = db.Model(item).
			Where("PK", "=", item.PK).
			Where("SK", "=", item.SK).
			Update()
		require.NoError(t, err)

		// Query back
		var result IdempotencyKey
		err = db.Model(&IdempotencyKey{}).
			Where("PK", "=", "IDEMPOTENCY#acct_test#key789").
			Where("SK", "=", "METADATA").
			First(&result)
		require.NoError(t, err)

		assert.Equal(t, `{"order_id": "ord_123", "status": "complete"}`, result.ResponseBody)
		assert.Equal(t, 200, result.ResponseStatus)
		assert.Equal(t, "complete", result.Status)
	})
}

func TestUpdateBuilderEdgeCases(t *testing.T) {
	testCtx := InitTestDB(t)
	testCtx.CreateTable(t, &IdempotencyKey{})
	db := testCtx.DB

	t.Run("Empty string update", func(t *testing.T) {
		item := &IdempotencyKey{
			PK:             "IDEMPOTENCY#test#empty",
			SK:             "METADATA",
			ResponseBody:   "initial content",
			ResponseStatus: 200,
			Status:         "complete",
			UpdatedAt:      time.Now(),
		}

		err := db.Model(item).Create()
		require.NoError(t, err)

		// Update to empty string
		builder := db.Model(&IdempotencyKey{}).
			Where("PK", "=", item.PK).
			Where("SK", "=", item.SK).
			UpdateBuilder()

		err = builder.Set("ResponseBody", "").Execute()
		require.NoError(t, err)

		var result IdempotencyKey
		err = db.Model(&IdempotencyKey{}).
			Where("PK", "=", item.PK).
			Where("SK", "=", item.SK).
			First(&result)
		require.NoError(t, err)

		assert.Equal(t, "", result.ResponseBody, "Should be able to set empty string")
	})

	t.Run("Special characters in JSON", func(t *testing.T) {
		item := &IdempotencyKey{
			PK:             "IDEMPOTENCY#test#special",
			SK:             "METADATA",
			ResponseBody:   "",
			ResponseStatus: 0,
			Status:         "pending",
			UpdatedAt:      time.Now(),
		}

		err := db.Model(item).Create()
		require.NoError(t, err)

		// JSON with special characters
		complexJSON := `{"message": "Hello \"World\"!", "path": "C:\\Users\\test", "unicode": "Hello 世界 🌍"}`

		builder := db.Model(&IdempotencyKey{}).
			Where("PK", "=", item.PK).
			Where("SK", "=", item.SK).
			UpdateBuilder()

		err = builder.Set("ResponseBody", complexJSON).Execute()
		require.NoError(t, err)

		var result IdempotencyKey
		err = db.Model(&IdempotencyKey{}).
			Where("PK", "=", item.PK).
			Where("SK", "=", item.SK).
			First(&result)
		require.NoError(t, err)

		assert.Equal(t, complexJSON, result.ResponseBody)
	})
}
