package utils

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v6"
	payment "github.com/theory-cloud/tabletheory/v6/examples/payment"
	"github.com/theory-cloud/tabletheory/v6/pkg/core"
	"github.com/theory-cloud/tabletheory/v6/pkg/session"
	"github.com/theory-cloud/tabletheory/v6/pkg/testing/fakedb"
)

func newIdempotencyDB(t *testing.T) core.ExtendedDB {
	t.Helper()

	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, fakedb.New())
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&payment.IdempotencyRecord{}))
	return db
}

func TestIdempotencyKeyIsScopedPerMerchant(t *testing.T) {
	require.NotEqual(t,
		scopedIdempotencyKey("a#b", "c"),
		scopedIdempotencyKey("a", "b#c"),
		"the merchant boundary must be unambiguous")
	require.Equal(t,
		scopedIdempotencyKey("merchant-a", "key"),
		scopedIdempotencyKey("merchant-a", "key"))
	require.NotEqual(t,
		scopedIdempotencyKey("merchant-a", "key"),
		"key",
		"a scoped key must not collide with the legacy raw key")
}

func TestIdempotencyReservationCannotDenyAnotherMerchant(t *testing.T) {
	db := newIdempotencyDB(t)
	middleware := NewIdempotencyMiddleware(db, time.Hour)
	ctx := context.Background()

	merchantACalls := 0
	resultA, err := middleware.Process(ctx, "merchant-a", "shared-key", "hash-a", func() (any, error) {
		merchantACalls++
		return map[string]string{"merchant": "a"}, nil
	})
	require.NoError(t, err)
	require.Equal(t, 1, merchantACalls)
	require.Equal(t, "a", resultA.(map[string]string)["merchant"])

	merchantBCalls := 0
	resultB, err := middleware.Process(ctx, "merchant-b", "shared-key", "hash-b", func() (any, error) {
		merchantBCalls++
		return map[string]string{"merchant": "b"}, nil
	})
	require.NoError(t, err, "another merchant's reservation must not deny this merchant")
	require.Equal(t, 1, merchantBCalls)
	require.Equal(t, "b", resultB.(map[string]string)["merchant"])
}

func TestIdempotencyReplaysTheSameMerchantRequest(t *testing.T) {
	db := newIdempotencyDB(t)
	middleware := NewIdempotencyMiddleware(db, time.Hour)
	ctx := context.Background()

	calls := 0
	first, err := middleware.Process(ctx, "merchant-a", "key-1", "hash-1", func() (any, error) {
		calls++
		return map[string]string{"status": "captured"}, nil
	})
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	require.Equal(t, "captured", first.(map[string]string)["status"])

	second, err := middleware.Process(ctx, "merchant-a", "key-1", "hash-1", func() (any, error) {
		calls++
		return map[string]string{"status": "should-not-run"}, nil
	})
	require.ErrorIs(t, err, ErrDuplicateRequest)
	require.Equal(t, 1, calls)
	replay, ok := second.(*Replay)
	require.True(t, ok)
	require.Equal(t, 200, replay.StatusCode)
	require.Equal(t, "captured", replay.Body.(map[string]any)["status"])

	var stored payment.IdempotencyRecord
	require.NoError(t, db.Model(&payment.IdempotencyRecord{}).
		Where("Key", "=", scopedIdempotencyKey("merchant-a", "key-1")).
		First(&stored))
	require.Equal(t, "hash-1", stored.RequestHash)
}

func TestIdempotencyLegacyRecordsWithoutRequestHashFailClosed(t *testing.T) {
	db := newIdempotencyDB(t)
	middleware := NewIdempotencyMiddleware(db, time.Hour)
	ctx := context.Background()

	require.NoError(t, db.Model(&payment.IdempotencyRecord{
		Key:        "legacy-key",
		MerchantID: "merchant-a",
		Response:   `{"status":"legacy"}`,
		StatusCode: 200,
		CreatedAt:  time.Now(),
		ExpiresAt:  time.Now().Add(time.Hour).Unix(),
	}).Create())

	called := false
	result, err := middleware.Process(ctx, "merchant-a", "legacy-key", "current-hash", func() (any, error) {
		called = true
		return map[string]string{"status": "should-not-run"}, nil
	})
	require.ErrorIs(t, err, ErrIdempotencyConflict)
	require.Nil(t, result)
	require.False(t, called, "a legacy record without a request hash must not replay or execute")

	var stored payment.IdempotencyRecord
	require.NoError(t, db.Model(&payment.IdempotencyRecord{}).
		Where("Key", "=", "legacy-key").
		First(&stored))
	require.Empty(t, stored.RequestHash, "a refused legacy record must not be rewritten")
	require.Equal(t, `{"status":"legacy"}`, stored.Response)

	otherMerchant, err := middleware.Process(ctx, "merchant-b", "legacy-key", "fresh-hash", func() (any, error) {
		return map[string]string{"status": "fresh"}, nil
	})
	require.NoError(t, err, "a legacy record belonging to another merchant must not block this merchant")
	require.Equal(t, "fresh", otherMerchant.(map[string]string)["status"])
}

func TestIdempotencyRejectsSameKeyWithDifferentRequestHash(t *testing.T) {
	db := newIdempotencyDB(t)
	middleware := NewIdempotencyMiddleware(db, time.Hour)
	ctx := context.Background()

	calls := 0
	first, err := middleware.Process(ctx, "merchant-a", "key-1", "hash-a", func() (any, error) {
		calls++
		return map[string]string{"status": "captured"}, nil
	})
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	require.Equal(t, "captured", first.(map[string]string)["status"])

	second, err := middleware.Process(ctx, "merchant-a", "key-1", "hash-b", func() (any, error) {
		calls++
		return map[string]string{"status": "should-not-run"}, nil
	})
	require.ErrorIs(t, err, ErrIdempotencyConflict)
	require.Nil(t, second)
	require.Equal(t, 1, calls, "a mismatched payload must not execute the handler")

	var stored payment.IdempotencyRecord
	require.NoError(t, db.Model(&payment.IdempotencyRecord{}).
		Where("Key", "=", scopedIdempotencyKey("merchant-a", "key-1")).
		First(&stored))
	require.Equal(t, "hash-a", stored.RequestHash, "a refused mismatch must not rewrite the stored hash")
	require.Equal(t, "captured", mustDecodeStatus(t, stored.Response))
}

func TestIdempotencyRequiresRequestHash(t *testing.T) {
	db := newIdempotencyDB(t)
	middleware := NewIdempotencyMiddleware(db, time.Hour)

	called := false
	_, err := middleware.Process(context.Background(), "merchant-a", "key-1", "", func() (any, error) {
		called = true
		return nil, nil
	})
	require.Error(t, err)
	require.False(t, called)
	require.Zero(t, countIdempotencyRecords(t, db))
}

func mustDecodeStatus(t *testing.T, raw string) string {
	t.Helper()

	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &body))
	status, ok := body["status"].(string)
	require.True(t, ok)
	return status
}

func countIdempotencyRecords(t *testing.T, db core.ExtendedDB) int {
	t.Helper()

	var records []payment.IdempotencyRecord
	require.NoError(t, db.Model(&payment.IdempotencyRecord{}).Scan(&records))
	return len(records)
}

type testStatusError struct{ status int }

func (e testStatusError) Error() string   { return "refused" }
func (e testStatusError) HTTPStatus() int { return e.status }

func TestIdempotencyReplaysRecordedRefusalStatus(t *testing.T) {
	db := newIdempotencyDB(t)
	middleware := NewIdempotencyMiddleware(db, time.Hour)
	ctx := context.Background()

	calls := 0
	_, err := middleware.Process(ctx, "merchant-a", "refused-key", "refused-hash", func() (any, error) {
		calls++
		return nil, testStatusError{status: 429}
	})
	require.Error(t, err)
	require.Equal(t, 1, calls)

	replay, err := middleware.Process(ctx, "merchant-a", "refused-key", "refused-hash", func() (any, error) {
		calls++
		return nil, nil
	})
	require.ErrorIs(t, err, ErrDuplicateRequest)
	require.Equal(t, 1, calls, "a replay must not execute the handler again")
	stored, ok := replay.(*Replay)
	require.True(t, ok)
	require.Equal(t, 429, stored.StatusCode)
	require.Equal(t, "refused", stored.Body.(map[string]any)["error"])
}
