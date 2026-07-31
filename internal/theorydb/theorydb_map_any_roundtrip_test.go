package theorydb

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v3/pkg/session"
)

type mapAnyRoundTripRecord struct {
	_               struct{}       `theorydb:"naming:snake_case"`
	DisplayMetadata map[string]any `theorydb:"attr:display_metadata,json" json:"display_metadata"`
	Payload         map[string]any `theorydb:"json" json:"payload"`
	ID              string         `theorydb:"pk" json:"id"`
	Status          string         `json:"status"`
}

func setupMapAnyRoundTripDB(t *testing.T) (*DB, *capturingHTTPClient) {
	t.Helper()

	client := newCapturingHTTPClient(nil)
	stubSessionConfigLoad(t, func(ctx context.Context, opts ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(client), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)

	return mustDB(t, dbAny), client
}

func marshalCapturedResponseBody(t *testing.T, payload any) string {
	t.Helper()

	data, err := json.Marshal(payload)
	require.NoError(t, err)
	return string(data)
}

func capturedPutItems(t *testing.T, client *capturingHTTPClient) []map[string]any {
	t.Helper()

	requests := client.Requests()
	items := make([]map[string]any, 0, len(requests))
	for _, req := range requests {
		if req.Target != "DynamoDB_20120810.PutItem" {
			continue
		}

		item, ok := req.Payload["Item"].(map[string]any)
		require.True(t, ok, "expected captured PutItem payload to include Item map")
		items = append(items, item)
	}

	return items
}

func assertMapAnyRoundTripRecord(t *testing.T, got mapAnyRoundTripRecord, wantID string, wantDisplayName string, wantPurpose string, wantCount int64) {
	t.Helper()

	require.Equal(t, wantID, got.ID)
	require.Equal(t, "pending", got.Status)

	require.NotNil(t, got.DisplayMetadata)
	assert.Equal(t, wantDisplayName, got.DisplayMetadata["display_name"])
	assert.Equal(t, true, got.DisplayMetadata["visible"])

	theme, ok := got.DisplayMetadata["theme"].(map[string]any)
	require.True(t, ok, "theme should be a map[string]any")
	assert.Equal(t, "ocean", theme["palette"])

	require.NotNil(t, got.Payload)
	assert.Equal(t, wantPurpose, got.Payload["purpose"])
	assert.Equal(t, wantCount, got.Payload["count"])

	flags, ok := got.Payload["flags"].([]any)
	require.True(t, ok, "flags should be a []any")
	require.Len(t, flags, 3)
	assert.Equal(t, "alpha", flags[0])
	assert.Equal(t, int64(2), flags[1])
	assert.Equal(t, true, flags[2])

	metadata, ok := got.Payload["metadata"].(map[string]any)
	require.True(t, ok, "metadata should be a map[string]any")
	assert.Equal(t, "api", metadata["source"])
	assert.Equal(t, wantID, metadata["request_id"])
}

func TestMapAnyRoundTripForModelReads(t *testing.T) {
	t.Run("first round-trips persisted map[string]any fields", func(t *testing.T) {
		db, client := setupMapAnyRoundTripDB(t)

		record := &mapAnyRoundTripRecord{
			ID:     "entry-1",
			Status: "pending",
			DisplayMetadata: map[string]any{
				"display_name": "Knowledge Base",
				"visible":      true,
				"theme": map[string]any{
					"palette": "ocean",
				},
			},
			Payload: map[string]any{
				"purpose": "memory_entry",
				"count":   3,
				"flags":   []any{"alpha", 2, true},
				"metadata": map[string]any{
					"source":     "api",
					"request_id": "entry-1",
				},
			},
		}

		require.NoError(t, db.Model(record).Create())

		putReq := findRequestByTarget(client.Requests(), "DynamoDB_20120810.PutItem")
		require.NotNil(t, putReq)
		item, ok := putReq.Payload["Item"].(map[string]any)
		require.True(t, ok)

		client.SetResponseSequence("DynamoDB_20120810.GetItem", []stubbedResponse{
			{body: marshalCapturedResponseBody(t, map[string]any{"Item": item})},
		})

		var got mapAnyRoundTripRecord
		err := db.Model(&mapAnyRoundTripRecord{}).Where("ID", "=", record.ID).First(&got)
		require.NoError(t, err)

		assertMapAnyRoundTripRecord(t, got, "entry-1", "Knowledge Base", "memory_entry", 3)
	})

	t.Run("scan and all round-trip persisted map[string]any fields", func(t *testing.T) {
		db, client := setupMapAnyRoundTripDB(t)

		records := []*mapAnyRoundTripRecord{
			{
				ID:     "entry-1",
				Status: "pending",
				DisplayMetadata: map[string]any{
					"display_name": "Manifest Cache",
					"visible":      true,
					"theme": map[string]any{
						"palette": "ocean",
					},
				},
				Payload: map[string]any{
					"purpose": "manifest_cache",
					"count":   1,
					"flags":   []any{"alpha", 2, true},
					"metadata": map[string]any{
						"source":     "api",
						"request_id": "entry-1",
					},
				},
			},
			{
				ID:     "entry-2",
				Status: "pending",
				DisplayMetadata: map[string]any{
					"display_name": "Memory Entry",
					"visible":      true,
					"theme": map[string]any{
						"palette": "ocean",
					},
				},
				Payload: map[string]any{
					"purpose": "memory_entry",
					"count":   7,
					"flags":   []any{"alpha", 2, true},
					"metadata": map[string]any{
						"source":     "api",
						"request_id": "entry-2",
					},
				},
			},
		}

		for _, record := range records {
			require.NoError(t, db.Model(record).Create())
		}

		items := capturedPutItems(t, client)
		require.Len(t, items, 2)

		scanBody := marshalCapturedResponseBody(t, map[string]any{
			"Items":        items,
			"Count":        len(items),
			"ScannedCount": len(items),
		})
		client.SetResponseSequence("DynamoDB_20120810.Scan", []stubbedResponse{
			{body: scanBody},
			{body: scanBody},
		})

		var scanned []mapAnyRoundTripRecord
		require.NoError(t, db.Model(&mapAnyRoundTripRecord{}).Scan(&scanned))
		require.Len(t, scanned, 2)
		assertMapAnyRoundTripRecord(t, scanned[0], "entry-1", "Manifest Cache", "manifest_cache", 1)
		assertMapAnyRoundTripRecord(t, scanned[1], "entry-2", "Memory Entry", "memory_entry", 7)

		var all []mapAnyRoundTripRecord
		require.NoError(t, db.Model(&mapAnyRoundTripRecord{}).All(&all))
		require.Len(t, all, 2)
		assertMapAnyRoundTripRecord(t, all[0], "entry-1", "Manifest Cache", "manifest_cache", 1)
		assertMapAnyRoundTripRecord(t, all[1], "entry-2", "Memory Entry", "memory_entry", 7)
	})
}
