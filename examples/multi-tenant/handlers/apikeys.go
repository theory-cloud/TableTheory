package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"golang.org/x/crypto/bcrypt"

	"github.com/theory-cloud/tabletheory/v4/examples/multi-tenant/models"
	"github.com/theory-cloud/tabletheory/v4/pkg/core"
	derrors "github.com/theory-cloud/tabletheory/v4/pkg/errors"
)

// APIKeyHandler handles API key management
type APIKeyHandler struct {
	db       core.ExtendedDB
	verifier *keyVerifier
}

// NewAPIKeyHandler creates a new API key handler
func NewAPIKeyHandler(db core.ExtendedDB) *APIKeyHandler {
	return &APIKeyHandler{db: db, verifier: newKeyVerifier()}
}

// CreateAPIKey creates a new API key for programmatic access
func (h *APIKeyHandler) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	orgID := vars["org_id"]
	userID := r.Context().Value("user_id").(string)

	// Ensure proper org ID format
	if !strings.HasPrefix(orgID, "org#") {
		orgID = fmt.Sprintf("org#%s", orgID)
	}

	var req struct {
		Name      string   `json:"name"`
		ProjectID string   `json:"project_id"`
		Scopes    []string `json:"scopes"`
		RateLimit int      `json:"rate_limit"`
		ExpiresIn int      `json:"expires_in"` // days, 0 for never
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	// Default rate limit
	if req.RateLimit == 0 {
		req.RateLimit = 1000 // per hour
	}

	// Verify project belongs to organization if specified
	if req.ProjectID != "" {
		if !strings.HasPrefix(req.ProjectID, "project#") {
			req.ProjectID = fmt.Sprintf("project#%s", req.ProjectID)
		}

		var project models.Project
		if err := h.db.Model(&models.Project{}).
			Where("ID", "=", fmt.Sprintf("%s#%s", orgID, req.ProjectID)).
			First(&project); err != nil {
			if err == derrors.ErrItemNotFound {
				http.Error(w, "project not found", http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Generate API key (store a bcrypt hash of the secret portion)
	keyID := uuid.New().String()
	secretBytes := make([]byte, 24)
	if _, err := rand.Read(secretBytes); err != nil {
		http.Error(w, "failed to generate API key", http.StatusInternalServerError)
		return
	}

	// Create key format: sk_live_{keyID}.{secret}
	keyPrefix := "sk_live_"
	if strings.Contains(r.Host, "localhost") || strings.Contains(r.Host, "127.0.0.1") {
		keyPrefix = "sk_test_"
	}
	secret := base64.RawURLEncoding.EncodeToString(secretBytes)
	apiKey := fmt.Sprintf("%s%s.%s", keyPrefix, keyID, secret)

	// Hash the secret for storage
	keyHashBytes, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "failed to generate API key hash", http.StatusInternalServerError)
		return
	}

	// Create API key record
	apiKeyRecord := &models.APIKey{
		ID:        fmt.Sprintf("%s#key#%s", orgID, keyID),
		OrgID:     orgID,
		KeyID:     fmt.Sprintf("key#%s", keyID),
		Name:      req.Name,
		KeyHash:   string(keyHashBytes),
		KeyPrefix: apiKey[:12], // Store first 12 chars for identification
		ProjectID: req.ProjectID,
		Scopes:    req.Scopes,
		RateLimit: req.RateLimit,
		CreatedBy: userID,
		Active:    true,
	}

	// Set expiration if specified
	if req.ExpiresIn > 0 {
		apiKeyRecord.ExpiresAt = time.Now().AddDate(0, 0, req.ExpiresIn)
	}

	// Save API key
	if err := h.db.Model(apiKeyRecord).Create(); err != nil {
		http.Error(w, "failed to create API key", http.StatusInternalServerError)
		return
	}

	// Log audit event
	h.logAuditEvent(orgID, userID, "create", "api_key", apiKeyRecord.KeyID, nil, true, "")

	// Return response with full key (only shown once)
	response := map[string]any{
		"key_id":     apiKeyRecord.KeyID,
		"key":        apiKey, // Only returned on creation
		"key_prefix": apiKeyRecord.KeyPrefix,
		"name":       apiKeyRecord.Name,
		"scopes":     apiKeyRecord.Scopes,
		"rate_limit": apiKeyRecord.RateLimit,
		"expires_at": apiKeyRecord.ExpiresAt,
		"created_at": apiKeyRecord.CreatedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ListAPIKeys lists all API keys for an organization
func (h *APIKeyHandler) ListAPIKeys(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	orgID := vars["org_id"]

	// Ensure proper org ID format
	if !strings.HasPrefix(orgID, "org#") {
		orgID = fmt.Sprintf("org#%s", orgID)
	}

	// Build query
	query := h.db.Model(&models.APIKey{}).Where("OrgID", "=", orgID)

	// Filter by project if specified
	if projectID := r.URL.Query().Get("project_id"); projectID != "" {
		if !strings.HasPrefix(projectID, "project#") {
			projectID = fmt.Sprintf("project#%s", projectID)
		}
		query = query.Where("ProjectID", "=", projectID)
	}

	// Filter by active status
	if active := r.URL.Query().Get("active"); active != "" {
		query = query.Where("Active", "=", active == "true")
	}

	var apiKeys []models.APIKey
	if err := query.All(&apiKeys); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Remove sensitive data
	for i := range apiKeys {
		apiKeys[i].KeyHash = ""
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(apiKeys)
}

// GetAPIKey retrieves details about a specific API key
func (h *APIKeyHandler) GetAPIKey(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	orgID := vars["org_id"]
	keyID := vars["key_id"]

	// Ensure proper ID formats
	if !strings.HasPrefix(orgID, "org#") {
		orgID = fmt.Sprintf("org#%s", orgID)
	}
	if !strings.HasPrefix(keyID, "key#") {
		keyID = fmt.Sprintf("key#%s", keyID)
	}

	var apiKey models.APIKey
	compositeID := fmt.Sprintf("%s#%s", orgID, keyID)

	if err := h.db.Model(&models.APIKey{}).
		Where("ID", "=", compositeID).
		First(&apiKey); err != nil {
		if err == derrors.ErrItemNotFound {
			http.Error(w, "API key not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Remove sensitive data
	apiKey.KeyHash = ""

	// Include usage statistics
	if r.URL.Query().Get("include") == "usage" {
		usageStats := h.includeUsageStats(&apiKey)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"api_key": apiKey,
			"usage":   usageStats,
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(apiKey)
}

// UpdateAPIKey updates an API key's settings
func (h *APIKeyHandler) UpdateAPIKey(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	orgID := vars["org_id"]
	keyID := vars["key_id"]
	userID := r.Context().Value("user_id").(string)

	// Ensure proper ID formats
	if !strings.HasPrefix(orgID, "org#") {
		orgID = fmt.Sprintf("org#%s", orgID)
	}
	if !strings.HasPrefix(keyID, "key#") {
		keyID = fmt.Sprintf("key#%s", keyID)
	}

	var req struct {
		Name      string   `json:"name"`
		Scopes    []string `json:"scopes"`
		RateLimit int      `json:"rate_limit"`
		Active    *bool    `json:"active"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Get current API key
	var apiKey models.APIKey
	compositeID := fmt.Sprintf("%s#%s", orgID, keyID)

	if err := h.db.Model(&models.APIKey{}).
		Where("ID", "=", compositeID).
		First(&apiKey); err != nil {
		if err == derrors.ErrItemNotFound {
			http.Error(w, "API key not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Track changes for audit
	changes := make(map[string]models.Change)

	// Update fields if provided
	if req.Name != "" && req.Name != apiKey.Name {
		changes["name"] = models.Change{From: apiKey.Name, To: req.Name}
		apiKey.Name = req.Name
	}

	if req.Scopes != nil {
		changes["scopes"] = models.Change{From: apiKey.Scopes, To: req.Scopes}
		apiKey.Scopes = req.Scopes
	}

	if req.RateLimit > 0 && req.RateLimit != apiKey.RateLimit {
		changes["rate_limit"] = models.Change{From: apiKey.RateLimit, To: req.RateLimit}
		apiKey.RateLimit = req.RateLimit
	}

	if req.Active != nil && *req.Active != apiKey.Active {
		changes["active"] = models.Change{From: apiKey.Active, To: *req.Active}
		apiKey.Active = *req.Active
	}

	// Update API key
	if err := h.db.Model(&apiKey).Update(); err != nil {
		http.Error(w, "failed to update API key", http.StatusInternalServerError)
		return
	}

	// Log audit event
	h.logAuditEvent(orgID, userID, "update", "api_key", keyID, changes, true, "")

	// Remove sensitive data
	apiKey.KeyHash = ""

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(apiKey)
}

// DeleteAPIKey deactivates an API key
func (h *APIKeyHandler) DeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	orgID := vars["org_id"]
	keyID := vars["key_id"]
	userID := r.Context().Value("user_id").(string)

	// Ensure proper ID formats
	if !strings.HasPrefix(orgID, "org#") {
		orgID = fmt.Sprintf("org#%s", orgID)
	}
	if !strings.HasPrefix(keyID, "key#") {
		keyID = fmt.Sprintf("key#%s", keyID)
	}

	// Get API key
	var apiKey models.APIKey
	compositeID := fmt.Sprintf("%s#%s", orgID, keyID)

	if err := h.db.Model(&models.APIKey{}).
		Where("ID", "=", compositeID).
		First(&apiKey); err != nil {
		if err == derrors.ErrItemNotFound {
			http.Error(w, "API key not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Deactivate instead of hard delete
	apiKey.Active = false
	if err := h.db.Model(&apiKey).Update(); err != nil {
		http.Error(w, "failed to deactivate API key", http.StatusInternalServerError)
		return
	}

	// Log audit event
	h.logAuditEvent(orgID, userID, "deactivate", "api_key", keyID, nil, true, "")

	w.WriteHeader(http.StatusNoContent)
}

// ValidateAPIKey validates an API key and returns its details (used by middleware)
func (h *APIKeyHandler) ValidateAPIKey(apiKey string) (*models.APIKey, error) {
	keyID, secret, err := splitAPIKey(apiKey)
	if err != nil {
		return nil, err
	}

	// Search for the key by ID
	var apiKeyRecord models.APIKey
	if err := h.db.Model(&models.APIKey{}).
		Where("KeyID", "=", fmt.Sprintf("key#%s", keyID)).
		Where("Active", "=", true).
		First(&apiKeyRecord); err != nil {
		if err == derrors.ErrItemNotFound {
			return nil, fmt.Errorf("invalid API key")
		}
		return nil, err
	}

	// The record lookup already proves the caller knows a real, active key
	// identifier (creation and listing return it). Bound the expensive work that
	// follows so that knowledge cannot be turned into pre-auth CPU exhaustion: a
	// caller may hold at most one verification slot, and repeated failures for
	// one key identifier are throttled before the bcrypt comparison runs.
	release, ok := h.verifier.acquire()
	if !ok {
		return nil, errAPIKeyVerificationBusy
	}
	defer release()

	now := time.Now()
	if h.verifier.throttled(keyID, now) {
		return nil, errAPIKeyVerificationThrottled
	}

	if err := h.verifier.compare([]byte(apiKeyRecord.KeyHash), []byte(secret)); err != nil {
		h.verifier.recordFailure(keyID, now)
		return nil, fmt.Errorf("invalid API key")
	}
	h.verifier.clearFailures(keyID)

	// Check if expired
	if !apiKeyRecord.ExpiresAt.IsZero() && time.Now().After(apiKeyRecord.ExpiresAt) {
		return nil, fmt.Errorf("API key has expired")
	}

	// Update last used timestamp
	apiKeyRecord.LastUsedAt = time.Now()
	h.db.Model(&apiKeyRecord).Update()

	return &apiKeyRecord, nil
}

// splitAPIKey parses the "sk_{env}_{keyID}.{secret}" key format.
func splitAPIKey(apiKey string) (keyID, secret string, err error) {
	keyParts := strings.SplitN(apiKey, ".", 2)
	if len(keyParts) != 2 {
		return "", "", fmt.Errorf("invalid API key")
	}
	keyIDWithPrefix := keyParts[0]
	secret = keyParts[1]

	switch {
	case strings.HasPrefix(keyIDWithPrefix, "sk_live_"):
		keyID = strings.TrimPrefix(keyIDWithPrefix, "sk_live_")
	case strings.HasPrefix(keyIDWithPrefix, "sk_test_"):
		keyID = strings.TrimPrefix(keyIDWithPrefix, "sk_test_")
	default:
		return "", "", fmt.Errorf("invalid API key")
	}
	if keyID == "" || secret == "" {
		return "", "", fmt.Errorf("invalid API key")
	}
	return keyID, secret, nil
}

const (
	// maxConcurrentKeyVerifications bounds how many bcrypt comparisons run at
	// once, so a burst of API-key attempts cannot saturate the CPU.
	maxConcurrentKeyVerifications = 8
	// keyVerificationFailureLimit and keyVerificationFailureWindow throttle
	// repeated pre-auth failures for one key identifier, the primitive a caller
	// who knows a valid key identifier would otherwise use to burn CPU.
	keyVerificationFailureLimit  = 5
	keyVerificationFailureWindow = 30 * time.Second
	// keyVerificationWait bounds how long a request waits for a verification slot
	// before it is rejected instead of queueing unboundedly.
	keyVerificationWait = 250 * time.Millisecond
)

var (
	// errAPIKeyVerificationThrottled rejects a key identifier that has failed
	// verification too often inside the window.
	errAPIKeyVerificationThrottled = errors.New("invalid API key")
	// errAPIKeyVerificationBusy rejects a request when every verification slot is
	// occupied past the bounded wait.
	errAPIKeyVerificationBusy = errors.New("API key verification is busy")
)

// keyVerifier bounds the pre-authentication work of API-key verification. It
// never caches a successful verification: a revoked or rotated credential takes
// effect on the next record lookup.
type keyVerifier struct {
	compare func(hashedPassword, password []byte) error

	slots chan struct{}

	mu       sync.Mutex
	failures map[string]*verificationFailures
}

// verificationFailures tracks recent pre-auth failures for one key identifier.
// Entries only exist for identifiers that resolved to a stored key, so the map is
// bounded by the number of keys, not by attacker input.
type verificationFailures struct {
	windowStart time.Time
	count       int
}

func newKeyVerifier() *keyVerifier {
	return &keyVerifier{
		compare:  bcrypt.CompareHashAndPassword,
		slots:    make(chan struct{}, maxConcurrentKeyVerifications),
		failures: make(map[string]*verificationFailures),
	}
}

// acquire reserves a verification slot, waiting at most keyVerificationWait
// before giving up. The returned release function must be called.
func (v *keyVerifier) acquire() (release func(), ok bool) {
	select {
	case v.slots <- struct{}{}:
		return func() { <-v.slots }, true
	default:
	}

	timer := time.NewTimer(keyVerificationWait)
	defer timer.Stop()
	select {
	case v.slots <- struct{}{}:
		return func() { <-v.slots }, true
	case <-timer.C:
		return nil, false
	}
}

// throttled reports whether keyID exhausted its failure budget inside the
// window. A stale window is cleared so the throttle cannot persist forever.
func (v *keyVerifier) throttled(keyID string, now time.Time) bool {
	v.mu.Lock()
	defer v.mu.Unlock()

	entry, ok := v.failures[keyID]
	if !ok {
		return false
	}
	if now.Sub(entry.windowStart) > keyVerificationFailureWindow {
		delete(v.failures, keyID)
		return false
	}
	return entry.count >= keyVerificationFailureLimit
}

func (v *keyVerifier) recordFailure(keyID string, now time.Time) {
	v.mu.Lock()
	defer v.mu.Unlock()

	entry, ok := v.failures[keyID]
	if !ok || now.Sub(entry.windowStart) > keyVerificationFailureWindow {
		v.failures[keyID] = &verificationFailures{count: 1, windowStart: now}
		return
	}
	entry.count++
}

func (v *keyVerifier) clearFailures(keyID string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	delete(v.failures, keyID)
}

// CheckRateLimit checks if the API key has exceeded its rate limit
func (h *APIKeyHandler) CheckRateLimit(keyID string) error {
	// In production, this would use Redis or similar for accurate rate limiting
	// For this example, we'll use a simple DynamoDB approach

	hourAgo := time.Now().Add(-1 * time.Hour)

	// Count requests in the last hour
	// Count requests in the last hour
	count64, err := h.db.Model(&models.Resource{}).
		Where("Metadata.api_key_id", "=", keyID).
		Where("Timestamp", ">", hourAgo).
		Count()
	if err != nil {
		return err
	}
	count := int(count64)

	// Get API key to check limit
	var apiKey models.APIKey
	h.db.Model(&models.APIKey{}).
		Where("KeyID", "=", keyID).
		First(&apiKey)

	if count >= apiKey.RateLimit {
		return fmt.Errorf("rate limit exceeded (%d/%d requests per hour)", count, apiKey.RateLimit)
	}

	return nil
}

// Helper functions

type apiKeyUsageStats struct {
	TotalRequests int            `json:"total_requests"`
	TotalCost     int            `json:"total_cost"`
	LastUsed      string         `json:"last_used,omitempty"`
	UsageByType   map[string]int `json:"usage_by_type"`
}

func (h *APIKeyHandler) includeUsageStats(apiKey *models.APIKey) apiKeyUsageStats {
	// Get usage statistics for the last 30 days
	thirtyDaysAgo := time.Now().AddDate(0, 0, -30)

	var resources []models.Resource
	stats := apiKeyUsageStats{
		UsageByType: make(map[string]int),
	}

	if err := h.db.Model(&models.Resource{}).
		Where("Metadata.api_key_id", "=", apiKey.KeyID).
		Where("Timestamp", ">", thirtyDaysAgo).
		All(&resources); err != nil {
		return stats
	}

	// Calculate stats
	for _, resource := range resources {
		stats.TotalRequests++
		stats.TotalCost += resource.Cost
		stats.UsageByType[resource.Type]++
	}

	if !apiKey.LastUsedAt.IsZero() {
		stats.LastUsed = apiKey.LastUsedAt.Format(time.RFC3339)
	}

	return stats
}

func (h *APIKeyHandler) logAuditEvent(orgID, userID, action, resourceType, resourceID string, changes map[string]models.Change, success bool, errorMsg string) {
	audit := &models.AuditLog{
		ID:           fmt.Sprintf("%s#%s#%s", orgID, time.Now().Format(time.RFC3339Nano), uuid.New().String()),
		OrgID:        orgID,
		Timestamp:    time.Now(),
		EventID:      uuid.New().String(),
		UserID:       userID,
		UserTime:     time.Now(),
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Changes:      changes,
		Success:      success,
		ErrorMessage: errorMsg,
		TTL:          time.Now().AddDate(0, 3, 0).Unix(), // 90 days retention
	}

	// Best effort - don't fail the main operation if audit fails
	h.db.Model(audit).Create()
}
