package theorydb

import (
	"bytes"
	"context"
	"errors"
	"log"
	"sync"
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

func TestMultiAccountDB_refreshExpiredCredentials_RefreshesAndSkipsInvalidEntries_COV6(t *testing.T) {
	mdb := &MultiAccountDB{
		cache:      &sync.Map{},
		baseConfig: minimalAWSConfig(nil),
	}

	mdb.cache.Store(123, &cacheEntry{})
	mdb.cache.Store("not-an-entry", 42)
	mdb.cache.Store("nil-entry", (*cacheEntry)(nil))

	partnerID := "partner"
	mdb.cache.Store(partnerID, &cacheEntry{
		db:     &LambdaDB{},
		expiry: time.Now().Add(-time.Hour),
		accountCfg: AccountConfig{
			RoleARN:         "arn:aws:iam::123456789012:role/PartnerRole",
			ExternalID:      "ext",
			Region:          "us-east-1",
			SessionDuration: time.Hour,
		},
	})

	stubCalled := make(chan struct{})
	var once sync.Once

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		once.Do(func() { close(stubCalled) })
		return minimalAWSConfig(nil), nil
	})

	mdb.refreshExpiredCredentials()

	select {
	case <-stubCalled:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected refresh to invoke config loader")
	}

	require.Eventually(t, func() bool {
		value, ok := mdb.cache.Load(partnerID)
		if !ok {
			return false
		}
		entry, ok := value.(*cacheEntry)
		if !ok || entry == nil {
			return false
		}
		return entry.expiry.After(time.Now())
	}, 500*time.Millisecond, 10*time.Millisecond)
}

func TestMultiAccountDB_refreshExpiredCredentials_LogsOnRefreshError_COV6(t *testing.T) {
	mdb := &MultiAccountDB{
		cache:      &sync.Map{},
		baseConfig: minimalAWSConfig(nil),
	}

	partnerID := "123456789012"
	mdb.cache.Store(partnerID, &cacheEntry{
		db:     &LambdaDB{},
		expiry: time.Now().Add(-time.Hour),
		accountCfg: AccountConfig{
			RoleARN:         "arn:aws:iam::123456789012:role/PartnerRole",
			ExternalID:      "ext",
			Region:          "us-east-1",
			SessionDuration: time.Hour,
		},
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

	stubCalled := make(chan struct{})
	var once sync.Once

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		once.Do(func() { close(stubCalled) })
		return aws.Config{}, errors.New("boom")
	})

	mdb.refreshExpiredCredentials()

	select {
	case <-stubCalled:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected refresh to invoke config loader")
	}

	require.Eventually(t, func() bool {
		return bytes.Contains([]byte(buf.String()), []byte("Credential refresh failed"))
	}, 500*time.Millisecond, 10*time.Millisecond)
}
