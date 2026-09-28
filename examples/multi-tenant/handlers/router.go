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
	r.HandleFunc("/organizations/{org_id}", orgHandler.GetOrganization).Methods("GET")
	r.HandleFunc("/organizations/{org_id}/settings", orgHandler.UpdateOrganizationSettings).Methods("PUT")

	// User routes
	r.HandleFunc("/organizations/{org_id}/invitations", userHandler.InviteUser).Methods("POST")
	r.HandleFunc("/invitations/accept", userHandler.AcceptInvitation).Methods("POST")
	r.HandleFunc("/organizations/{org_id}/users", userHandler.ListUsers).Methods("GET")
	r.HandleFunc("/organizations/{org_id}/users/{user_id}", userHandler.GetUser).Methods("GET")
	r.HandleFunc("/organizations/{org_id}/users/{user_id}", userHandler.UpdateUser).Methods("PUT")
	r.HandleFunc("/organizations/{org_id}/users/{user_id}", userHandler.DeleteUser).Methods("DELETE")

	// Project routes
	r.HandleFunc("/organizations/{org_id}/projects", projectHandler.CreateProject).Methods("POST")
	r.HandleFunc("/organizations/{org_id}/projects", projectHandler.ListProjects).Methods("GET")
	r.HandleFunc("/organizations/{org_id}/projects/{project_id}", projectHandler.GetProject).Methods("GET")
	r.HandleFunc("/organizations/{org_id}/projects/{project_id}", projectHandler.UpdateProject).Methods("PUT")
	r.HandleFunc("/organizations/{org_id}/projects/{project_id}", projectHandler.DeleteProject).Methods("DELETE")

	// Resource tracking routes
	r.HandleFunc("/organizations/{org_id}/resources", resourceHandler.RecordUsage).Methods("POST")
	r.HandleFunc("/organizations/{org_id}/usage", resourceHandler.GetUsageReport).Methods("GET")
	r.HandleFunc("/organizations/{org_id}/projects/{project_id}/usage", resourceHandler.GetProjectUsage).Methods("GET")

	// API key routes
	r.HandleFunc("/organizations/{org_id}/api-keys", apiKeyHandler.CreateAPIKey).Methods("POST")
	r.HandleFunc("/organizations/{org_id}/api-keys", apiKeyHandler.ListAPIKeys).Methods("GET")
	r.HandleFunc("/organizations/{org_id}/api-keys/{key_id}", apiKeyHandler.GetAPIKey).Methods("GET")
	r.HandleFunc("/organizations/{org_id}/api-keys/{key_id}", apiKeyHandler.UpdateAPIKey).Methods("PUT")
	r.HandleFunc("/organizations/{org_id}/api-keys/{key_id}", apiKeyHandler.DeleteAPIKey).Methods("DELETE")

	return r
}
