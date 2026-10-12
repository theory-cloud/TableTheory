package handlers

import (
	"net/http"

	"github.com/gorilla/mux"

	"github.com/theory-cloud/tabletheory/v4/pkg/core"
)

// NewRouter registers every route the example exposes on a fresh mux router.
//
// Both entrypoints serve this one table — the local dev server (cmd/local) and
// the Lambda adapter (cmd/lambda) — so local and deployed routing cannot drift
// apart. It performs no I/O, starts nothing, and leaves middleware (logging,
// in-request authentication for the local server) to the caller.
func NewRouter(db core.ExtendedDB) *mux.Router {
	orgHandler := NewOrganizationHandler(db)
	userHandler := NewUserHandler(db)
	projectHandler := NewProjectHandler(db)
	resourceHandler := NewResourceHandler(db)
	apiKeyHandler := NewAPIKeyHandler(db)

	r := mux.NewRouter()

	// Health check
	r.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}).Methods("GET")

	// Organization routes
	r.HandleFunc("/organizations", orgHandler.CreateOrganization).Methods("POST")
	r.HandleFunc("/organizations", orgHandler.ListOrganizations).Methods("GET")
	r.HandleFunc("/organizations/{org_id}", requireOrgBinding(orgHandler.GetOrganization)).Methods("GET")
	r.HandleFunc("/organizations/{org_id}/settings", requireOrgBinding(orgHandler.UpdateOrganizationSettings)).Methods("PUT")

	// User routes
	r.HandleFunc("/organizations/{org_id}/invitations", requireOrgBinding(userHandler.InviteUser)).Methods("POST")
	r.HandleFunc("/invitations/accept", userHandler.AcceptInvitation).Methods("POST")
	r.HandleFunc("/organizations/{org_id}/users", requireOrgBinding(userHandler.ListUsers)).Methods("GET")
	r.HandleFunc("/organizations/{org_id}/users/{user_id}", requireOrgBinding(userHandler.GetUser)).Methods("GET")
	r.HandleFunc("/organizations/{org_id}/users/{user_id}", requireOrgBinding(userHandler.UpdateUser)).Methods("PUT")
	r.HandleFunc("/organizations/{org_id}/users/{user_id}", requireOrgBinding(userHandler.DeleteUser)).Methods("DELETE")

	// Project routes
	r.HandleFunc("/organizations/{org_id}/projects", requireOrgBinding(projectHandler.CreateProject)).Methods("POST")
	r.HandleFunc("/organizations/{org_id}/projects", requireOrgBinding(projectHandler.ListProjects)).Methods("GET")
	r.HandleFunc("/organizations/{org_id}/projects/{project_id}", requireOrgBinding(projectHandler.GetProject)).Methods("GET")
	r.HandleFunc("/organizations/{org_id}/projects/{project_id}", requireOrgBinding(projectHandler.UpdateProject)).Methods("PUT")
	r.HandleFunc("/organizations/{org_id}/projects/{project_id}", requireOrgBinding(projectHandler.DeleteProject)).Methods("DELETE")

	// Resource tracking routes
	r.HandleFunc("/organizations/{org_id}/resources", requireOrgBinding(resourceHandler.RecordUsage)).Methods("POST")
	r.HandleFunc("/organizations/{org_id}/usage", requireOrgBinding(resourceHandler.GetUsageReport)).Methods("GET")
	r.HandleFunc("/organizations/{org_id}/projects/{project_id}/usage", requireOrgBinding(resourceHandler.GetProjectUsage)).Methods("GET")

	// API key routes
	r.HandleFunc("/organizations/{org_id}/api-keys", requireOrgBinding(apiKeyHandler.CreateAPIKey)).Methods("POST")
	r.HandleFunc("/organizations/{org_id}/api-keys", requireOrgBinding(apiKeyHandler.ListAPIKeys)).Methods("GET")
	r.HandleFunc("/organizations/{org_id}/api-keys/{key_id}", requireOrgBinding(apiKeyHandler.GetAPIKey)).Methods("GET")
	r.HandleFunc("/organizations/{org_id}/api-keys/{key_id}", requireOrgBinding(apiKeyHandler.UpdateAPIKey)).Methods("PUT")
	r.HandleFunc("/organizations/{org_id}/api-keys/{key_id}", requireOrgBinding(apiKeyHandler.DeleteAPIKey)).Methods("DELETE")

	return r
}

// requireOrgBinding rejects a request whose URL organization does not match the
// organization the caller is bound to by its token. It wraps every
// organization-scoped route so a caller holding a token for one organization
// cannot reach another organization's resources by editing the URL.
func requireOrgBinding(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bound, ok := orgIDFromContext(r.Context())
		if !ok {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if normalizeOrgID(mux.Vars(r)["org_id"]) != bound {
			// Report the other organization as missing rather than confirming it
			// exists to a caller that is not bound to it.
			http.Error(w, "organization not found", http.StatusNotFound)
			return
		}
		next(w, r)
	}
}
