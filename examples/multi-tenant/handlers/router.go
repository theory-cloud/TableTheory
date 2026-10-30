package handlers

import (
	"net/http"

	"github.com/gorilla/mux"

	"github.com/theory-cloud/tabletheory/v6/pkg/core"
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

	// protected composes the two gates every organization-scoped route needs:
	// the URL organization must match the caller's trusted binding, and the
	// caller must resolve to an active stored principal.
	protected := func(next http.HandlerFunc) http.HandlerFunc {
		return requireOrgBinding(requireActivePrincipal(db, next))
	}

	// Health check
	r.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}).Methods("GET")

	// Organization routes. CreateOrganization is the public signup flow and
	// ListOrganizations requires an active stored user but is not bound to one
	// organization URL.
	r.HandleFunc("/organizations", orgHandler.CreateOrganization).Methods("POST")
	r.HandleFunc("/organizations", orgHandler.ListOrganizations).Methods("GET")
	r.HandleFunc("/organizations/{org_id}", protected(orgHandler.GetOrganization)).Methods("GET")
	r.HandleFunc("/organizations/{org_id}/settings", protected(orgHandler.UpdateOrganizationSettings)).Methods("PUT")

	// User routes. Invitation acceptance is a public token exchange.
	r.HandleFunc("/organizations/{org_id}/invitations", protected(userHandler.InviteUser)).Methods("POST")
	r.HandleFunc("/invitations/accept", userHandler.AcceptInvitation).Methods("POST")
	r.HandleFunc("/organizations/{org_id}/users", protected(userHandler.ListUsers)).Methods("GET")
	r.HandleFunc("/organizations/{org_id}/users/{user_id}", protected(userHandler.GetUser)).Methods("GET")
	r.HandleFunc("/organizations/{org_id}/users/{user_id}", protected(userHandler.UpdateUser)).Methods("PUT")
	r.HandleFunc("/organizations/{org_id}/users/{user_id}", protected(userHandler.DeleteUser)).Methods("DELETE")

	// Project routes
	r.HandleFunc("/organizations/{org_id}/projects", protected(projectHandler.CreateProject)).Methods("POST")
	r.HandleFunc("/organizations/{org_id}/projects", protected(projectHandler.ListProjects)).Methods("GET")
	r.HandleFunc("/organizations/{org_id}/projects/{project_id}", protected(projectHandler.GetProject)).Methods("GET")
	r.HandleFunc("/organizations/{org_id}/projects/{project_id}", protected(projectHandler.UpdateProject)).Methods("PUT")
	r.HandleFunc("/organizations/{org_id}/projects/{project_id}", protected(projectHandler.DeleteProject)).Methods("DELETE")

	// Resource tracking routes. Usage recording is API-key authenticated; the
	// other two are user routes.
	r.HandleFunc("/organizations/{org_id}/resources", protected(resourceHandler.RecordUsage)).Methods("POST")
	r.HandleFunc("/organizations/{org_id}/usage", protected(resourceHandler.GetUsageReport)).Methods("GET")
	r.HandleFunc("/organizations/{org_id}/projects/{project_id}/usage", protected(resourceHandler.GetProjectUsage)).Methods("GET")

	// API key routes
	r.HandleFunc("/organizations/{org_id}/api-keys", protected(apiKeyHandler.CreateAPIKey)).Methods("POST")
	r.HandleFunc("/organizations/{org_id}/api-keys", protected(apiKeyHandler.ListAPIKeys)).Methods("GET")
	r.HandleFunc("/organizations/{org_id}/api-keys/{key_id}", protected(apiKeyHandler.GetAPIKey)).Methods("GET")
	r.HandleFunc("/organizations/{org_id}/api-keys/{key_id}", protected(apiKeyHandler.UpdateAPIKey)).Methods("PUT")
	r.HandleFunc("/organizations/{org_id}/api-keys/{key_id}", protected(apiKeyHandler.DeleteAPIKey)).Methods("DELETE")

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
