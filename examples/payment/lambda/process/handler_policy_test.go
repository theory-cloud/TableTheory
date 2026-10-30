package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v6"
	payment "github.com/theory-cloud/tabletheory/v6/examples/payment"
	"github.com/theory-cloud/tabletheory/v6/examples/payment/utils"
	"github.com/theory-cloud/tabletheory/v6/pkg/core"
	"github.com/theory-cloud/tabletheory/v6/pkg/session"
	"github.com/theory-cloud/tabletheory/v6/pkg/testing/fakedb"
)

const policyTestSecret = "policy-test-secret"

var policyTestNow = time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)

type policyTestKMS struct{}

func (policyTestKMS) GenerateDataKey(context.Context, *kms.GenerateDataKeyInput, ...func(*kms.Options)) (*kms.GenerateDataKeyOutput, error) {
	return &kms.GenerateDataKeyOutput{
		Plaintext:      make([]byte, 32),
		CiphertextBlob: []byte("wrapped"),
	}, nil
}

func (policyTestKMS) Decrypt(context.Context, *kms.DecryptInput, ...func(*kms.Options)) (*kms.DecryptOutput, error) {
	return &kms.DecryptOutput{Plaintext: make([]byte, 32)}, nil
}

type recordingWebhook struct {
	mu   sync.Mutex
	jobs []*utils.WebhookJob
}

func (r *recordingWebhook) SendSync(_ context.Context, job *utils.WebhookJob) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.jobs = append(r.jobs, job)
	return nil
}

func (r *recordingWebhook) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.jobs)
}

func newPolicyTestDB(t *testing.T) core.ExtendedDB {
	t.Helper()

	db, err := tabletheory.NewWithClient(session.Config{
		Region:    "us-east-1",
		KMSKeyARN: "arn:aws:kms:us-east-1:111111111111:key/test",
		KMSClient: policyTestKMS{},
	}, fakedb.New())
	require.NoError(t, err)

	for _, model := range []any{
		&payment.Payment{},
		&payment.Transaction{},
		&payment.Merchant{},
		&payment.IdempotencyRecord{},
		&payment.Webhook{},
		&payment.UsageCounter{},
	} {
		require.NoError(t, db.CreateTable(model))
	}

	return db
}

func newPolicyTestHandler(t *testing.T, db core.ExtendedDB) (*Handler, *recordingWebhook) {
	t.Helper()

	recorder := &recordingWebhook{}
	return &Handler{
		db:            db,
		idempotency:   utils.NewIdempotencyMiddleware(db, time.Hour),
		webhookSender: recorder,
		jwtValidator:  utils.NewSimpleJWTValidator(policyTestSecret, "", ""),
		now:           func() time.Time { return policyTestNow },
	}, recorder
}

func seedPolicyMerchant(t *testing.T, db core.ExtendedDB, id, status string, limits payment.RateLimits) *payment.Merchant {
	t.Helper()

	merchant := &payment.Merchant{
		ID:         id,
		Name:       "Policy Merchant",
		Email:      id + "@example.com",
		Status:     status,
		RateLimits: limits,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		Version:    1,
	}
	require.NoError(t, db.Model(merchant).Create())
	return merchant
}

func signPolicyToken(t *testing.T, merchantID string, permissions []string) string {
	t.Helper()

	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	claims, err := json.Marshal(utils.JWTClaims{
		MerchantID:  merchantID,
		Permissions: permissions,
		ExpiresAt:   time.Now().Add(time.Hour).Unix(),
	})
	require.NoError(t, err)

	body := header + "." + base64.RawURLEncoding.EncodeToString(claims)
	mac := hmac.New(sha256.New, []byte(policyTestSecret))
	_, err = mac.Write([]byte(body))
	require.NoError(t, err)

	return body + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func policyRequest(t *testing.T, token, idempotencyKey string, amount int64) events.APIGatewayProxyRequest {
	t.Helper()

	body, err := json.Marshal(ProcessPaymentRequest{
		Amount:         amount,
		Currency:       "USD",
		PaymentMethod:  "card",
		IdempotencyKey: idempotencyKey,
	})
	require.NoError(t, err)

	return events.APIGatewayProxyRequest{
		Headers: map[string]string{"Authorization": "Bearer " + token},
		Body:    string(body),
	}
}

func countPolicyRows[T any](t *testing.T, db core.ExtendedDB) int {
	t.Helper()

	var rows []T
	require.NoError(t, db.Model(new(T)).Scan(&rows))
	return len(rows)
}

func usageCount(t *testing.T, db core.ExtendedDB, windowKind string) int64 {
	t.Helper()

	var counters []payment.UsageCounter
	require.NoError(t, db.Model(&payment.UsageCounter{}).Scan(&counters))
	for _, counter := range counters {
		if counter.WindowKind == windowKind {
			return counter.Count
		}
	}
	return 0
}

func TestPaymentPolicyRefusesBeforeAnySideEffect(t *testing.T) {
	validLimits := payment.RateLimits{
		PaymentsPerMinute: 10,
		PaymentsPerDay:    100,
		MaxPaymentAmount:  100,
	}

	tests := []struct {
		merchantStatus string
		name           string
		merchantID     string
		seedMerchant   bool
		permissions    []string
		limits         payment.RateLimits
		amount         int64
	}{
		{name: "missing merchant", merchantID: "merchant-missing", permissions: []string{"payments:write"}, limits: validLimits, amount: 100},
		{name: "suspended merchant", merchantID: "merchant-1", seedMerchant: true, merchantStatus: "suspended", permissions: []string{"payments:write"}, limits: validLimits, amount: 100},
		{name: "missing permission", merchantID: "merchant-1", seedMerchant: true, merchantStatus: payment.MerchantStatusActive, permissions: nil, limits: validLimits, amount: 100},
		{name: "wrong permission", merchantID: "merchant-1", seedMerchant: true, merchantStatus: payment.MerchantStatusActive, permissions: []string{"payments"}, limits: validLimits, amount: 100},
		{name: "permission prefix", merchantID: "merchant-1", seedMerchant: true, merchantStatus: payment.MerchantStatusActive, permissions: []string{"payments:write:all"}, limits: validLimits, amount: 100},
		{name: "zero max amount", merchantID: "merchant-1", seedMerchant: true, merchantStatus: payment.MerchantStatusActive, permissions: []string{"payments:write"}, limits: payment.RateLimits{PaymentsPerMinute: 10, PaymentsPerDay: 100}, amount: 100},
		{name: "negative max amount", merchantID: "merchant-1", seedMerchant: true, merchantStatus: payment.MerchantStatusActive, permissions: []string{"payments:write"}, limits: payment.RateLimits{PaymentsPerMinute: 10, PaymentsPerDay: 100, MaxPaymentAmount: -1}, amount: 100},
		{name: "amount over max", merchantID: "merchant-1", seedMerchant: true, merchantStatus: payment.MerchantStatusActive, permissions: []string{"payments:write"}, limits: validLimits, amount: 101},
		{name: "zero minute limit", merchantID: "merchant-1", seedMerchant: true, merchantStatus: payment.MerchantStatusActive, permissions: []string{"payments:write"}, limits: payment.RateLimits{PaymentsPerDay: 100, MaxPaymentAmount: 100}, amount: 100},
		{name: "zero day limit", merchantID: "merchant-1", seedMerchant: true, merchantStatus: payment.MerchantStatusActive, permissions: []string{"payments:write"}, limits: payment.RateLimits{PaymentsPerMinute: 10, MaxPaymentAmount: 100}, amount: 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newPolicyTestDB(t)
			if tt.seedMerchant {
				seedPolicyMerchant(t, db, tt.merchantID, tt.merchantStatus, tt.limits)
			}
			handler, recorder := newPolicyTestHandler(t, db)

			response, err := handler.HandleRequest(context.Background(), policyRequest(
				t,
				signPolicyToken(t, tt.merchantID, tt.permissions),
				"refusal-key",
				tt.amount,
			))
			require.NoError(t, err)
			require.Equal(t, 403, response.StatusCode)

			require.Zero(t, countPolicyRows[payment.Payment](t, db), "no payment may be written")
			require.Zero(t, countPolicyRows[payment.Transaction](t, db), "no transaction may be written")
			require.Zero(t, countPolicyRows[payment.IdempotencyRecord](t, db), "no idempotency reservation may be written")
			require.Zero(t, recorder.count(), "no webhook work may be scheduled")
		})
	}
}

func TestPaymentPolicyAllowsAuthorizedBoundaryPayment(t *testing.T) {
	limits := payment.RateLimits{
		PaymentsPerMinute: 10,
		PaymentsPerDay:    100,
		MaxPaymentAmount:  100,
	}
	db := newPolicyTestDB(t)
	merchant := seedPolicyMerchant(t, db, "merchant-1", payment.MerchantStatusActive, limits)
	handler, recorder := newPolicyTestHandler(t, db)

	response, err := handler.HandleRequest(context.Background(), policyRequest(
		t,
		signPolicyToken(t, merchant.ID, []string{payment.PermissionPaymentsWrite}),
		"success-key",
		limits.MaxPaymentAmount,
	))
	require.NoError(t, err)
	require.Equal(t, 201, response.StatusCode)

	require.Equal(t, 1, countPolicyRows[payment.Payment](t, db))
	require.Equal(t, 1, countPolicyRows[payment.Transaction](t, db))
	require.Equal(t, 1, recorder.count())
	require.EqualValues(t, 1, usageCount(t, db, payment.UsageWindowMinute))
	require.EqualValues(t, 1, usageCount(t, db, payment.UsageWindowDay))
}

func TestPaymentRateLimitEnforcesMinuteBoundary(t *testing.T) {
	db := newPolicyTestDB(t)
	merchant := seedPolicyMerchant(t, db, "merchant-1", payment.MerchantStatusActive, payment.RateLimits{
		PaymentsPerMinute: 2,
		PaymentsPerDay:    10,
		MaxPaymentAmount:  100,
	})
	handler, recorder := newPolicyTestHandler(t, db)
	token := signPolicyToken(t, merchant.ID, []string{payment.PermissionPaymentsWrite})

	for i := 0; i < 2; i++ {
		response, err := handler.HandleRequest(context.Background(), policyRequest(t, token, "minute-key-"+string(rune('a'+i)), 100))
		require.NoError(t, err)
		require.Equal(t, 201, response.StatusCode)
	}

	response, err := handler.HandleRequest(context.Background(), policyRequest(t, token, "minute-key-c", 100))
	require.NoError(t, err)
	require.Equal(t, 429, response.StatusCode)

	require.Equal(t, 2, countPolicyRows[payment.Payment](t, db))
	require.Equal(t, 2, countPolicyRows[payment.Transaction](t, db))
	require.Equal(t, 2, recorder.count())
	require.EqualValues(t, 2, usageCount(t, db, payment.UsageWindowMinute))
	require.EqualValues(t, 2, usageCount(t, db, payment.UsageWindowDay))
}

func TestPaymentRateLimitEnforcesDayBoundary(t *testing.T) {
	db := newPolicyTestDB(t)
	merchant := seedPolicyMerchant(t, db, "merchant-1", payment.MerchantStatusActive, payment.RateLimits{
		PaymentsPerMinute: 10,
		PaymentsPerDay:    2,
		MaxPaymentAmount:  100,
	})
	handler, recorder := newPolicyTestHandler(t, db)
	token := signPolicyToken(t, merchant.ID, []string{payment.PermissionPaymentsWrite})

	for i := 0; i < 2; i++ {
		response, err := handler.HandleRequest(context.Background(), policyRequest(t, token, "day-key-"+string(rune('a'+i)), 100))
		require.NoError(t, err)
		require.Equal(t, 201, response.StatusCode)
	}

	response, err := handler.HandleRequest(context.Background(), policyRequest(t, token, "day-key-c", 100))
	require.NoError(t, err)
	require.Equal(t, 429, response.StatusCode)

	require.Equal(t, 2, countPolicyRows[payment.Payment](t, db))
	require.Equal(t, 2, recorder.count())
	require.EqualValues(t, 2, usageCount(t, db, payment.UsageWindowMinute))
	require.EqualValues(t, 2, usageCount(t, db, payment.UsageWindowDay))
}

func TestPaymentIdempotentReplayDoesNotDoubleCount(t *testing.T) {
	db := newPolicyTestDB(t)
	merchant := seedPolicyMerchant(t, db, "merchant-1", payment.MerchantStatusActive, payment.RateLimits{
		PaymentsPerMinute: 10,
		PaymentsPerDay:    10,
		MaxPaymentAmount:  100,
	})
	handler, recorder := newPolicyTestHandler(t, db)
	token := signPolicyToken(t, merchant.ID, []string{payment.PermissionPaymentsWrite})

	first, err := handler.HandleRequest(context.Background(), policyRequest(t, token, "replay-key", 100))
	require.NoError(t, err)
	require.Equal(t, 201, first.StatusCode)

	var firstBody struct {
		Data payment.Payment `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(first.Body), &firstBody))

	replay, err := handler.HandleRequest(context.Background(), policyRequest(t, token, "replay-key", 100))
	require.NoError(t, err)
	require.Equal(t, 200, replay.StatusCode)

	var replayBody struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(replay.Body), &replayBody))
	require.Equal(t, firstBody.Data.ID, replayBody.Data["id"])

	require.Equal(t, 1, countPolicyRows[payment.Payment](t, db))
	require.Equal(t, 1, countPolicyRows[payment.Transaction](t, db))
	require.Equal(t, 1, recorder.count())
	require.EqualValues(t, 1, usageCount(t, db, payment.UsageWindowMinute))
	require.EqualValues(t, 1, usageCount(t, db, payment.UsageWindowDay))
}

func TestPaymentRateLimitRefusalReplaysAs429(t *testing.T) {
	db := newPolicyTestDB(t)
	merchant := seedPolicyMerchant(t, db, "merchant-1", payment.MerchantStatusActive, payment.RateLimits{
		PaymentsPerMinute: 1,
		PaymentsPerDay:    10,
		MaxPaymentAmount:  100,
	})
	handler, recorder := newPolicyTestHandler(t, db)
	token := signPolicyToken(t, merchant.ID, []string{payment.PermissionPaymentsWrite})

	first, err := handler.HandleRequest(context.Background(), policyRequest(t, token, "limit-key-a", 100))
	require.NoError(t, err)
	require.Equal(t, 201, first.StatusCode)

	refused, err := handler.HandleRequest(context.Background(), policyRequest(t, token, "limit-key-b", 100))
	require.NoError(t, err)
	require.Equal(t, 429, refused.StatusCode)

	replay, err := handler.HandleRequest(context.Background(), policyRequest(t, token, "limit-key-b", 100))
	require.NoError(t, err)
	require.Equal(t, 429, replay.StatusCode)

	require.Equal(t, 1, countPolicyRows[payment.Payment](t, db))
	require.Equal(t, 1, recorder.count())
	require.EqualValues(t, 1, usageCount(t, db, payment.UsageWindowMinute))
}

func TestConcurrentMinuteLimitCannotOvershoot(t *testing.T) {
	db := newPolicyTestDB(t)
	merchant := seedPolicyMerchant(t, db, "merchant-1", payment.MerchantStatusActive, payment.RateLimits{
		PaymentsPerMinute: 1,
		PaymentsPerDay:    10,
		MaxPaymentAmount:  100,
	})
	handler, recorder := newPolicyTestHandler(t, db)
	token := signPolicyToken(t, merchant.ID, []string{payment.PermissionPaymentsWrite})

	requests := make([]events.APIGatewayProxyRequest, 8)
	for i := range requests {
		requests[i] = policyRequest(t, token, "concurrent-minute-"+string(rune('a'+i)), 100)
	}

	var successes atomic.Int32
	var refusals atomic.Int32
	var unexpected atomic.Int32
	var wg sync.WaitGroup
	for _, request := range requests {
		wg.Add(1)
		go func(request events.APIGatewayProxyRequest) {
			defer wg.Done()
			response, err := handler.HandleRequest(context.Background(), request)
			if err != nil {
				unexpected.Add(1)
				return
			}
			switch response.StatusCode {
			case 201:
				successes.Add(1)
			case 429:
				refusals.Add(1)
			default:
				unexpected.Add(1)
			}
		}(request)
	}
	wg.Wait()

	require.Zero(t, unexpected.Load())
	require.EqualValues(t, 1, successes.Load())
	require.EqualValues(t, 7, refusals.Load())
	require.Equal(t, 1, countPolicyRows[payment.Payment](t, db))
	require.Equal(t, 1, countPolicyRows[payment.Transaction](t, db))
	require.Equal(t, 1, recorder.count())
	require.EqualValues(t, 1, usageCount(t, db, payment.UsageWindowMinute))
	require.EqualValues(t, 1, usageCount(t, db, payment.UsageWindowDay))
}

func TestConcurrentDayLimitCannotOvershoot(t *testing.T) {
	db := newPolicyTestDB(t)
	merchant := seedPolicyMerchant(t, db, "merchant-1", payment.MerchantStatusActive, payment.RateLimits{
		PaymentsPerMinute: 10,
		PaymentsPerDay:    1,
		MaxPaymentAmount:  100,
	})
	handler, recorder := newPolicyTestHandler(t, db)
	token := signPolicyToken(t, merchant.ID, []string{payment.PermissionPaymentsWrite})

	requests := make([]events.APIGatewayProxyRequest, 8)
	for i := range requests {
		requests[i] = policyRequest(t, token, "concurrent-day-"+string(rune('a'+i)), 100)
	}

	var successes atomic.Int32
	var refusals atomic.Int32
	var unexpected atomic.Int32
	var wg sync.WaitGroup
	for _, request := range requests {
		wg.Add(1)
		go func(request events.APIGatewayProxyRequest) {
			defer wg.Done()
			response, err := handler.HandleRequest(context.Background(), request)
			if err != nil {
				unexpected.Add(1)
				return
			}
			switch response.StatusCode {
			case 201:
				successes.Add(1)
			case 429:
				refusals.Add(1)
			default:
				unexpected.Add(1)
			}
		}(request)
	}
	wg.Wait()

	require.Zero(t, unexpected.Load())
	require.EqualValues(t, 1, successes.Load())
	require.EqualValues(t, 7, refusals.Load())
	require.Equal(t, 1, countPolicyRows[payment.Payment](t, db))
	require.Equal(t, 1, countPolicyRows[payment.Transaction](t, db))
	require.Equal(t, 1, recorder.count())
	require.EqualValues(t, 1, usageCount(t, db, payment.UsageWindowMinute))
	require.EqualValues(t, 1, usageCount(t, db, payment.UsageWindowDay))
}
