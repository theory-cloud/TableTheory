// multiaccount.go
package theorydb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/theory-cloud/tabletheory/v4/pkg/session"
)

// MultiAccountDB manages DynamoDB connections across multiple AWS accounts
type MultiAccountDB struct {
	baseDB     *LambdaDB
	accounts   map[string]AccountConfig
	cache      *sync.Map
	now        func() time.Time
	baseConfig aws.Config
	mu         sync.RWMutex
}

// AccountConfig holds configuration for a partner account
type AccountConfig struct {
	RoleARN    string
	ExternalID string
	Region     string
	// Optional: Custom session duration (default is 1 hour)
	SessionDuration time.Duration
}

// NewMultiAccount creates a multi-account aware DB.
//
// NewMultiAccount starts no background work. A partner session is created and
// refreshed synchronously on the Partner() call path, inside the invocation that
// needs it, because Lambda freezes the execution environment when the handler
// returns. One Partner() call touches only the partner it was asked for; it never
// sweeps the whole cache.
func NewMultiAccount(accounts map[string]AccountConfig) (*MultiAccountDB, error) {
	baseDB, err := NewLambdaOptimized()
	if err != nil {
		return nil, fmt.Errorf("failed to create base Lambda DB: %w", err)
	}

	// Load base AWS config
	baseConfig, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to load base AWS config: %w", err)
	}

	return &MultiAccountDB{
		baseDB:     baseDB,
		accounts:   accounts,
		cache:      &sync.Map{},
		now:        time.Now,
		baseConfig: baseConfig,
	}, nil
}

// Partner returns a DB instance for the specified partner account.
//
// A Partner() call performs work only for the partner it was asked for. An
// expired cache entry for another partner is left untouched until that partner is
// requested, so one caller cannot be blocked by unrelated sessions. When a
// refresh fails, the entry records an exponential backoff and later calls within
// that window fail fast instead of repeating the rebuild; a successful refresh
// clears the backoff.
func (mdb *MultiAccountDB) Partner(partnerID string) (*LambdaDB, error) {
	// Empty partner ID returns base DB
	if partnerID == "" {
		return mdb.baseDB, nil
	}

	now := mdb.clock()

	// Check cache first.
	if cached, ok := mdb.cache.Load(partnerID); ok {
		if entry, ok := cached.(*cacheEntry); ok && entry != nil {
			if !entry.isExpiredAt(now) {
				return entry.db, nil
			}
			if now.Before(entry.backoffUntil) {
				return nil, fmt.Errorf(
					"partner %s session refresh is backing off until %s",
					sanitizePartnerID(partnerID),
					entry.backoffUntil.UTC().Format(time.RFC3339),
				)
			}
		}
	}

	// Get account config
	mdb.mu.RLock()
	account, ok := mdb.accounts[partnerID]
	mdb.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("unknown partner: %s", partnerID)
	}

	// Create or refresh DB for this partner only.
	db, err := mdb.createPartnerDB(partnerID, account)
	if err != nil {
		mdb.recordPartnerRefreshFailure(partnerID, account, now)
		return nil, err
	}
	return db, nil
}

// AddPartner dynamically adds a new partner configuration
func (mdb *MultiAccountDB) AddPartner(partnerID string, config AccountConfig) {
	mdb.mu.Lock()
	defer mdb.mu.Unlock()
	mdb.accounts[partnerID] = config
}

// RemovePartner removes a partner and clears its cached connection
func (mdb *MultiAccountDB) RemovePartner(partnerID string) {
	mdb.mu.Lock()
	delete(mdb.accounts, partnerID)
	mdb.mu.Unlock()

	mdb.cache.Delete(partnerID)
}

// createPartnerDB creates a new DB instance for a partner account
func (mdb *MultiAccountDB) createPartnerDB(partnerID string, account AccountConfig) (*LambdaDB, error) {
	// Create STS client
	stsClient := sts.NewFromConfig(mdb.baseConfig)

	// Set session duration (default to 1 hour)
	sessionDuration := account.SessionDuration
	if sessionDuration == 0 {
		sessionDuration = time.Hour
	}

	// Create credentials provider for assume role
	creds := stscreds.NewAssumeRoleProvider(stsClient, account.RoleARN, func(o *stscreds.AssumeRoleOptions) {
		o.ExternalID = &account.ExternalID
		o.RoleSessionName = fmt.Sprintf("theorydb-%s", partnerID)
		o.Duration = sessionDuration
	})

	// Create new config with assumed role
	awsConfigOptions := []func(*config.LoadOptions) error{
		config.WithRegion(account.Region),
		config.WithCredentialsProvider(creds),
	}

	// Add Lambda optimizations if in Lambda environment
	if IsLambdaEnvironment() {
		httpClient := &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
				DisableKeepAlives:   false,
			},
		}
		awsConfigOptions = append(awsConfigOptions,
			config.WithHTTPClient(httpClient),
			config.WithRetryMode(aws.RetryModeAdaptive),
			config.WithRetryMaxAttempts(3),
		)
	}

	// Create partner-specific session config
	cfg := session.Config{
		Region:           account.Region,
		MaxRetries:       3,
		DefaultRCU:       5,
		DefaultWCU:       5,
		AutoMigrate:      false,
		EnableMetrics:    IsLambdaEnvironment(),
		AWSConfigOptions: awsConfigOptions,
	}

	// Create partner DB
	db, err := New(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create partner DB for %s: %w", partnerID, err)
	}

	// Type assert to get the concrete DB
	concreteDB, ok := db.(*DB)
	if !ok {
		return nil, fmt.Errorf("failed to get concrete DB implementation for partner %s", partnerID)
	}

	lambdaDB := &LambdaDB{
		ExtendedDB:     db,
		db:             concreteDB,
		modelCache:     &sync.Map{},
		isLambda:       IsLambdaEnvironment(),
		lambdaMemoryMB: GetLambdaMemoryMB(),
		xrayEnabled:    EnableXRayTracing(),
	}

	// Cache with expiration
	entry := &cacheEntry{
		db:         lambdaDB,
		expiry:     mdb.clock().Add(sessionDuration - 5*time.Minute), // Refresh 5 minutes before expiry
		partnerID:  partnerID,
		accountCfg: account,
	}
	mdb.cache.Store(partnerID, entry)

	return lambdaDB, nil
}

// Partner refresh backoff bounds. A failed partner build is retried after an
// exponentially growing delay, capped here, so a persistently failing partner
// cannot be rebuilt on every request while a healthy one stays fast.
const (
	partnerRefreshBaseBackoff = 250 * time.Millisecond
	partnerRefreshMaxBackoff  = 30 * time.Second
)

// clock returns the manager's time source, defaulting to the wall clock. Tests
// inject a deterministic clock through the unexported now field.
func (mdb *MultiAccountDB) clock() time.Time {
	if mdb != nil && mdb.now != nil {
		return mdb.now()
	}
	return time.Now()
}

// partnerRefreshBackoff returns the backoff for the given consecutive failure
// count, doubling from the base delay and capped at the maximum.
func partnerRefreshBackoff(failures int) time.Duration {
	if failures < 1 {
		failures = 1
	}
	backoff := partnerRefreshBaseBackoff
	for i := 1; i < failures && backoff < partnerRefreshMaxBackoff; i++ {
		backoff *= 2
	}
	if backoff > partnerRefreshMaxBackoff {
		backoff = partnerRefreshMaxBackoff
	}
	return backoff
}

// recordPartnerRefreshFailure stores a backoff marker for a partner whose
// synchronous refresh failed. The marker keeps the partner's account config so a
// later retry can rebuild it, and carries the failure count forward so repeated
// failures back off further. The stored entry has no db and an already-passed
// expiry, so a successful later rebuild replaces it.
func (mdb *MultiAccountDB) recordPartnerRefreshFailure(partnerID string, account AccountConfig, now time.Time) {
	failures := 1
	if existing, ok := mdb.cache.Load(partnerID); ok {
		if entry, ok := existing.(*cacheEntry); ok && entry != nil && entry.accountCfg == account {
			failures = entry.failures + 1
		}
	}

	// SECURITY: Log without exposing sensitive credential details.
	opID := generateOperationID()
	log.Printf("Credential refresh failed: operation_id=%s partner_id=%s",
		opID, sanitizePartnerID(partnerID))

	mdb.cache.Store(partnerID, &cacheEntry{
		expiry:       now,
		partnerID:    partnerID,
		accountCfg:   account,
		failures:     failures,
		backoffUntil: now.Add(partnerRefreshBackoff(failures)),
	})
}

// Close releases the base connection. It starts no background work, so there is
// nothing else to stop.
func (mdb *MultiAccountDB) Close() error {
	return mdb.baseDB.Close()
}

// WithContext returns a new MultiAccountDB with the given context
func (mdb *MultiAccountDB) WithContext(ctx context.Context) *MultiAccountDB {
	// Create new MultiAccountDB without copying sync.Map
	newMDB := &MultiAccountDB{
		baseDB:     mdb.baseDB.WithLambdaTimeout(ctx),
		accounts:   mdb.accounts,
		now:        mdb.now,
		baseConfig: mdb.baseConfig,
	}
	// Share the same cache pointer
	newMDB.cache = mdb.cache
	return newMDB
}

// cacheEntry holds a cached DB connection with expiration. backoffUntil and
// failures track a failed refresh so repeated Partner() calls do not repeat the
// rebuild; a successful createPartnerDB stores a fresh entry with these zeroed.
type cacheEntry struct {
	db           *LambdaDB
	expiry       time.Time
	backoffUntil time.Time
	partnerID    string
	accountCfg   AccountConfig
	failures     int
}

// isExpiredAt reports whether the cache entry has passed its renewal deadline at
// the given instant.
func (e *cacheEntry) isExpiredAt(now time.Time) bool {
	return now.After(e.expiry)
}

// PartnerContext adds partner information to context for tracing
func PartnerContext(ctx context.Context, partnerID string) context.Context {
	return context.WithValue(ctx, partnerContextKey{}, partnerID)
}

// GetPartnerFromContext retrieves partner ID from context
func GetPartnerFromContext(ctx context.Context) string {
	if partnerID, ok := ctx.Value(partnerContextKey{}).(string); ok {
		return partnerID
	}
	return ""
}

type partnerContextKey struct{}

// Security helper functions for safe logging

// generateOperationID generates a unique operation ID for error correlation
func generateOperationID() string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp if crypto/rand fails
		return fmt.Sprintf("op_%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("op_%s", hex.EncodeToString(bytes))
}

// sanitizePartnerID removes or masks sensitive information from partner IDs
func sanitizePartnerID(partnerID string) string {
	if partnerID == "" {
		return "[empty]"
	}

	// If it looks like an AWS account ID (12 digits), mask it
	if len(partnerID) == 12 && isNumeric(partnerID) {
		return partnerID[:4] + "****" + partnerID[8:]
	}

	// If it contains sensitive patterns, mask them
	if strings.Contains(strings.ToLower(partnerID), "arn:aws") {
		return "[masked_arn]"
	}

	// For other cases, limit length and remove special characters
	cleaned := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return -1
	}, partnerID)

	if len(cleaned) > 20 {
		return cleaned[:20] + "..."
	}

	return cleaned
}

// isNumeric checks if a string contains only digits
func isNumeric(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
