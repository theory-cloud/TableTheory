package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v6"
	"github.com/theory-cloud/tabletheory/v6/examples/multi-tenant/models"
	"github.com/theory-cloud/tabletheory/v6/pkg/core"
	"github.com/theory-cloud/tabletheory/v6/pkg/session"
	"github.com/theory-cloud/tabletheory/v6/pkg/testing/fakedb"
)

// newPrincipalTestDB builds the shared route-matrix store: every model the
// protected routes touch, minus the TTL-bearing audit/invitation tables whose
// best-effort writes are not part of this matrix.
func newPrincipalTestDB(t *testing.T, seed ...any) core.ExtendedDB {
	t.Helper()

	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, fakedb.New())
	require.NoError(t, err)
	for _, model := range []any{
		&models.Organization{},
		&models.User{},
		&models.Project{},
		&models.Resource{},
		&models.APIKey{},
		&models.UsageReport{},
	} {
		require.NoError(t, db.CreateTable(model))
	}
	for _, model := range seed {
		require.NoError(t, db.Model(model).Create())
	}
	return db
}

// principalRequest builds a request for the shared router with the trusted
// tenant context an entrypoint attaches after authentication.
func principalRequest(method, path, userID, apiKeyID, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	ctx := context.WithValue(req.Context(), "org_id", testOrgID)
	if userID != "" {
		ctx = context.WithValue(ctx, "user_id", userID)
	}
	if apiKeyID != "" {
		ctx = context.WithValue(ctx, "api_key_id", apiKeyID)
	}
	return req.WithContext(ctx)
}

func principalUser(orgID, userID, role, status string) *models.User {
	return &models.User{
		ID:     orgID + "#" + userID,
		OrgID:  orgID,
		UserID: userID,
		Email:  userID + "@example.test",
		Role:   role,
		Status: status,
	}
}

func activeRoleUser(role string) *models.User {
	return principalUser(testOrgID, "user#"+role, role, models.UserStatusActive)
}

func testOrganization() *models.Organization {
	return &models.Organization{
		ID:     testOrgID,
		Slug:   "acme",
		Name:   "Acme",
		Plan:   models.PlanFree,
		Status: "active",
	}
}

func testProject(projectID, name string) *models.Project {
	return &models.Project{
		ID:        testOrgID + "#" + projectID,
		OrgID:     testOrgID,
		ProjectID: projectID,
		Name:      name,
		Status:    "active",
	}
}

func activeAPIKey(keyID, projectID string) *models.APIKey {
	return &models.APIKey{
		ID:        testOrgID + "#" + keyID,
		OrgID:     testOrgID,
		KeyID:     keyID,
		ProjectID: projectID,
		Active:    true,
		RateLimit: 1000,
	}
}

func serveRoute(router http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// TestEveryOrgRouteRequiresActiveStoredPrincipal drives every protected route
// class with invited, suspended, deleted, and missing users. Each must be
// refused before a handler can read tenant data; the active admin control
// proves the same routes still serve a valid principal.
func TestEveryOrgRouteRequiresActiveStoredPrincipal(t *testing.T) {
	activeAdmin := principalUser(testOrgID, "user#active-admin", models.RoleAdmin, models.UserStatusActive)
	invited := principalUser(testOrgID, "user#invited-admin", models.RoleAdmin, models.UserStatusInvited)
	suspended := principalUser(testOrgID, "user#suspended-admin", models.RoleAdmin, models.UserStatusSuspended)
	deleted := principalUser(testOrgID, "user#deleted-admin", models.RoleAdmin, models.UserStatusDeleted)
	db := newPrincipalTestDB(t, testOrganization(), activeAdmin, invited, suspended, deleted, testProject("project#one", "One"))
	router := NewRouter(db)

	routes := []struct {
		name       string
		method     string
		path       string
		body       string
		activeWant int
	}{
		{"get organization", http.MethodGet, "/organizations/org%23acme", "", http.StatusOK},
		{"list users", http.MethodGet, "/organizations/org%23acme/users", "", http.StatusOK},
		{"get user", http.MethodGet, "/organizations/org%23acme/users/user%23active-admin", "", http.StatusOK},
		{"list projects", http.MethodGet, "/organizations/org%23acme/projects", "", http.StatusOK},
		{"organization usage", http.MethodGet, "/organizations/org%23acme/usage", "", http.StatusOK},
		{"create project", http.MethodPost, "/organizations/org%23acme/projects", `{}`, http.StatusBadRequest},
		// Usage recording is an API-key route; an active user is still refused
		// by the handler even though the principal gate admits them.
		{"record usage", http.MethodPost, "/organizations/org%23acme/resources", `{"project_id":"project#one","type":"api_call","quantity":1}`, http.StatusForbidden},
		{"list api keys", http.MethodGet, "/organizations/org%23acme/api-keys", "", http.StatusOK},
	}

	for _, principal := range []struct {
		name   string
		userID string
	}{
		{"invited", "user#invited-admin"},
		{"suspended", "user#suspended-admin"},
		{"deleted", "user#deleted-admin"},
		{"missing", "user#ghost"},
	} {
		for _, route := range routes {
			t.Run(principal.name+"/"+route.name, func(t *testing.T) {
				rec := serveRoute(router, principalRequest(route.method, route.path, principal.userID, "", route.body))
				require.Equal(t, http.StatusForbidden, rec.Code,
					"a %s user must not reach %s", principal.name, route.name)
			})
		}
	}

	// The same routes serve an active admin, so the denial above is about the
	// principal's state rather than the route being unreachable.
	for _, route := range routes {
		t.Run("active/"+route.name, func(t *testing.T) {
			rec := serveRoute(router, principalRequest(route.method, route.path, "user#active-admin", "", route.body))
			require.Equal(t, route.activeWant, rec.Code,
				"an active admin must reach the handler for %s", route.name)
		})
	}
}

// TestDirectoryReadsAreAdminOrSelf pins the approved directory policy: owners
// and admins may enumerate, while members and viewers may read only their own
// record. It runs through the shared router so the principal gate and handler
// policy are both exercised.
func TestDirectoryReadsAreAdminOrSelf(t *testing.T) {
	admin := principalUser(testOrgID, "user#admin", models.RoleAdmin, models.UserStatusActive)
	member := principalUser(testOrgID, "user#member", models.RoleMember, models.UserStatusActive)
	viewer := principalUser(testOrgID, "user#viewer", models.RoleViewer, models.UserStatusActive)
	db := newPrincipalTestDB(t, admin, member, viewer)
	router := NewRouter(db)

	cases := []struct {
		name   string
		path   string
		userID string
		want   int
	}{
		{"admin lists users", "/organizations/org%23acme/users", "user#admin", http.StatusOK},
		{"member lists users", "/organizations/org%23acme/users", "user#member", http.StatusForbidden},
		{"viewer lists users", "/organizations/org%23acme/users", "user#viewer", http.StatusForbidden},
		{"admin reads another user", "/organizations/org%23acme/users/user%23member", "user#admin", http.StatusOK},
		{"member reads self", "/organizations/org%23acme/users/user%23member", "user#member", http.StatusOK},
		{"viewer reads self", "/organizations/org%23acme/users/user%23viewer", "user#viewer", http.StatusOK},
		{"member reads another user", "/organizations/org%23acme/users/user%23admin", "user#member", http.StatusForbidden},
		{"viewer reads another user", "/organizations/org%23acme/users/user%23member", "user#viewer", http.StatusForbidden},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := serveRoute(router, principalRequest(http.MethodGet, tc.path, tc.userID, "", ""))
			require.Equal(t, tc.want, rec.Code)
		})
	}
}

// TestOrganizationUsageRequiresOwnerOrAdmin keeps organization-wide billing
// usage with the existing administrative authority while project-scoped usage
// remains available to assigned members.
func TestOrganizationUsageRequiresOwnerOrAdmin(t *testing.T) {
	owner := principalUser(testOrgID, "user#owner", models.RoleOwner, models.UserStatusActive)
	admin := principalUser(testOrgID, "user#admin", models.RoleAdmin, models.UserStatusActive)
	member := principalUser(testOrgID, "user#member", models.RoleMember, models.UserStatusActive)
	viewer := principalUser(testOrgID, "user#viewer", models.RoleViewer, models.UserStatusActive)
	db := newPrincipalTestDB(t, testOrganization(), owner, admin, member, viewer)
	router := NewRouter(db)

	for _, userID := range []string{"user#owner", "user#admin"} {
		rec := serveRoute(router, principalRequest(http.MethodGet, "/organizations/org%23acme/usage", userID, "", ""))
		require.Equal(t, http.StatusOK, rec.Code, "%s must retain organization usage access", userID)
	}
	for _, userID := range []string{"user#member", "user#viewer"} {
		rec := serveRoute(router, principalRequest(http.MethodGet, "/organizations/org%23acme/usage", userID, "", ""))
		require.Equal(t, http.StatusForbidden, rec.Code, "%s must not read organization usage", userID)
	}
}

// TestProjectAndKeyAccessMatrix proves the fixed project controls still hold
// with the new principal gate: assigned members and bound keys succeed, while
// unassigned projects, wrong-project keys, and revoked keys stay closed.
func TestProjectAndKeyAccessMatrix(t *testing.T) {
	admin := principalUser(testOrgID, "user#admin", models.RoleAdmin, models.UserStatusActive)
	member := principalUser(testOrgID, "user#member", models.RoleMember, models.UserStatusActive)
	member.Projects = []string{"project#assigned"}
	db := newPrincipalTestDB(t,
		testOrganization(),
		admin,
		member,
		testProject("project#assigned", "Assigned"),
		testProject("project#other", "Other"),
		activeAPIKey("key#scoped", "project#assigned"),
		activeAPIKey("key#orgwide", ""),
	)
	router := NewRouter(db)

	projectCases := []struct {
		name   string
		path   string
		userID string
		keyID  string
		want   int
	}{
		{"admin reads unassigned project", "/organizations/org%23acme/projects/project%23other", "user#admin", "", http.StatusOK},
		{"member reads assigned project", "/organizations/org%23acme/projects/project%23assigned", "user#member", "", http.StatusOK},
		{"member reads unassigned project", "/organizations/org%23acme/projects/project%23other", "user#member", "", http.StatusNotFound},
		{"scoped key reads its project", "/organizations/org%23acme/projects/project%23assigned", "", "key#scoped", http.StatusOK},
		{"scoped key reads another project", "/organizations/org%23acme/projects/project%23other", "", "key#scoped", http.StatusNotFound},
		{"org-wide key reads any project", "/organizations/org%23acme/projects/project%23other", "", "key#orgwide", http.StatusOK},
		{"member reads assigned project usage", "/organizations/org%23acme/projects/project%23assigned/usage", "user#member", "", http.StatusOK},
		{"member reads unassigned project usage", "/organizations/org%23acme/projects/project%23other/usage", "user#member", "", http.StatusNotFound},
	}
	for _, tc := range projectCases {
		t.Run(tc.name, func(t *testing.T) {
			rec := serveRoute(router, principalRequest(http.MethodGet, tc.path, tc.userID, tc.keyID, ""))
			require.Equal(t, tc.want, rec.Code)
		})
	}

	usageBody := `{"project_id":"project#assigned","type":"api_call","quantity":1}`
	usageCases := []struct {
		name  string
		body  string
		keyID string
		want  int
	}{
		{"scoped key records its project", usageBody, "key#scoped", http.StatusOK},
		{"scoped key records another project", `{"project_id":"project#other","type":"api_call","quantity":1}`, "key#scoped", http.StatusNotFound},
		{"org-wide key records any project", `{"project_id":"project#other","type":"api_call","quantity":1}`, "key#orgwide", http.StatusOK},
		{"org-wide key records missing project", `{"project_id":"project#missing","type":"api_call","quantity":1}`, "key#orgwide", http.StatusNotFound},
		{"missing key identity", usageBody, "", http.StatusForbidden},
		{"unknown key identity", usageBody, "key#unknown", http.StatusForbidden},
	}
	for _, tc := range usageCases {
		t.Run(tc.name, func(t *testing.T) {
			rec := serveRoute(router, principalRequest(http.MethodPost, "/organizations/org%23acme/resources", "", tc.keyID, tc.body))
			require.Equal(t, tc.want, rec.Code)
		})
	}

	// A user principal cannot borrow the API-key usage route, even though the
	// user is active.
	rec := serveRoute(router, principalRequest(http.MethodPost, "/organizations/org%23acme/resources", "user#admin", "", usageBody))
	require.Equal(t, http.StatusForbidden, rec.Code)
}

// TestRevokedKeyIsDenied proves deactivation takes effect on the next route
// lookup rather than being masked by the principal gate's context values.
func TestRevokedKeyIsDenied(t *testing.T) {
	revoked := activeAPIKey("key#revoked", "project#assigned")
	revoked.Active = false
	db := newPrincipalTestDB(t,
		testOrganization(),
		testProject("project#assigned", "Assigned"),
		revoked,
	)
	router := NewRouter(db)

	rec := serveRoute(router, principalRequest(http.MethodGet, "/organizations/org%23acme/projects/project%23assigned", "", "key#revoked", ""))
	require.Equal(t, http.StatusForbidden, rec.Code)

	rec = serveRoute(router, principalRequest(http.MethodPost, "/organizations/org%23acme/resources", "", "key#revoked",
		`{"project_id":"project#assigned","type":"api_call","quantity":1}`))
	require.Equal(t, http.StatusForbidden, rec.Code)
}

// TestRecordUsageWithoutKeyIdentityIsControlled invokes the handler directly,
// bypassing the router's principal gate, to prove the handler itself never
// panics on a missing identity and creates no usage row.
func TestRecordUsageWithoutKeyIdentityIsControlled(t *testing.T) {
	db := newPrincipalTestDB(t, testOrganization(), testProject("project#one", "One"))
	req := principalRequest(http.MethodPost, "/organizations/acme/resources", "", "",
		`{"project_id":"project#one","type":"api_call","quantity":1}`)
	req = mux.SetURLVars(req, map[string]string{"org_id": testOrgID})
	rec := httptest.NewRecorder()

	require.NotPanics(t, func() {
		NewResourceHandler(db).RecordUsage(rec, req)
	})
	require.Equal(t, http.StatusForbidden, rec.Code)

	count, err := db.Model(&models.Resource{}).Where("OrgID", "=", testOrgID).Count()
	require.NoError(t, err)
	require.Zero(t, count, "a request without key identity must not record usage")
}

// TestHandlersRejectNonActiveUsersWithoutRouter proves the active-status check
// lives in the shared principal helper, not only in router middleware, so a
// direct handler invocation cannot bypass it.
func TestHandlersRejectNonActiveUsersWithoutRouter(t *testing.T) {
	suspended := principalUser(testOrgID, "user#suspended", models.RoleAdmin, models.UserStatusSuspended)
	db := newPrincipalTestDB(t, testOrganization(), suspended, testProject("project#one", "One"))

	cases := []struct {
		name string
		call func(w http.ResponseWriter, r *http.Request)
		path string
		vars map[string]string
	}{
		{"get organization", NewOrganizationHandler(db).GetOrganization, "/organizations/acme", map[string]string{"org_id": testOrgID}},
		{"list users", NewUserHandler(db).ListUsers, "/organizations/acme/users", map[string]string{"org_id": testOrgID}},
		{"get user", NewUserHandler(db).GetUser, "/organizations/acme/users/user%23suspended", map[string]string{"org_id": testOrgID, "user_id": "user#suspended"}},
		{"list projects", NewProjectHandler(db).ListProjects, "/organizations/acme/projects", map[string]string{"org_id": testOrgID}},
		{"organization usage", NewResourceHandler(db).GetUsageReport, "/organizations/acme/usage", map[string]string{"org_id": testOrgID}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := principalRequest(http.MethodGet, tc.path, "user#suspended", "", "")
			req = mux.SetURLVars(req, tc.vars)
			rec := httptest.NewRecorder()
			tc.call(rec, req)
			require.Equal(t, http.StatusForbidden, rec.Code)
		})
	}
}
