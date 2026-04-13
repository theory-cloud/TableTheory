package theorydb

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type jsonTagCodecRecord struct {
	ID       string         `theorydb:"pk" json:"id"`
	Status   string         `json:"status"`
	Payload  map[string]any `theorydb:"json" json:"payload"`
	Response string         `theorydb:"json" json:"response"`
}

func TestJSONTaggedFieldsUseCompatibleStorageAndReads(t *testing.T) {
	t.Run("create keeps structured fields native and string fields text-backed", func(t *testing.T) {
		db, client := setupMapAnyRoundTripDB(t)

		record := &jsonTagCodecRecord{
			ID:     "rec-1",
			Status: "pending",
			Payload: map[string]any{
				"count": int64(2),
				"mode":  "live",
			},
			Response: `{"accepted":true}`,
		}

		require.NoError(t, db.Model(record).Create())

		putReq := findRequestByTarget(client.Requests(), "DynamoDB_20120810.PutItem")
		require.NotNil(t, putReq)

		item, ok := putReq.Payload["Item"].(map[string]any)
		require.True(t, ok)

		payloadAttr, ok := item["payload"].(map[string]any)
		require.True(t, ok)
		payloadMap, ok := payloadAttr["M"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, "live", payloadMap["mode"].(map[string]any)["S"])
		require.Equal(t, "2", payloadMap["count"].(map[string]any)["N"])

		responseAttr, ok := item["response"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, `{"accepted":true}`, responseAttr["S"])

		client.SetResponseSequence("DynamoDB_20120810.GetItem", []stubbedResponse{
			{body: marshalCapturedResponseBody(t, map[string]any{"Item": item})},
		})

		var got jsonTagCodecRecord
		require.NoError(t, db.Model(&jsonTagCodecRecord{}).Where("ID", "=", record.ID).First(&got))
		require.Equal(t, record.ID, got.ID)
		require.Equal(t, int64(2), got.Payload["count"])
		require.Equal(t, "live", got.Payload["mode"])
		require.Equal(t, `{"accepted":true}`, got.Response)
	})

	t.Run("reads accept legacy json strings and native documents interchangeably", func(t *testing.T) {
		db, client := setupMapAnyRoundTripDB(t)

		client.SetResponseSequence("DynamoDB_20120810.GetItem", []stubbedResponse{
			{body: marshalCapturedResponseBody(t, map[string]any{
				"Item": map[string]any{
					"id":     map[string]any{"S": "rec-legacy"},
					"status": map[string]any{"S": "pending"},
					"payload": map[string]any{
						"S": `{"count":5,"mode":"legacy"}`,
					},
					"response": map[string]any{
						"M": map[string]any{
							"accepted": map[string]any{"BOOL": true},
							"count":    map[string]any{"N": "2"},
						},
					},
				},
			})},
		})

		var got jsonTagCodecRecord
		require.NoError(t, db.Model(&jsonTagCodecRecord{}).Where("ID", "=", "rec-legacy").First(&got))
		require.Equal(t, "rec-legacy", got.ID)
		require.Equal(t, int64(5), got.Payload["count"])
		require.Equal(t, "legacy", got.Payload["mode"])
		require.Equal(t, `{"accepted":true,"count":2}`, got.Response)
	})
}
