package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"github.com/theory-cloud/tabletheory/examples/multi-tenant/models"
	"github.com/theory-cloud/tabletheory/pkg/core"
	derrors "github.com/theory-cloud/tabletheory/pkg/errors"
)

// APIKeyHandler handles API key management
type APIKeyHandler struct {
	db core.ExtendedDB
}

// NewAPIKeyHandler creates a new API key handler
func NewAPIKeyHandler(db core.ExtendedDB) *APIKeyHandler {
	return &APIKeyHandler{db: db}
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

	// Generate API key
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		http.Error(w, "failed to generate API key", http.StatusInternalServerError)
		return
	}

	// Create key format: sk_live_{random}
	keyPrefix := "sk_live_"
	if strings.Contains(r.Host, "localhost") || strings.Contains(r.Host, "127.0.0.1") {
		keyPrefix = "sk_test_"
	}
	apiKey := keyPrefix + base64.URLEncoding.EncodeToString(keyBytes)[:32]

	// Hash the key for storage
	hash := sha256.Sum256([]byte(apiKey))
	keyHash := base64.StdEncoding.EncodeToString(hash[:])

	// Create API key record
	keyID := uuid.New().String()
	apiKeyRecord := &models.APIKey{
		ID:        fmt.Sprintf("%s#key#%s", orgID, keyID),
		OrgID:     orgID,
		KeyID:     fmt.Sprintf("key#%s", keyID),
		Name:      req.Name,
		KeyHash:   keyHash,
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
		h.includeUsageStats(&apiKey)
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
	// Hash the provided key
	hash := sha256.Sum256([]byte(apiKey))
	keyHash := base64.StdEncoding.EncodeToString(hash[:])

	// Search for the key by hash
	var apiKeyRecord models.APIKey
	if err := h.db.Model(&models.APIKey{}).
		Where("KeyHash", "=", keyHash).
		Where("Active", "=", true).
		First(&apiKeyRecord); err != nil {
		if err == derrors.ErrItemNotFound {
			return nil, fmt.Errorf("invalid API key")
		}
		return nil, err
	}

	// Check if expired
	if !apiKeyRecord.ExpiresAt.IsZero() && time.Now().After(apiKeyRecord.ExpiresAt) {
		return nil, fmt.Errorf("API key has expired")
	}

	// Update last used timestamp
	apiKeyRecord.LastUsedAt = time.Now()
	h.db.Model(&apiKeyRecord).Update()

	return &apiKeyRecord, nil
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

func (h *APIKeyHandler) includeUsageStats(apiKey *models.APIKey) {
	// Get usage statistics for the last 30 days
	thirtyDaysAgo := time.Now().AddDate(0, 0, -30)

	var resources []models.Resource
	h.db.Model(&models.Resource{}).
		Where("Metadata.api_key_id", "=", apiKey.KeyID).
		Where("Timestamp", ">", thirtyDaysAgo).
		All(&resources)

	// Calculate stats
	stats := struct {
		TotalRequests int            `json:"total_requests"`
		TotalCost     int            `json:"total_cost"`
		LastUsed      string         `json:"last_used"`
		UsageByType   map[string]int `json:"usage_by_type"`
	}{
		UsageByType: make(map[string]int),
	}

	for _, resource := range resources {
		stats.TotalRequests++
		stats.TotalCost += resource.Cost
		stats.UsageByType[resource.Type]++
	}

	if apiKey.LastUsedAt.After(time.Time{}) {
		stats.LastUsed = apiKey.LastUsedAt.Format(time.RFC3339)
	}

	// Add stats to response (this is a simplified approach)
	// In production, you'd properly marshal this into the response
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
