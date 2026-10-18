package handlers

import (
	"context"
	"io"
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

const testOrgID = "org#acme"

func newAuthzTestDB(t *testing.T, users ...*models.User) core.ExtendedDB {
	t.Helper()

	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, fakedb.New())
	require.NoError(t, err)
	for _, model := range []any{&models.User{}, &models.Project{}, &models.APIKey{}} {
		require.NoError(t, db.CreateTable(model))
	}
	for _, user := range users {
		require.NoError(t, db.Model(user).Create())
	}
	return db
}

func authzRequest(method, urlOrg string, vars map[string]string, userID, apiKeyID string) *http.Request {
	if vars == nil {
		vars = map[string]string{}
	}
	vars["org_id"] = urlOrg

	req := httptest.NewRequest(method, "/organizations/"+urlOrg, nil)
	req = mux.SetURLVars(req, vars)
	ctx := context.WithValue(req.Context(), "org_id", testOrgID)
	if userID != "" {
		ctx = context.WithValue(ctx, "user_id", userID)
	}
	if apiKeyID != "" {
		ctx = context.WithValue(ctx, "api_key_id", apiKeyID)
	}
	return req.WithContext(ctx)
}

func memberUser() *models.User {
	return &models.User{
		ID:     testOrgID + "#user#member",
		OrgID:  testOrgID,
		UserID: "user#member",
		Role:   models.RoleMember,
		Status: "active",
	}
}

func TestMemberCannotPerformAdministrativeMutations(t *testing.T) {
	db := newAuthzTestDB(t, memberUser())

	tests := []struct {
		name string
		call func(w http.ResponseWriter, r *http.Request)
		path string
	}{
		{"invite user", NewUserHandler(db).InviteUser, "/organizations/acme/invitations"},
		{"update user", NewUserHandler(db).UpdateUser, "/organizations/acme/users/user#target"},
		{"delete user", NewUserHandler(db).DeleteUser, "/organizations/acme/users/user#target"},
		{"update organization settings", NewOrganizationHandler(db).UpdateOrganizationSettings, "/organizations/acme/settings"},
		{"create project", NewProjectHandler(db).CreateProject, "/organizations/acme/projects"},
		{"create api key", NewAPIKeyHandler(db).CreateAPIKey, "/organizations/acme/api-keys"},
		{"list api keys", NewAPIKeyHandler(db).ListAPIKeys, "/organizations/acme/api-keys"},
		{"get api key", NewAPIKeyHandler(db).GetAPIKey, "/organizations/acme/api-keys/key#one"},
		{"update api key", NewAPIKeyHandler(db).UpdateAPIKey, "/organizations/acme/api-keys/key#one"},
		{"delete api key", NewAPIKeyHandler(db).DeleteAPIKey, "/organizations/acme/api-keys/key#one"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			vars := map[string]string{}
			switch test.name {
			case "update user", "delete user":
				vars["user_id"] = "target"
			case "get api key", "update api key", "delete api key":
				vars["key_id"] = "one"
			}
			req := authzRequest(http.MethodPost, "acme", vars, "user#member", "")
			rec := httptest.NewRecorder()

			test.call(rec, req)

			require.Equal(t, http.StatusForbidden, rec.Code, "a member must not reach %s", test.name)
		})
	}
}

func TestAdminCannotGrantOwnerRole(t *testing.T) {
	admin := &models.User{
		ID:     testOrgID + "#user#admin",
		OrgID:  testOrgID,
		UserID: "user#admin",
		Role:   models.RoleAdmin,
		Status: "active",
	}
	target := &models.User{
		ID:     testOrgID + "#user#target",
		OrgID:  testOrgID,
		UserID: "user#target",
		Role:   models.RoleMember,
		Status: "active",
	}
	db := newAuthzTestDB(t, admin, target)

	req := authzRequest(http.MethodPut, "acme", map[string]string{"user_id": "target"}, "user#admin", "")
	req.Body = io.NopCloser(strings.NewReader(`{"role":"owner"}`))
	rec := httptest.NewRecorder()

	NewUserHandler(db).UpdateUser(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestMemberCanOnlyReadAssignedProjects(t *testing.T) {
	member := memberUser()
	member.Projects = []string{"project#assigned"}

	db := newAuthzTestDB(t, member)
	for _, project := range []*models.Project{
		{
			ID:        testOrgID + "#project#assigned",
			OrgID:     testOrgID,
			ProjectID: "project#assigned",
			Name:      "Assigned",
			Status:    "active",
		},
		{
			ID:        testOrgID + "#project#unassigned",
			OrgID:     testOrgID,
			ProjectID: "project#unassigned",
			Name:      "Unassigned",
			Status:    "active",
		},
	} {
		require.NoError(t, db.Model(project).Create())
	}

	projectHandler := NewProjectHandler(db)

	req := authzRequest(http.MethodGet, "acme", map[string]string{"project_id": "assigned"}, "user#member", "")
	rec := httptest.NewRecorder()
	projectHandler.GetProject(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	req = authzRequest(http.MethodGet, "acme", map[string]string{"project_id": "unassigned"}, "user#member", "")
	rec = httptest.NewRecorder()
	projectHandler.GetProject(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)

	req = authzRequest(http.MethodPut, "acme", map[string]string{"project_id": "unassigned"}, "user#member", "")
	req.Body = io.NopCloser(strings.NewReader(`{}`))
	rec = httptest.NewRecorder()
	projectHandler.UpdateProject(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)

	req = authzRequest(http.MethodGet, "acme", map[string]string{"project_id": "unassigned"}, "user#member", "")
	rec = httptest.NewRecorder()
	NewResourceHandler(db).GetProjectUsage(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestProjectScopedAPIKeyCannotRecordUsageForAnotherProject(t *testing.T) {
	key := &models.APIKey{
		ID:        testOrgID + "#key#one",
		OrgID:     testOrgID,
		KeyID:     "key#one",
		ProjectID: "project#assigned",
	}
	db := newAuthzTestDB(t)
	require.NoError(t, db.Model(key).Create())

	req := authzRequest(http.MethodPost, "acme", nil, "", "key#one")
	req.Body = io.NopCloser(strings.NewReader(`{"project_id":"project#other","type":"api_call","quantity":1}`))
	rec := httptest.NewRecorder()

	NewResourceHandler(db).RecordUsage(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
}
