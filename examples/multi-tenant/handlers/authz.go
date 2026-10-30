package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/theory-cloud/tabletheory/v6/examples/multi-tenant/models"
	"github.com/theory-cloud/tabletheory/v6/pkg/core"
)

// callerOrgUser loads the authenticated caller's user row in the organization
// its token is bound to. Every user route resolves its principal here, so an
// invited, suspended, deleted, or missing user is refused before a handler
// reads tenant data. The organization and user IDs come from the trusted
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
	if user.Status != models.UserStatusActive {
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

// requireActivePrincipal is the gate every organization-scoped route passes
// through before its handler runs. It resolves the principal from the trusted
// entrypoint context — an active stored user for user routes, or an active
// stored API key for key routes — and refuses anything else. A request carrying
// a user identity is evaluated only as that user, so a suspended user cannot
// fall back to an API key attached to the same request.
func requireActivePrincipal(db core.ExtendedDB, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := userIDFromContext(r.Context()); ok {
			if _, _, ok := callerOrgUser(db, r.Context()); !ok {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next(w, r)
			return
		}
		if _, ok := apiKeyIDFromContext(r.Context()); ok {
			if _, ok := callerAPIKey(db, r.Context()); !ok {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next(w, r)
			return
		}
		http.Error(w, "forbidden", http.StatusForbidden)
	}
}

// canReadDirectoryEntry reports whether the caller may read a user directory
// entry: owners and admins may read every entry; every other active user may
// read only their own.
func canReadDirectoryEntry(user *models.User, userID, targetUserID string) bool {
	if user == nil {
		return false
	}
	if user.Role == models.RoleOwner || user.Role == models.RoleAdmin {
		return true
	}
	return userID == targetUserID
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

// callerAPIKey loads the authenticated caller's API key row in the organization
// its request is bound to. A key that is missing, revoked, expired, or bound to
// another organization is reported as unauthorized, so callers never act on a
// credential the store no longer honors.
func callerAPIKey(db core.ExtendedDB, ctx context.Context) (*models.APIKey, bool) {
	orgID, ok := orgIDFromContext(ctx)
	if !ok {
		return nil, false
	}
	keyID, ok := apiKeyIDFromContext(ctx)
	if !ok {
		return nil, false
	}

	var key models.APIKey
	if err := db.Model(&models.APIKey{}).
		Where("ID", "=", fmt.Sprintf("%s#%s", orgID, keyID)).
		First(&key); err != nil {
		return nil, false
	}
	if !key.Active {
		return nil, false
	}
	if key.OrgID != "" && key.OrgID != orgID {
		return nil, false
	}
	if !key.ExpiresAt.IsZero() && time.Now().After(key.ExpiresAt) {
		return nil, false
	}
	return &key, true
}

// apiKeyAllowsProject reports whether an API key may act on a project. A key
// with no project binding is organization-wide; a project-bound key is limited
// to that project. A missing, revoked, or expired key allows nothing.
func apiKeyAllowsProject(db core.ExtendedDB, ctx context.Context, projectID string) bool {
	key, ok := callerAPIKey(db, ctx)
	if !ok {
		return false
	}
	return key.ProjectID == "" || key.ProjectID == projectID
}

// callerCanReadProject reports whether the caller may read the given project.
// User callers are checked against their organization role and project
// assignments; API-key callers against the key's project binding. A request
// that carries a user identity is evaluated only as that user, so a suspended
// user cannot borrow an API key attached to the same request.
func callerCanReadProject(db core.ExtendedDB, ctx context.Context, projectID string) bool {
	if _, ok := userIDFromContext(ctx); ok {
		user, _, ok := callerOrgUser(db, ctx)
		return ok && canReadProject(user, projectID)
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
