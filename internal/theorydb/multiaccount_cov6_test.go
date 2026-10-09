package theorydb

import (
	"bytes"
	"context"
	"errors"
	"log"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/stretchr/testify/require"
)

type cov6LogBuffer struct {
	buf bytes.Buffer
	mu  sync.Mutex
}

func (b *cov6LogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *cov6LogBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// TestMultiAccountDB_PartnerRefreshesOnlyRequestedPartner_COV6 proves a Partner()
// call performs no work for other cached partners: an expired unrelated entry is
// left untouched while the requested healthy partner is served from cache.
func TestMultiAccountDB_PartnerRefreshesOnlyRequestedPartner_COV6(t *testing.T) {
	mdb := &MultiAccountDB{
		cache:      &sync.Map{},
		baseConfig: minimalAWSConfig(nil),
		accounts: map[string]AccountConfig{
			"stale":   {RoleARN: "arn:aws:iam::123456789012:role/StaleRole", Region: "us-east-1", SessionDuration: time.Hour},
			"healthy": {RoleARN: "arn:aws:iam::123456789012:role/HealthyRole", Region: "us-east-1", SessionDuration: time.Hour},
		},
	}

	mdb.cache.Store("stale", &cacheEntry{
		db:         &LambdaDB{},
		expiry:     time.Now().Add(-time.Hour),
		partnerID:  "stale",
		accountCfg: mdb.accounts["stale"],
	})
	healthyDB := &LambdaDB{}
	mdb.cache.Store("healthy", &cacheEntry{
		db:         healthyDB,
		expiry:     time.Now().Add(time.Hour),
		partnerID:  "healthy",
		accountCfg: mdb.accounts["healthy"],
	})

	var loads int32
	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		atomic.AddInt32(&loads, 1)
		return minimalAWSConfig(nil), nil
	})

	got, err := mdb.Partner("healthy")
	require.NoError(t, err)
	require.Same(t, healthyDB, got)
	require.Equal(t, int32(0), atomic.LoadInt32(&loads),
		"a request for a healthy partner must not refresh unrelated expired partners")
}

// TestMultiAccountDB_PartnerRefreshFailureBacksOffAndLogs_COV6 proves a failed
// partner refresh records a backoff, is not repeated while the backoff is active,
// and is retried (and recovers) once the backoff elapses.
func TestMultiAccountDB_PartnerRefreshFailureBacksOffAndLogs_COV6(t *testing.T) {
	fakeNow := time.Unix(1000, 0)
	partnerID := "partner"

	mdb := &MultiAccountDB{
		cache:      &sync.Map{},
		baseConfig: minimalAWSConfig(nil),
		now:        func() time.Time { return fakeNow },
		accounts: map[string]AccountConfig{
			partnerID: {
				RoleARN:         "arn:aws:iam::123456789012:role/PartnerRole",
				ExternalID:      "ext",
				Region:          "us-east-1",
				SessionDuration: time.Hour,
			},
		},
	}
	mdb.cache.Store(partnerID, &cacheEntry{
		db:         &LambdaDB{},
		expiry:     fakeNow.Add(-time.Hour),
		partnerID:  partnerID,
		accountCfg: mdb.accounts[partnerID],
	})

	var buf cov6LogBuffer
	prevWriter := log.Writer()
	prevFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(prevWriter)
		log.SetFlags(prevFlags)
	})

	var loads int32
	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		atomic.AddInt32(&loads, 1)
		return aws.Config{}, errors.New("boom")
	})

	_, err := mdb.Partner(partnerID)
	require.Error(t, err)
	require.Equal(t, int32(1), atomic.LoadInt32(&loads))
	require.Contains(t, buf.String(), "Credential refresh failed")

	// Within the backoff window the expensive rebuild must not be repeated.
	fakeNow = fakeNow.Add(10 * time.Millisecond)
	_, err = mdb.Partner(partnerID)
	require.Error(t, err)
	require.Equal(t, int32(1), atomic.LoadInt32(&loads),
		"a backed-off partner must not be rebuilt on every request")

	// Once the backoff elapses the refresh is retried and can recover.
	fakeNow = fakeNow.Add(partnerRefreshBaseBackoff)
	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		atomic.AddInt32(&loads, 1)
		return minimalAWSConfig(nil), nil
	})

	db, err := mdb.Partner(partnerID)
	require.NoError(t, err)
	require.NotNil(t, db)
	require.Equal(t, int32(2), atomic.LoadInt32(&loads))

	value, ok := mdb.cache.Load(partnerID)
	require.True(t, ok)
	entry, ok := value.(*cacheEntry)
	require.True(t, ok)
	require.True(t, entry.expiry.After(fakeNow), "a successful retry must refresh the entry")
	require.Zero(t, entry.failures, "a successful retry must clear the failure count")
}
