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

// ProjectHandler handles project-related requests
type ProjectHandler struct {
	db core.ExtendedDB
}

// NewProjectHandler creates a new project handler
func NewProjectHandler(db core.ExtendedDB) *ProjectHandler {
	return &ProjectHandler{db: db}
}

// CreateProject creates a new project within an organization
func (h *ProjectHandler) CreateProject(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	orgID := vars["org_id"]
	userID := r.Context().Value("user_id").(string)

	// Ensure proper org ID format
	if !strings.HasPrefix(orgID, "org#") {
		orgID = fmt.Sprintf("org#%s", orgID)
	}

	var req struct {
		Name        string                 `json:"name"`
		Slug        string                 `json:"slug"`
		Description string                 `json:"description"`
		Type        string                 `json:"type"`
		Environment string                 `json:"environment"`
		Team        []models.TeamMember    `json:"team"`
		Settings    models.ProjectSettings `json:"settings"`
		Tags        []string               `json:"tags"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.Name == "" || req.Slug == "" {
		http.Error(w, "name and slug are required", http.StatusBadRequest)
		return
	}

	// Check project limit
	if err := h.checkProjectLimit(orgID); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	// Create project
	projectID := uuid.New().String()
	project := &models.Project{
		ID:          fmt.Sprintf("%s#project#%s", orgID, projectID),
		OrgID:       orgID,
		ProjectID:   fmt.Sprintf("project#%s", projectID),
		Name:        req.Name,
		ProjectName: req.Name, // For GSI
		Slug:        req.Slug,
		Description: req.Description,
		Type:        req.Type,
		Status:      "active",
		Environment: req.Environment,
		Settings:    req.Settings,
		Team:        req.Team,
		Tags:        req.Tags,
		CreatedBy:   userID,
		Resources: models.ResourceQuota{
			CPULimit:     1000,                    // 1 CPU
			MemoryLimit:  2048,                    // 2GB
			StorageLimit: 10 * 1024 * 1024 * 1024, // 10GB
		},
	}

	// Default environment if not specified
	if project.Environment == "" {
		project.Environment = "development"
	}

	// Add creator to team if not already present
	creatorInTeam := false
	for _, member := range project.Team {
		if member.UserID == userID {
			creatorInTeam = true
			break
		}
	}
	if !creatorInTeam {
		project.Team = append(project.Team, models.TeamMember{
			UserID:  userID,
			Role:    models.ProjectRoleLead,
			AddedAt: time.Now(),
			AddedBy: userID,
		})
	}

	// Save project
	if err := h.db.Model(project).Create(); err != nil {
		http.Error(w, "failed to create project", http.StatusInternalServerError)
		return
	}

	// Update organization project count
	h.updateOrgProjectCount(orgID, 1)

	// Add project to creator's project list
	h.addProjectToUser(orgID, userID, project.ProjectID)

	// Log audit event
	h.logAuditEvent(orgID, userID, "create", "project", project.ProjectID, nil, true, "")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(project)
}

// ListProjects lists all projects in an organization
func (h *ProjectHandler) ListProjects(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	orgID := vars["org_id"]
	userID := r.Context().Value("user_id").(string)

	// Ensure proper org ID format
	if !strings.HasPrefix(orgID, "org#") {
		orgID = fmt.Sprintf("org#%s", orgID)
	}

	// Get user to check their role and project access
	var user models.User
	if err := h.db.Model(&models.User{}).
		Where("ID", "=", fmt.Sprintf("%s#%s", orgID, userID)).
		First(&user); err != nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	// Build query
	query := h.db.Model(&models.Project{}).Where("OrgID", "=", orgID)

	// Apply filters
	if status := r.URL.Query().Get("status"); status != "" {
		query = query.Where("Status", "=", status)
	}

	if env := r.URL.Query().Get("environment"); env != "" {
		query = query.Where("Environment", "=", env)
	}

	if projectType := r.URL.Query().Get("type"); projectType != "" {
		query = query.Where("Type", "=", projectType)
	}

	var projects []models.Project
	if err := query.All(&projects); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Filter projects based on user access if not admin/owner
	if user.Role == models.RoleMember || user.Role == models.RoleViewer {
		filteredProjects := []models.Project{}
		for _, project := range projects {
			// Check if user has access to this project
			for _, projectID := range user.Projects {
				if project.ProjectID == projectID {
					filteredProjects = append(filteredProjects, project)
					break
				}
			}
		}
		projects = filteredProjects
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(projects)
}

// GetProject retrieves a specific project
func (h *ProjectHandler) GetProject(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	orgID := vars["org_id"]
	projectID := vars["project_id"]

	// Ensure proper ID formats
	if !strings.HasPrefix(orgID, "org#") {
		orgID = fmt.Sprintf("org#%s", orgID)
	}
	if !strings.HasPrefix(projectID, "project#") {
		projectID = fmt.Sprintf("project#%s", projectID)
	}

	var project models.Project
	compositeID := fmt.Sprintf("%s#%s", orgID, projectID)

	if err := h.db.Model(&models.Project{}).
		Where("ID", "=", compositeID).
		First(&project); err != nil {
		if err == derrors.ErrItemNotFound {
			http.Error(w, "project not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Include resource usage if requested
	if r.URL.Query().Get("include") == "resources" {
		h.includeResourceUsage(&project)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(project)
}

// UpdateProject updates a project
func (h *ProjectHandler) UpdateProject(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	orgID := vars["org_id"]
	projectID := vars["project_id"]
	userID := r.Context().Value("user_id").(string)

	// Ensure proper ID formats
	if !strings.HasPrefix(orgID, "org#") {
		orgID = fmt.Sprintf("org#%s", orgID)
	}
	if !strings.HasPrefix(projectID, "project#") {
		projectID = fmt.Sprintf("project#%s", projectID)
	}

	var req struct {
		Name        string                 `json:"name"`
		Description string                 `json:"description"`
		Status      string                 `json:"status"`
		Settings    models.ProjectSettings `json:"settings"`
		Team        []models.TeamMember    `json:"team"`
		Tags        []string               `json:"tags"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Get current project
	var project models.Project
	compositeID := fmt.Sprintf("%s#%s", orgID, projectID)

	if err := h.db.Model(&models.Project{}).
		Where("ID", "=", compositeID).
		First(&project); err != nil {
		if err == derrors.ErrItemNotFound {
			http.Error(w, "project not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Track changes for audit
	changes := make(map[string]models.Change)

	// Update fields if provided
	if req.Name != "" && req.Name != project.Name {
		changes["name"] = models.Change{From: project.Name, To: req.Name}
		project.Name = req.Name
		project.ProjectName = req.Name // Update GSI field
	}

	if req.Description != project.Description {
		changes["description"] = models.Change{From: project.Description, To: req.Description}
		project.Description = req.Description
	}

	if req.Status != "" && req.Status != project.Status {
		changes["status"] = models.Change{From: project.Status, To: req.Status}
		project.Status = req.Status
	}

	if req.Team != nil {
		changes["team"] = models.Change{From: len(project.Team), To: len(req.Team)}
		project.Team = req.Team
		// Update user project associations
		h.updateProjectTeamAssociations(orgID, projectID, req.Team)
	}

	if req.Tags != nil {
		changes["tags"] = models.Change{From: project.Tags, To: req.Tags}
		project.Tags = req.Tags
	}

	// Update project
	if err := h.db.Model(&project).Update(); err != nil {
		http.Error(w, "failed to update project", http.StatusInternalServerError)
		return
	}

	// Log audit event
	h.logAuditEvent(orgID, userID, "update", "project", projectID, changes, true, "")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(project)
}

// DeleteProject archives a project
func (h *ProjectHandler) DeleteProject(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	orgID := vars["org_id"]
	projectID := vars["project_id"]
	userID := r.Context().Value("user_id").(string)

	// Ensure proper ID formats
	if !strings.HasPrefix(orgID, "org#") {
		orgID = fmt.Sprintf("org#%s", orgID)
	}
	if !strings.HasPrefix(projectID, "project#") {
		projectID = fmt.Sprintf("project#%s", projectID)
	}

	// Get project
	var project models.Project
	compositeID := fmt.Sprintf("%s#%s", orgID, projectID)

	if err := h.db.Model(&models.Project{}).
		Where("ID", "=", compositeID).
		First(&project); err != nil {
		if err == derrors.ErrItemNotFound {
			http.Error(w, "project not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Archive instead of hard delete
	project.Status = "archived"
	if err := h.db.Model(&project).Update(); err != nil {
		http.Error(w, "failed to archive project", http.StatusInternalServerError)
		return
	}

	// Update organization project count
	h.updateOrgProjectCount(orgID, -1)

	// Remove project from all users
	h.removeProjectFromAllUsers(orgID, projectID)

	// Log audit event
	h.logAuditEvent(orgID, userID, "archive", "project", projectID, nil, true, "")

	w.WriteHeader(http.StatusNoContent)
}

// Helper functions

func (h *ProjectHandler) checkProjectLimit(orgID string) error {
	var org models.Organization
	if err := h.db.Model(&models.Organization{}).
		Where("ID", "=", orgID).
		First(&org); err != nil {
		return fmt.Errorf("failed to get organization")
	}

	if org.Limits.MaxProjects > 0 && org.ProjectCount >= org.Limits.MaxProjects {
		return fmt.Errorf("organization has reached project limit (%d)", org.Limits.MaxProjects)
	}

	return nil
}

func (h *ProjectHandler) updateOrgProjectCount(orgID string, delta int) {
	var org models.Organization
	if err := h.db.Model(&models.Organization{}).
		Where("ID", "=", orgID).
		First(&org); err != nil {
		return
	}

	org.ProjectCount += delta
	h.db.Model(&org).Update()
}

func (h *ProjectHandler) addProjectToUser(orgID, userID, projectID string) {
	var user models.User
	if err := h.db.Model(&models.User{}).
		Where("ID", "=", fmt.Sprintf("%s#%s", orgID, userID)).
		First(&user); err != nil {
		return
	}

	// Add project if not already present
	found := false
	for _, pid := range user.Projects {
		if pid == projectID {
			found = true
			break
		}
	}
	if !found {
		user.Projects = append(user.Projects, projectID)
		h.db.Model(&user).Update()
	}
}

func (h *ProjectHandler) updateProjectTeamAssociations(orgID, projectID string, team []models.TeamMember) {
	// Get all users in the organization
	var users []models.User
	h.db.Model(&models.User{}).Where("OrgID", "=", orgID).All(&users)

	// Update each user's project list based on team membership
	for _, user := range users {
		inTeam := false
		for _, member := range team {
			if member.UserID == user.UserID {
				inTeam = true
				break
			}
		}

		// Update user's project list
		newProjects := []string{}
		for _, pid := range user.Projects {
			if pid != projectID {
				newProjects = append(newProjects, pid)
			}
		}
		if inTeam {
			newProjects = append(newProjects, projectID)
		}

		if len(newProjects) != len(user.Projects) {
			user.Projects = newProjects
			h.db.Model(&user).Update()
		}
	}
}

func (h *ProjectHandler) removeProjectFromAllUsers(orgID, projectID string) {
	var users []models.User
	h.db.Model(&models.User{}).Where("OrgID", "=", orgID).All(&users)

	for _, user := range users {
		newProjects := []string{}
		for _, pid := range user.Projects {
			if pid != projectID {
				newProjects = append(newProjects, pid)
			}
		}
		if len(newProjects) != len(user.Projects) {
			user.Projects = newProjects
			h.db.Model(&user).Update()
		}
	}
}

func (h *ProjectHandler) includeResourceUsage(project *models.Project) {
	// Calculate resource usage from resource records
	startOfMonth := time.Now().Truncate(24*time.Hour).AddDate(0, 0, -time.Now().Day()+1)

	var resources []models.Resource
	h.db.Model(&models.Resource{}).
		Where("ProjectID", "=", project.ProjectID).
		Where("Timestamp", ">=", startOfMonth).
		All(&resources)

	// Aggregate usage
	for _, resource := range resources {
		switch resource.Type {
		case models.ResourceTypeCompute:
			project.Resources.CPUUsed += int(resource.Quantity)
		case models.ResourceTypeStorage:
			project.Resources.StorageUsed = int64(resource.Quantity)
		case models.ResourceTypeBandwidth:
			project.Resources.BandwidthUsed += int64(resource.Quantity)
		}
	}
}

func (h *ProjectHandler) logAuditEvent(orgID, userID, action, resourceType, resourceID string, changes map[string]models.Change, success bool, errorMsg string) {
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
