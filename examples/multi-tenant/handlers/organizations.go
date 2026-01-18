package handlers

import (
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

// OrganizationHandler handles organization-related requests
type OrganizationHandler struct {
	db core.ExtendedDB
}

// NewOrganizationHandler creates a new organization handler
func NewOrganizationHandler(db core.ExtendedDB) *OrganizationHandler {
	return &OrganizationHandler{db: db}
}

// CreateOrganization creates a new organization
func (h *OrganizationHandler) CreateOrganization(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name       string `json:"name"`
		Slug       string `json:"slug"`
		OwnerEmail string `json:"owner_email"`
		Plan       string `json:"plan"`
		Domain     string `json:"domain,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.Name == "" || req.Slug == "" || req.OwnerEmail == "" {
		http.Error(w, "name, slug, and owner_email are required", http.StatusBadRequest)
		return
	}

	// Default to free plan if not specified
	if req.Plan == "" {
		req.Plan = models.PlanFree
	}

	// Get plan limits based on plan type
	limits := getPlanLimits(req.Plan)

	// Check if slug exists
	var existing models.Organization
	if err := h.db.Model(&models.Organization{}).
		Where("Slug", "=", req.Slug).
		First(&existing); err == nil {
		http.Error(w, "organization with this slug already exists", http.StatusConflict)
		return
	} else if err != derrors.ErrItemNotFound {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Create organization
	org := &models.Organization{
		ID:     fmt.Sprintf("org#%s", uuid.New().String()),
		Slug:   req.Slug,
		Name:   req.Name,
		Domain: req.Domain,
		Plan:   req.Plan,
		Status: "active",
		Settings: models.OrgSettings{
			AllowSignup:    true,
			RequireMFA:     false,
			DefaultRole:    models.RoleMember,
			SessionTimeout: 60,
		},
		Limits: limits,
		BillingInfo: models.BillingInfo{
			BillingEmail: req.OwnerEmail,
		},
	}

	// Set trial end date for paid plans
	if req.Plan != models.PlanFree {
		org.TrialEndsAt = time.Now().AddDate(0, 0, 14) // 14-day trial
	}

	// Save organization
	if err := h.db.Model(org).Create(); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			http.Error(w, "organization with this slug already exists", http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Create owner user
	ownerID := uuid.New().String()
	owner := &models.User{
		ID:       fmt.Sprintf("org#%s#user#%s", strings.TrimPrefix(org.ID, "org#"), ownerID),
		OrgID:    org.ID,
		UserID:   fmt.Sprintf("user#%s", ownerID),
		Email:    req.OwnerEmail,
		OrgEmail: fmt.Sprintf("%s#%s", org.ID, req.OwnerEmail),
		Role:     models.RoleOwner,
		Status:   "active",
		Permissions: []string{
			"org:manage",
			"users:manage",
			"projects:manage",
			"billing:manage",
		},
	}

	if err := h.db.Model(owner).Create(); err != nil {
		// Rollback organization creation
		h.db.Model(org).Delete()
		http.Error(w, fmt.Sprintf("failed to create owner user: %v", err), http.StatusInternalServerError)
		return
	}

	// Update organization with owner ID
	org.OwnerID = owner.UserID
	org.UserCount = 1
	if err := h.db.Model(org).Update(); err != nil {
		http.Error(w, "failed to update organization", http.StatusInternalServerError)
		return
	}

	// Log audit event
	h.logAuditEvent(org.ID, owner.UserID, "create", "organization", org.ID, nil, true, "")

	// Return created organization
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(org)
}

// GetOrganization retrieves an organization by ID
func (h *OrganizationHandler) GetOrganization(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	orgID := vars["org_id"]

	// Ensure proper org ID format
	if !strings.HasPrefix(orgID, "org#") {
		orgID = fmt.Sprintf("org#%s", orgID)
	}

	var org models.Organization
	if err := h.db.Model(&models.Organization{}).
		Where("ID", "=", orgID).
		First(&org); err != nil {
		if err == derrors.ErrItemNotFound {
			http.Error(w, "organization not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(org)
}

// UpdateOrganizationSettings updates organization settings
func (h *OrganizationHandler) UpdateOrganizationSettings(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	orgID := vars["org_id"]
	userID := r.Context().Value("user_id").(string)

	// Ensure proper org ID format
	if !strings.HasPrefix(orgID, "org#") {
		orgID = fmt.Sprintf("org#%s", orgID)
	}

	var settings models.OrgSettings
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Get current organization
	var org models.Organization
	if err := h.db.Model(&models.Organization{}).
		Where("ID", "=", orgID).
		First(&org); err != nil {
		if err == derrors.ErrItemNotFound {
			http.Error(w, "organization not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Track changes for audit log
	changes := make(map[string]models.Change)
	if settings.RequireMFA != org.Settings.RequireMFA {
		changes["require_mfa"] = models.Change{
			From: org.Settings.RequireMFA,
			To:   settings.RequireMFA,
		}
	}

	// Update settings
	org.Settings = settings
	if err := h.db.Model(&org).Update(); err != nil {
		http.Error(w, "failed to update organization", http.StatusInternalServerError)
		return
	}

	// Log audit event
	h.logAuditEvent(orgID, userID, "update", "organization_settings", orgID, changes, true, "")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(org)
}

// ListOrganizations lists organizations (admin only)
func (h *OrganizationHandler) ListOrganizations(w http.ResponseWriter, r *http.Request) {
	// This would typically be admin-only and include pagination
	var orgs []models.Organization

	query := h.db.Model(&models.Organization{})

	// Add filters based on query parameters
	if status := r.URL.Query().Get("status"); status != "" {
		query = query.Where("Status", "=", status)
	}

	if plan := r.URL.Query().Get("plan"); plan != "" {
		query = query.Where("Plan", "=", plan)
	}

	if err := query.All(&orgs); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(orgs)
}

// getPlanLimits returns the limits for a given plan
func getPlanLimits(plan string) models.PlanLimits {
	switch plan {
	case models.PlanFree:
		return models.PlanLimits{
			MaxUsers:       3,
			MaxProjects:    1,
			MaxStorage:     1 * 1024 * 1024 * 1024, // 1GB
			MaxAPIRequests: 10000,
			MaxComputeTime: 100,
			CustomDomain:   false,
			SSO:            false,
			AuditLog:       false,
			Support:        "community",
		}
	case models.PlanStarter:
		return models.PlanLimits{
			MaxUsers:       10,
			MaxProjects:    5,
			MaxStorage:     10 * 1024 * 1024 * 1024, // 10GB
			MaxAPIRequests: 100000,
			MaxComputeTime: 1000,
			CustomDomain:   false,
			SSO:            false,
			AuditLog:       true,
			Support:        "email",
		}
	case models.PlanPro:
		return models.PlanLimits{
			MaxUsers:       50,
			MaxProjects:    20,
			MaxStorage:     100 * 1024 * 1024 * 1024, // 100GB
			MaxAPIRequests: 1000000,
			MaxComputeTime: 10000,
			CustomDomain:   true,
			SSO:            true,
			AuditLog:       true,
			Support:        "priority",
		}
	case models.PlanEnterprise:
		return models.PlanLimits{
			MaxUsers:       -1, // unlimited
			MaxProjects:    -1,
			MaxStorage:     -1,
			MaxAPIRequests: -1,
			MaxComputeTime: -1,
			CustomDomain:   true,
			SSO:            true,
			AuditLog:       true,
			Support:        "dedicated",
		}
	default:
		return getPlanLimits(models.PlanFree)
	}
}

// logAuditEvent creates an audit log entry
func (h *OrganizationHandler) logAuditEvent(orgID, userID, action, resourceType, resourceID string, changes map[string]models.Change, success bool, errorMsg string) {
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
