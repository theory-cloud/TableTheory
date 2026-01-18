package integration

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type ChargeBase struct {
	CreatedAt time.Time `theorydb:""`
	UpdatedAt time.Time `theorydb:""`
	PK        string    `theorydb:"pk"`
	SK        string    `theorydb:"sk"`
}

type ChargeModel struct {
	ChargeBase
	Currency       string `theorydb:""`
	Status         string `theorydb:""`
	ResponseBody   string `theorydb:""`
	Amount         int    `theorydb:""`
	ResponseStatus int    `theorydb:""`
}

func (ChargeModel) TableName() string {
	return "Charges"
}

func TestUpdateBuilderWithEmbeddedStruct(t *testing.T) {
	testCtx := InitTestDB(t)
	testCtx.CreateTable(t, &ChargeModel{})
	db := testCtx.DB

	t.Run("UpdateBuilder with embedded struct fields", func(t *testing.T) {
		// Create initial charge
		charge := &ChargeModel{
			ChargeBase: ChargeBase{
				PK:        "CHARGE#acct_test#charge_123",
				SK:        "METADATA",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
			Amount:         1000,
			Currency:       "USD",
			Status:         "pending",
			ResponseBody:   "", // Empty initial value as originally intended
			ResponseStatus: 0,
		}

		// Create the item
		err := db.Model(charge).Create()
		require.NoError(t, err)

		// Prepare JSON response
		responseData := map[string]interface{}{
			"id":     "ch_123",
			"status": "succeeded",
			"amount": 1000,
		}
		jsonBytes, err := json.Marshal(responseData)
		require.NoError(t, err)
		jsonString := string(jsonBytes)

		// Update using UpdateBuilder
		builder := db.Model(&ChargeModel{}).
			Where("PK", "=", "CHARGE#acct_test#charge_123").
			Where("SK", "=", "METADATA").
			UpdateBuilder()

		builder.Set("ResponseBody", jsonString).
			Set("ResponseStatus", 200).
			Set("Status", "succeeded").
			Set("UpdatedAt", time.Now())

		err = builder.Execute()
		require.NoError(t, err)

		// Query the item back
		var result ChargeModel
		err = db.Model(&ChargeModel{}).
			Where("PK", "=", "CHARGE#acct_test#charge_123").
			Where("SK", "=", "METADATA").
			First(&result)
		require.NoError(t, err)

		// Verify all fields were updated
		assert.Equal(t, "CHARGE#acct_test#charge_123", result.PK)
		assert.Equal(t, "METADATA", result.SK)
		assert.Equal(t, jsonString, result.ResponseBody, "ResponseBody should contain the JSON string")
		assert.Equal(t, 200, result.ResponseStatus)
		assert.Equal(t, "succeeded", result.Status)
		assert.False(t, result.UpdatedAt.IsZero())
		assert.True(t, result.UpdatedAt.After(result.CreatedAt))
	})

	t.Run("UpdateBuilder with embedded base fields", func(t *testing.T) {
		// Create initial charge
		charge := &ChargeModel{
			ChargeBase: ChargeBase{
				PK:        "CHARGE#acct_test#charge_456",
				SK:        "METADATA",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
			Amount:   2000,
			Currency: "EUR",
			Status:   "pending",
		}

		err := db.Model(charge).Create()
		require.NoError(t, err)

		// Update embedded struct field
		newTime := time.Now().Add(1 * time.Hour)
		builder := db.Model(&ChargeModel{}).
			Where("PK", "=", "CHARGE#acct_test#charge_456").
			Where("SK", "=", "METADATA").
			UpdateBuilder()

		builder.Set("UpdatedAt", newTime).
			Set("Status", "completed")

		err = builder.Execute()
		require.NoError(t, err)

		// Query back
		var result ChargeModel
		err = db.Model(&ChargeModel{}).
			Where("PK", "=", "CHARGE#acct_test#charge_456").
			Where("SK", "=", "METADATA").
			First(&result)
		require.NoError(t, err)

		assert.Equal(t, "completed", result.Status)
		assert.Equal(t, newTime.Unix(), result.UpdatedAt.Unix())
	})
}
