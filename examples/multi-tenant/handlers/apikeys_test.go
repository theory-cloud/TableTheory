package handlers

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/theory-cloud/tabletheory/v5/examples/multi-tenant/models"
	"github.com/theory-cloud/tabletheory/v5/pkg/core"
	derrors "github.com/theory-cloud/tabletheory/v5/pkg/errors"
)

// fakeAPIKeyStore is the DynamoDB boundary for these tests: it returns the
// current record the way the real `Where("Active", "=", true)` lookup would.
type fakeAPIKeyStore struct {
	mu     sync.Mutex
	record *models.APIKey
}

func (s *fakeAPIKeyStore) set(record *models.APIKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.record = record
}

func (s *fakeAPIKeyStore) lookup() (*models.APIKey, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.record == nil || !s.record.Active {
		return nil, false
	}
	clone := *s.record
	return &clone, true
}

// fakeQuery is the only query surface ValidateAPIKey uses. The embedded
// interface keeps the full core.Query method set satisfied without stubbing
// methods the code under test never calls.
type fakeQuery struct {
	core.Query
	store *fakeAPIKeyStore
}

func (q *fakeQuery) Where(string, string, any) core.Query { return q }

func (q *fakeQuery) First(dest any) error {
	record, ok := q.store.lookup()
	if !ok {
		return derrors.ErrItemNotFound
	}
	apiKey, ok := dest.(*models.APIKey)
	if !ok {
		return fmt.Errorf("unexpected destination type %T", dest)
	}
	*apiKey = *record
	return nil
}

func (q *fakeQuery) Update(...string) error { return nil }

// fakeDB returns the scripted query for every model lookup.
type fakeDB struct {
	core.ExtendedDB
	query *fakeQuery
}

func (db *fakeDB) Model(any) core.Query { return db.query }

func newScriptedHandler(t *testing.T, store *fakeAPIKeyStore, compare func(hashedPassword, password []byte) error) *APIKeyHandler {
	t.Helper()

	db := &fakeDB{query: &fakeQuery{store: store}}
	handler := NewAPIKeyHandler(db)
	if compare != nil {
		handler.verifier.compare = compare
	}
	return handler
}

const testKeyID = "11111111-1111-1111-1111-111111111111"

func activeTestRecord(t *testing.T, secret string) *models.APIKey {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	require.NoError(t, err)
	return &models.APIKey{
		ID:      "org#acme#key#" + testKeyID,
		OrgID:   "org#acme",
		KeyID:   "key#" + testKeyID,
		KeyHash: string(hash),
		Active:  true,
	}
}

// TestValidateAPIKeyBoundsRepeatedWrongSecrets reproduces finding 3b3b4aea's
// primitive: a caller who knows a valid key identifier sends repeated wrong
// secrets. Each attempt used to reach the expensive bcrypt comparison; now the
// comparison count is bounded by the failure limit.
func TestValidateAPIKeyBoundsRepeatedWrongSecrets(t *testing.T) {
	store := &fakeAPIKeyStore{}
	store.set(activeTestRecord(t, "correct-secret"))

	var compares int32
	handler := newScriptedHandler(t, store, func(hashedPassword, password []byte) error {
		atomic.AddInt32(&compares, 1)
		return bcrypt.CompareHashAndPassword(hashedPassword, password)
	})

	for i := 0; i < 20; i++ {
		_, err := handler.ValidateAPIKey(fmt.Sprintf("sk_test_%s.wrong-%d", testKeyID, i))
		require.Error(t, err)
	}

	require.Equal(t, int32(keyVerificationFailureLimit), atomic.LoadInt32(&compares),
		"repeated wrong secrets must not run bcrypt past the failure limit")
}

// TestValidateAPIKeyBoundsConcurrentVerificationWork proves the concurrency
// bound: a parallel burst of attempts for distinct valid key identifiers cannot
// run more than maxConcurrentKeyVerifications comparisons at once.
func TestValidateAPIKeyBoundsConcurrentVerificationWork(t *testing.T) {
	store := &fakeAPIKeyStore{}
	store.set(activeTestRecord(t, "correct-secret"))

	var inFlight, maxInFlight int32
	handler := newScriptedHandler(t, store, func(_, _ []byte) error {
		current := atomic.AddInt32(&inFlight, 1)
		for {
			observed := atomic.LoadInt32(&maxInFlight)
			if current <= observed || atomic.CompareAndSwapInt32(&maxInFlight, observed, current) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		atomic.AddInt32(&inFlight, -1)
		return fmt.Errorf("invalid API key")
	})

	const attempts = 32
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			keyID := fmt.Sprintf("key-%d", i)
			_, _ = handler.ValidateAPIKey(fmt.Sprintf("sk_test_%s.wrong-%d", keyID, i))
		}(i)
	}
	wg.Wait()

	require.LessOrEqual(t, atomic.LoadInt32(&maxInFlight), int32(maxConcurrentKeyVerifications),
		"concurrent bcrypt work must stay within the verification slot bound")
	require.GreaterOrEqual(t, atomic.LoadInt32(&maxInFlight), int32(2),
		"the fixture must exercise real concurrency")
}

// TestValidateAPIKeyOrdinarySuccess is the authorized-success control.
func TestValidateAPIKeyOrdinarySuccess(t *testing.T) {
	store := &fakeAPIKeyStore{}
	store.set(activeTestRecord(t, "correct-secret"))

	handler := newScriptedHandler(t, store, nil)

	key, err := handler.ValidateAPIKey("sk_test_" + testKeyID + ".correct-secret")
	require.NoError(t, err)
	require.Equal(t, "key#"+testKeyID, key.KeyID)
}

// TestValidateAPIKeyNeverCachesCredentials proves a successful verification is
// not cached: revocation and rotation both take effect on the next lookup.
func TestValidateAPIKeyNeverCachesCredentials(t *testing.T) {
	store := &fakeAPIKeyStore{}
	store.set(activeTestRecord(t, "old-secret"))
	handler := newScriptedHandler(t, store, nil)

	_, err := handler.ValidateAPIKey("sk_test_" + testKeyID + ".old-secret")
	require.NoError(t, err)

	// Revocation must be observed immediately.
	revoked := activeTestRecord(t, "old-secret")
	revoked.Active = false
	store.set(revoked)
	_, err = handler.ValidateAPIKey("sk_test_" + testKeyID + ".old-secret")
	require.Error(t, err)

	// Rotation must be observed immediately: the old secret stops working and the
	// new secret authenticates.
	store.set(activeTestRecord(t, "new-secret"))
	_, err = handler.ValidateAPIKey("sk_test_" + testKeyID + ".old-secret")
	require.Error(t, err, "the superseded secret must stop working after rotation")

	key, err := handler.ValidateAPIKey("sk_test_" + testKeyID + ".new-secret")
	require.NoError(t, err, "the rotated secret must authenticate")
	require.Equal(t, "key#"+testKeyID, key.KeyID)
}
