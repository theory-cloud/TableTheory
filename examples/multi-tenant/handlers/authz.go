package handlers

import (
	"context"
	"fmt"
	"net/http"

	"github.com/theory-cloud/tabletheory/v6/examples/multi-tenant/models"
	"github.com/theory-cloud/tabletheory/v6/pkg/core"
)

// callerOrgUser loads the authenticated caller's user row in the organization
// its token is bound to. The organization and user IDs come from the trusted
// request context, never from the request body or URL.
func callerOrgUser(db core.ExtendedDB, ctx context.Context) (*models.User, string, bool) {
	orgID, ok := orgIDFromContext(ctx)
	if !ok {
		return nil, "", false
	}
	userID, ok := userIDFromContext(ctx)
	if !ok {
		return nil, "", false
	}

	var user models.User
	if err := db.Model(&models.User{}).
		Where("ID", "=", fmt.Sprintf("%s#%s", orgID, userID)).
		First(&user); err != nil {
		return nil, "", false
	}
	return &user, userID, true
}

// requireOrganizationRole authorizes the caller against the organization role
// carried by the stored user row. It writes the refusal itself, so callers only
// need to return when false is reported.
func requireOrganizationRole(w http.ResponseWriter, db core.ExtendedDB, r *http.Request, roles ...string) (*models.User, string, bool) {
	user, userID, ok := callerOrgUser(db, r.Context())
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil, "", false
	}
	for _, role := range roles {
		if user.Role == role {
			return user, userID, true
		}
	}
	http.Error(w, "forbidden", http.StatusForbidden)
	return nil, "", false
}

// canReadProject reports whether the caller may read a project. Owners and
// admins may read every project in their organization; members and viewers only
// projects assigned to them.
func canReadProject(user *models.User, projectID string) bool {
	if user.Role == models.RoleOwner || user.Role == models.RoleAdmin {
		return true
	}
	for _, assigned := range user.Projects {
		if assigned == projectID {
			return true
		}
	}
	return false
}

// canManageProject reports whether the caller may change a project. Owners and
// admins may change every project; a project lead may change an assigned
// project. Developers and viewers cannot change project settings.
func canManageProject(user *models.User, userID string, project *models.Project) bool {
	if user.Role == models.RoleOwner || user.Role == models.RoleAdmin {
		return true
	}
	if !canReadProject(user, project.ProjectID) {
		return false
	}
	for _, member := range project.Team {
		if member.UserID == userID && member.Role == models.ProjectRoleLead {
			return true
		}
	}
	return false
}

// apiKeyIDFromContext reads the validated API key ID attached by the
// entrypoint's API-key authentication path.
func apiKeyIDFromContext(ctx context.Context) (string, bool) {
	value, ok := ctx.Value("api_key_id").(string)
	if !ok || value == "" {
		return "", false
	}
	return value, true
}

// apiKeyAllowsProject reports whether an API key may act on a project. A key
// with no project binding is organization-wide; a project-bound key is limited
// to that project.
func apiKeyAllowsProject(db core.ExtendedDB, ctx context.Context, projectID string) bool {
	orgID, ok := orgIDFromContext(ctx)
	if !ok {
		return false
	}
	keyID, ok := apiKeyIDFromContext(ctx)
	if !ok {
		return false
	}

	var key models.APIKey
	if err := db.Model(&models.APIKey{}).
		Where("ID", "=", fmt.Sprintf("%s#%s", orgID, keyID)).
		First(&key); err != nil {
		return false
	}
	return key.ProjectID == "" || key.ProjectID == projectID
}

// callerCanReadProject reports whether the caller may read the given project.
// User callers are checked against their organization role and project
// assignments; API-key callers against the key's project binding.
func callerCanReadProject(db core.ExtendedDB, ctx context.Context, projectID string) bool {
	if user, _, ok := callerOrgUser(db, ctx); ok {
		return canReadProject(user, projectID)
	}
	return apiKeyAllowsProject(db, ctx, projectID)
}

// callerCanManageProject reports whether the caller may change the project.
func callerCanManageProject(db core.ExtendedDB, ctx context.Context, project *models.Project) bool {
	user, userID, ok := callerOrgUser(db, ctx)
	if !ok {
		return false
	}
	return canManageProject(user, userID, project)
}
