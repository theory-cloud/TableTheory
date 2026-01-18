package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"golang.org/x/crypto/bcrypt"

	"github.com/theory-cloud/tabletheory/examples/multi-tenant/models"
	"github.com/theory-cloud/tabletheory/pkg/core"
	derrors "github.com/theory-cloud/tabletheory/pkg/errors"
)

// UserHandler handles user-related requests
type UserHandler struct {
	db core.ExtendedDB
}

// NewUserHandler creates a new user handler
func NewUserHandler(db core.ExtendedDB) *UserHandler {
	return &UserHandler{db: db}
}

// InviteUser sends an invitation to a new user
func (h *UserHandler) InviteUser(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	orgID := vars["org_id"]
	inviterID := r.Context().Value("user_id").(string)

	// Ensure proper org ID format
	if !strings.HasPrefix(orgID, "org#") {
		orgID = fmt.Sprintf("org#%s", orgID)
	}

	var req struct {
		Email    string   `json:"email"`
		Role     string   `json:"role"`
		Projects []string `json:"projects,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate email and role
	if req.Email == "" {
		http.Error(w, "email is required", http.StatusBadRequest)
		return
	}
	if req.Role == "" {
		req.Role = models.RoleMember
	}

	// Check if user already exists in org
	var existingUser models.User
	err := h.db.Model(&models.User{}).
		Where("OrgEmail", "=", fmt.Sprintf("%s#%s", orgID, req.Email)).
		First(&existingUser)

	if err == nil {
		http.Error(w, "user already exists in organization", http.StatusConflict)
		return
	}

	// Check organization user limit
	if err := h.checkUserLimit(orgID); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	// Create invitation
	inviteID := uuid.New().String()
	inviteToken := uuid.New().String()

	invitation := &models.Invitation{
		ID:        fmt.Sprintf("%s#invite#%s", orgID, inviteID),
		OrgID:     orgID,
		InviteID:  inviteID,
		Email:     req.Email,
		Role:      req.Role,
		Projects:  req.Projects,
		InvitedBy: inviterID,
		Token:     inviteToken,
		ExpiresAt: time.Now().AddDate(0, 0, 7).Unix(), // 7 days
	}

	if err := h.db.Model(invitation).Create(); err != nil {
		http.Error(w, "failed to create invitation", http.StatusInternalServerError)
		return
	}

	// Log audit event
	h.logAuditEvent(orgID, inviterID, "invite", "user", req.Email, nil, true, "")

	// In production, send invitation email here
	// sendInvitationEmail(req.Email, inviteToken)

	response := map[string]any{
		"invitation_id": inviteID,
		"email":         req.Email,
		"expires_at":    invitation.ExpiresAt,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ListUsers lists all users in an organization
func (h *UserHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	orgID := vars["org_id"]

	// Ensure proper org ID format
	if !strings.HasPrefix(orgID, "org#") {
		orgID = fmt.Sprintf("org#%s", orgID)
	}

	// Build query
	query := h.db.Model(&models.User{}).Where("OrgID", "=", orgID)

	// Apply filters
	if role := r.URL.Query().Get("role"); role != "" {
		query = query.Where("Role", "=", role)
	}

	if status := r.URL.Query().Get("status"); status != "" {
		query = query.Where("Status", "=", status)
	}

	if projectID := r.URL.Query().Get("project_id"); projectID != "" {
		query = query.Where("Projects", "contains", projectID)
	}

	var users []models.User
	if err := query.All(&users); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(users)
}

// GetUser retrieves a specific user
func (h *UserHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	orgID := vars["org_id"]
	userID := vars["user_id"]

	// Ensure proper ID formats
	if !strings.HasPrefix(orgID, "org#") {
		orgID = fmt.Sprintf("org#%s", orgID)
	}
	if !strings.HasPrefix(userID, "user#") {
		userID = fmt.Sprintf("user#%s", userID)
	}

	var user models.User
	compositeID := fmt.Sprintf("%s#%s", orgID, userID)

	if err := h.db.Model(&models.User{}).
		Where("ID", "=", compositeID).
		First(&user); err != nil {
		if err == derrors.ErrItemNotFound {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

// UpdateUser updates a user's role and projects
func (h *UserHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	orgID := vars["org_id"]
	userID := vars["user_id"]
	updaterID := r.Context().Value("user_id").(string)

	// Ensure proper ID formats
	if !strings.HasPrefix(orgID, "org#") {
		orgID = fmt.Sprintf("org#%s", orgID)
	}
	if !strings.HasPrefix(userID, "user#") {
		userID = fmt.Sprintf("user#%s", userID)
	}

	var req struct {
		Role     string   `json:"role"`
		Projects []string `json:"projects"`
		Status   string   `json:"status"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Get current user
	var user models.User
	compositeID := fmt.Sprintf("%s#%s", orgID, userID)

	if err := h.db.Model(&models.User{}).
		Where("ID", "=", compositeID).
		First(&user); err != nil {
		if err == derrors.ErrItemNotFound {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Track changes for audit
	changes := make(map[string]models.Change)

	// Update fields if provided
	if req.Role != "" && req.Role != user.Role {
		// Cannot change owner role
		if user.Role == models.RoleOwner {
			http.Error(w, "cannot change owner role", http.StatusForbidden)
			return
		}
		changes["role"] = models.Change{From: user.Role, To: req.Role}
		user.Role = req.Role
		user.Permissions = getRolePermissions(req.Role)
	}

	if req.Projects != nil {
		changes["projects"] = models.Change{From: user.Projects, To: req.Projects}
		user.Projects = req.Projects
	}

	if req.Status != "" && req.Status != user.Status {
		changes["status"] = models.Change{From: user.Status, To: req.Status}
		user.Status = req.Status
	}

	// Update user
	if err := h.db.Model(&user).Update(); err != nil {
		http.Error(w, "failed to update user", http.StatusInternalServerError)
		return
	}

	// Log audit event
	h.logAuditEvent(orgID, updaterID, "update", "user", userID, changes, true, "")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

// DeleteUser removes a user from an organization
func (h *UserHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	orgID := vars["org_id"]
	userID := vars["user_id"]
	deleterID := r.Context().Value("user_id").(string)

	// Ensure proper ID formats
	if !strings.HasPrefix(orgID, "org#") {
		orgID = fmt.Sprintf("org#%s", orgID)
	}
	if !strings.HasPrefix(userID, "user#") {
		userID = fmt.Sprintf("user#%s", userID)
	}

	// Get user to check if they're the owner
	var user models.User
	compositeID := fmt.Sprintf("%s#%s", orgID, userID)

	if err := h.db.Model(&models.User{}).
		Where("ID", "=", compositeID).
		First(&user); err != nil {
		if err == derrors.ErrItemNotFound {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Cannot delete organization owner
	if user.Role == models.RoleOwner {
		http.Error(w, "cannot delete organization owner", http.StatusForbidden)
		return
	}

	// Delete user
	if err := h.db.Model(&user).Delete(); err != nil {
		http.Error(w, "failed to delete user", http.StatusInternalServerError)
		return
	}

	// Update organization user count
	h.updateOrgUserCount(orgID, -1)

	// Log audit event
	h.logAuditEvent(orgID, deleterID, "delete", "user", userID, nil, true, "")

	w.WriteHeader(http.StatusNoContent)
}

// AcceptInvitation accepts a user invitation
func (h *UserHandler) AcceptInvitation(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token     string `json:"token"`
		Password  string `json:"password"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
		Username  string `json:"username"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Find invitation by token
	var invitation models.Invitation
	if err := h.db.Model(&models.Invitation{}).
		Where("Token", "=", req.Token).
		First(&invitation); err != nil {
		if err == derrors.ErrItemNotFound {
			http.Error(w, "invalid invitation token", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Check if invitation is expired
	if time.Now().Unix() > invitation.ExpiresAt {
		http.Error(w, "invitation has expired", http.StatusGone)
		return
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "failed to hash password", http.StatusInternalServerError)
		return
	}

	// Create user
	userID := uuid.New().String()
	user := &models.User{
		ID:          fmt.Sprintf("%s#user#%s", invitation.OrgID, userID),
		OrgID:       invitation.OrgID,
		UserID:      fmt.Sprintf("user#%s", userID),
		Email:       invitation.Email,
		OrgEmail:    fmt.Sprintf("%s#%s", invitation.OrgID, invitation.Email),
		Username:    req.Username,
		FirstName:   req.FirstName,
		LastName:    req.LastName,
		Role:        invitation.Role,
		Status:      "active",
		Projects:    invitation.Projects,
		Permissions: getRolePermissions(invitation.Role),
		InvitedBy:   invitation.InvitedBy,
	}

	// Store password hash separately in production
	_ = hashedPassword

	if err := h.db.Model(user).Create(); err != nil {
		http.Error(w, "failed to create user", http.StatusInternalServerError)
		return
	}

	// Delete invitation
	h.db.Model(&invitation).Delete()

	// Update organization user count
	h.updateOrgUserCount(invitation.OrgID, 1)

	// Log audit event
	h.logAuditEvent(invitation.OrgID, user.UserID, "accept_invitation", "user", user.UserID, nil, true, "")

	// Generate auth token (simplified - use proper JWT in production)
	token := uuid.New().String()

	response := map[string]any{
		"user":  user,
		"token": token,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Helper functions

func (h *UserHandler) checkUserLimit(orgID string) error {
	var org models.Organization
	if err := h.db.Model(&models.Organization{}).
		Where("ID", "=", orgID).
		First(&org); err != nil {
		return fmt.Errorf("failed to get organization")
	}

	if org.Limits.MaxUsers > 0 && org.UserCount >= org.Limits.MaxUsers {
		return fmt.Errorf("organization has reached user limit (%d)", org.Limits.MaxUsers)
	}

	return nil
}

func (h *UserHandler) updateOrgUserCount(orgID string, delta int) {
	var org models.Organization
	if err := h.db.Model(&models.Organization{}).
		Where("ID", "=", orgID).
		First(&org); err != nil {
		return
	}

	org.UserCount += delta
	h.db.Model(&org).Update()
}

func getRolePermissions(role string) []string {
	switch role {
	case models.RoleOwner:
		return []string{
			"org:manage",
			"users:manage",
			"projects:manage",
			"billing:manage",
			"api_keys:manage",
			"audit:view",
		}
	case models.RoleAdmin:
		return []string{
			"users:manage",
			"projects:manage",
			"api_keys:manage",
			"audit:view",
		}
	case models.RoleMember:
		return []string{
			"projects:view",
			"projects:edit",
			"api_keys:create",
		}
	case models.RoleViewer:
		return []string{
			"projects:view",
			"audit:view:own",
		}
	default:
		return []string{}
	}
}

func (h *UserHandler) logAuditEvent(orgID, userID, action, resourceType, resourceID string, changes map[string]models.Change, success bool, errorMsg string) {
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
