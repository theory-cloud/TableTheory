package utils

import (
	"context"
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
	resultA, err := middleware.Process(ctx, "merchant-a", "shared-key", func() (any, error) {
		merchantACalls++
		return map[string]string{"merchant": "a"}, nil
	})
	require.NoError(t, err)
	require.Equal(t, 1, merchantACalls)
	require.Equal(t, "a", resultA.(map[string]string)["merchant"])

	merchantBCalls := 0
	resultB, err := middleware.Process(ctx, "merchant-b", "shared-key", func() (any, error) {
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
	first, err := middleware.Process(ctx, "merchant-a", "key-1", func() (any, error) {
		calls++
		return map[string]string{"status": "captured"}, nil
	})
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	require.Equal(t, "captured", first.(map[string]string)["status"])

	second, err := middleware.Process(ctx, "merchant-a", "key-1", func() (any, error) {
		calls++
		return map[string]string{"status": "should-not-run"}, nil
	})
	require.ErrorIs(t, err, ErrDuplicateRequest)
	require.Equal(t, 1, calls)
	require.Equal(t, "captured", second.(map[string]any)["status"])
}

func TestIdempotencyReadsLegacyUnscopedRecords(t *testing.T) {
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
	result, err := middleware.Process(ctx, "merchant-a", "legacy-key", func() (any, error) {
		called = true
		return nil, nil
	})
	require.ErrorIs(t, err, ErrDuplicateRequest)
	require.False(t, called, "a legacy record must still replay instead of re-executing")
	require.Equal(t, "legacy", result.(map[string]any)["status"])

	otherMerchant, err := middleware.Process(ctx, "merchant-b", "legacy-key", func() (any, error) {
		return map[string]string{"status": "fresh"}, nil
	})
	require.NoError(t, err, "a legacy record belonging to another merchant must not block this merchant")
	require.Equal(t, "fresh", otherMerchant.(map[string]string)["status"])
}
