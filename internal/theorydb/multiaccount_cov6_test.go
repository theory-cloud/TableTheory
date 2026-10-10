package theorydb

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
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

// TestMultiAccountDB_FailureLogExcludesHostilePartnerID_COV6 proves the failure
// log sink carries only the generated correlation id. Hostile, log-forging
// partner input can never appear in the emitted log entry (the tainted value
// never reaches the sink at all).
func TestMultiAccountDB_FailureLogExcludesHostilePartnerID_COV6(t *testing.T) {
	fakeNow := time.Unix(4000, 0)
	hostile := "partner\r\nINJECTED forged line"
	account := AccountConfig{
		RoleARN:         "arn:aws:iam::123456789012:role/HostileRole",
		Region:          "us-east-1",
		SessionDuration: time.Hour,
	}

	mdb := &MultiAccountDB{
		cache:      &sync.Map{},
		baseConfig: minimalAWSConfig(nil),
		now:        func() time.Time { return fakeNow },
		accounts:   map[string]AccountConfig{hostile: account},
	}
	mdb.cache.Store(hostile, &cacheEntry{
		db:         &LambdaDB{},
		expiry:     fakeNow.Add(-time.Hour),
		partnerID:  hostile,
		accountCfg: account,
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

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return aws.Config{}, errors.New("boom")
	})

	_, err := mdb.Partner(hostile)
	require.Error(t, err)

	logged := buf.String()
	require.Contains(t, logged, "Credential refresh failed")
	require.Contains(t, logged, "operation_id=")
	require.NotContains(t, logged, "INJECTED", "hostile input must never reach the log")
	require.NotContains(t, logged, "partner_id=", "the partner id must not be logged at all")
	require.NotContains(t, logged, "\r", "a carriage return from hostile input must not reach the log")
	require.Equal(t, 1, strings.Count(logged, "\n"),
		"the failure must emit exactly one log line, with no forged extra line")
}

// TestMultiAccountDB_FailureMarkerNeverServedAsHealthy_COV6 proves a
// failed-refresh marker is never returned as a healthy cached DB (never
// (nil, nil)) even when a frozen clock sits exactly on the marker's expiry, and
// that no rebuild happens inside the backoff window.
func TestMultiAccountDB_FailureMarkerNeverServedAsHealthy_COV6(t *testing.T) {
	frozen := time.Unix(5000, 0)
	partnerID := "partner"
	account := AccountConfig{
		RoleARN:         "arn:aws:iam::123456789012:role/PartnerRole",
		Region:          "us-east-1",
		SessionDuration: time.Hour,
	}

	mdb := &MultiAccountDB{
		cache:      &sync.Map{},
		baseConfig: minimalAWSConfig(nil),
		now:        func() time.Time { return frozen },
		accounts:   map[string]AccountConfig{partnerID: account},
	}
	mdb.cache.Store(partnerID, &cacheEntry{
		db:         &LambdaDB{},
		expiry:     frozen.Add(-time.Hour),
		partnerID:  partnerID,
		accountCfg: account,
	})

	var loads int32
	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		atomic.AddInt32(&loads, 1)
		return aws.Config{}, errors.New("boom")
	})

	_, err := mdb.Partner(partnerID)
	require.Error(t, err)
	require.Equal(t, int32(1), atomic.LoadInt32(&loads))

	// The clock never advances, so every later call observes the marker at its
	// exact expiry instant. It must fail stably and never return (nil, nil).
	for i := 0; i < 5; i++ {
		db, err := mdb.Partner(partnerID)
		require.Error(t, err, "a failure marker must not be served as a healthy DB")
		require.Nil(t, db)
		require.Contains(t, err.Error(), "backing off")
	}
	require.Equal(t, int32(1), atomic.LoadInt32(&loads),
		"a backed-off partner must not be rebuilt while the clock is frozen")
}

// TestMultiAccountDB_ConcurrentRefreshFailureKeepsFreshEntry_COV6 reproduces the
// interleaving where two callers observe the same stale entry, one refreshes it
// successfully, and the other's refresh then fails: the successful fresh entry
// must remain cached rather than being replaced by a nil-db backoff marker.
func TestMultiAccountDB_ConcurrentRefreshFailureKeepsFreshEntry_COV6(t *testing.T) {
	fakeNow := time.Unix(6000, 0)
	partnerID := "partner"
	account := AccountConfig{
		RoleARN:         "arn:aws:iam::123456789012:role/PartnerRole",
		Region:          "us-east-1",
		SessionDuration: time.Hour,
	}

	mdb := &MultiAccountDB{
		cache:      &sync.Map{},
		baseConfig: minimalAWSConfig(nil),
		now:        func() time.Time { return fakeNow },
		accounts:   map[string]AccountConfig{partnerID: account},
	}
	mdb.cache.Store(partnerID, &cacheEntry{
		db:         &LambdaDB{},
		expiry:     fakeNow.Add(-time.Hour),
		partnerID:  partnerID,
		accountCfg: account,
	})

	// The failing caller observes the stale entry before the successful caller
	// stores a fresh one.
	observed, ok := mdb.cache.Load(partnerID)
	require.True(t, ok)
	observedEntry, ok := observed.(*cacheEntry)
	require.True(t, ok)

	healthy := &LambdaDB{}
	fresh := &cacheEntry{
		db:         healthy,
		expiry:     fakeNow.Add(time.Hour),
		partnerID:  partnerID,
		accountCfg: account,
	}

	// Force the adversarial ordering across real goroutines: the success stores
	// first, then the failure records against the entry it observed.
	stored := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		mdb.cache.Store(partnerID, fresh)
		close(stored)
	}()

	go func() {
		defer wg.Done()
		<-stored
		mdb.recordPartnerRefreshFailure(partnerID, account, fakeNow, observedEntry)
	}()

	wg.Wait()

	value, ok := mdb.cache.Load(partnerID)
	require.True(t, ok)
	entry, ok := value.(*cacheEntry)
	require.True(t, ok)
	require.Same(t, fresh, entry,
		"a failed refresh must not clobber a concurrently stored healthy entry")
	require.NotNil(t, entry.db)
	require.True(t, entry.expiry.After(fakeNow))

	// Partner must serve the healthy entry without rebuilding.
	var loads int32
	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		atomic.AddInt32(&loads, 1)
		return minimalAWSConfig(nil), nil
	})
	db, err := mdb.Partner(partnerID)
	require.NoError(t, err)
	require.Same(t, healthy, db)
	require.Equal(t, int32(0), atomic.LoadInt32(&loads))
}

// TestMultiAccountDB_FailureOnCacheMissKeepsFreshEntry_COV6 proves the
// original-miss path (LoadOrStore) also refuses to overwrite a healthy entry a
// concurrent caller stored after the failing caller observed an empty cache.
func TestMultiAccountDB_FailureOnCacheMissKeepsFreshEntry_COV6(t *testing.T) {
	fakeNow := time.Unix(7000, 0)
	partnerID := "partner"
	account := AccountConfig{
		RoleARN:         "arn:aws:iam::123456789012:role/PartnerRole",
		Region:          "us-east-1",
		SessionDuration: time.Hour,
	}

	mdb := &MultiAccountDB{
		cache:      &sync.Map{},
		baseConfig: minimalAWSConfig(nil),
		now:        func() time.Time { return fakeNow },
		accounts:   map[string]AccountConfig{partnerID: account},
	}

	// The failing caller saw an original miss; a concurrent caller then stored a
	// fresh healthy entry before the failure was recorded.
	fresh := &cacheEntry{
		db:         &LambdaDB{},
		expiry:     fakeNow.Add(time.Hour),
		partnerID:  partnerID,
		accountCfg: account,
	}
	mdb.cache.Store(partnerID, fresh)

	mdb.recordPartnerRefreshFailure(partnerID, account, fakeNow, nil)

	value, ok := mdb.cache.Load(partnerID)
	require.True(t, ok)
	entry, ok := value.(*cacheEntry)
	require.True(t, ok)
	require.Same(t, fresh, entry,
		"an original-miss failure must not clobber a concurrently stored healthy entry")
	require.NotNil(t, entry.db)
}
