package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"os"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory"
	"github.com/theory-cloud/tabletheory/examples/multi-tenant/handlers"
	"github.com/theory-cloud/tabletheory/examples/multi-tenant/models"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/session"
)

func setupTestDB(t *testing.T) core.ExtendedDB {
	endpoint := os.Getenv("AWS_ENDPOINT_URL")
	if endpoint == "" {
		// Default to localhost if not set, or skip
		endpoint = "http://localhost:8000"
	}

	dbConfig := session.Config{
		Region:   "us-east-1",
		Endpoint: endpoint,
	}
	db, err := theorydb.New(dbConfig)
	require.NoError(t, err)

	// Create test tables
	models := []any{
		&models.Organization{},
		&models.User{},
		&models.Project{},
		&models.Resource{},
		&models.APIKey{},
		&models.AuditLog{},
		&models.Invitation{},
		&models.UsageReport{},
	}

	for _, model := range models {
		err := db.CreateTable(model)
		if err != nil && err.Error() != "ResourceInUseException" {
			t.Fatalf("Failed to create table for %T: %v", model, err)
		}
	}

	return db
}

func TestCreateOrganization(t *testing.T) {
	db := setupTestDB(t)
	handler := handlers.NewOrganizationHandler(db)

	// Generate a unique slug for this test run to ensure conflict test works reliably
	baseSlug := fmt.Sprintf("test-corp-%d", time.Now().UnixNano())

	tests := []struct {
		name           string
		payload        map[string]any
		expectedStatus int
		checkResponse  func(*testing.T, map[string]any)
	}{
		{
			name: "Create organization successfully",
			payload: map[string]any{
				"name":        "Test Corp",
				"slug":        baseSlug,
				"owner_email": "test@test.com",
				"plan":        "starter",
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp map[string]any) {
				assert.Equal(t, "Test Corp", resp["name"])
				assert.Equal(t, baseSlug, resp["slug"])
				assert.Equal(t, "starter", resp["plan"])
				assert.Equal(t, "active", resp["status"])
				assert.NotEmpty(t, resp["id"])
				assert.NotEmpty(t, resp["owner_id"])
			},
		},
		{
			name: "Create organization with free plan",
			payload: map[string]any{
				"name":        "Free Corp",
				"slug":        fmt.Sprintf("free-corp-%d", time.Now().UnixNano()),
				"owner_email": "free@test.com",
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp map[string]any) {
				assert.Equal(t, "free", resp["plan"])
				limits := resp["limits"].(map[string]any)
				assert.Equal(t, float64(3), limits["max_users"])
				assert.Equal(t, float64(1), limits["max_projects"])
			},
		},
		{
			name: "Missing required fields",
			payload: map[string]any{
				"name": "Incomplete Corp",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Duplicate slug",
			payload: map[string]any{
				"name":        "Duplicate Corp",
				"slug":        baseSlug, // Same as first test
				"owner_email": "dup@test.com",
			},
			expectedStatus: http.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.payload)
			req := httptest.NewRequest("POST", "/organizations", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			handler.CreateOrganization(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)

			if tt.checkResponse != nil && rr.Code == http.StatusOK {
				var resp map[string]any
				err := json.Unmarshal(rr.Body.Bytes(), &resp)
				require.NoError(t, err)
				tt.checkResponse(t, resp)
			}
		})
	}
}

func TestGetOrganization(t *testing.T) {
	db := setupTestDB(t)
	handler := handlers.NewOrganizationHandler(db)

	// Create a test organization first
	org := &models.Organization{
		ID:     "org#test-123",
		Name:   "Test Organization",
		Slug:   "test-org",
		Plan:   "pro",
		Status: "active",
	}
	err := db.Model(org).Create()
	require.NoError(t, err)

	tests := []struct {
		name           string
		orgID          string
		expectedStatus int
	}{
		{
			name:           "Get existing organization",
			orgID:          "test-123",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Get non-existent organization",
			orgID:          "non-existent",
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/organizations/"+tt.orgID, nil)
			req = mux.SetURLVars(req, map[string]string{"org_id": tt.orgID})

			rr := httptest.NewRecorder()
			handler.GetOrganization(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)

			if rr.Code == http.StatusOK {
				var resp models.Organization
				err := json.Unmarshal(rr.Body.Bytes(), &resp)
				require.NoError(t, err)
				assert.Equal(t, org.Name, resp.Name)
				assert.Equal(t, org.Plan, resp.Plan)
			}
		})
	}
}

func TestUpdateOrganizationSettings(t *testing.T) {
	db := setupTestDB(t)
	handler := handlers.NewOrganizationHandler(db)

	// Create a test organization
	org := &models.Organization{
		ID:     "org#test-456",
		Name:   "Settings Test Org",
		Slug:   "settings-test",
		Plan:   "starter",
		Status: "active",
		Settings: models.OrgSettings{
			RequireMFA:     false,
			SessionTimeout: 60,
		},
	}
	err := db.Model(org).Create()
	require.NoError(t, err)

	// Test updating settings
	newSettings := models.OrgSettings{
		RequireMFA:     true,
		SessionTimeout: 120,
		AllowedDomains: []string{"test.com", "example.com"},
	}

	body, _ := json.Marshal(newSettings)
	req := httptest.NewRequest("PUT", "/organizations/test-456/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = mux.SetURLVars(req, map[string]string{"org_id": "test-456"})

	// Add user context
	ctx := context.WithValue(req.Context(), "user_id", "user#admin")
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.UpdateOrganizationSettings(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	// Verify the settings were updated
	var updatedOrg models.Organization
	err = json.Unmarshal(rr.Body.Bytes(), &updatedOrg)
	require.NoError(t, err)
	assert.True(t, updatedOrg.Settings.RequireMFA)
	assert.Equal(t, 120, updatedOrg.Settings.SessionTimeout)
	assert.Equal(t, []string{"test.com", "example.com"}, updatedOrg.Settings.AllowedDomains)
}

func TestOrganizationPlanLimits(t *testing.T) {
	db := setupTestDB(t)
	handler := handlers.NewOrganizationHandler(db)

	plans := []struct {
		plan             string
		expectedUsers    int
		expectedProjects int
		expectedAPIReqs  int
		hasCustomDomain  bool
		hasSSO           bool
	}{
		{"free", 3, 1, 10000, false, false},
		{"starter", 10, 5, 100000, false, false},
		{"pro", 50, 20, 1000000, true, true},
		{"enterprise", -1, -1, -1, true, true}, // -1 means unlimited
	}

	for _, p := range plans {
		t.Run(p.plan+" plan limits", func(t *testing.T) {
			payload := map[string]any{
				"name":        p.plan + " Test Corp",
				"slug":        fmt.Sprintf("%s-test-corp-%d", p.plan, time.Now().UnixNano()),
				"owner_email": p.plan + "@test.com",
				"plan":        p.plan,
			}

			body, _ := json.Marshal(payload)
			req := httptest.NewRequest("POST", "/organizations", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			handler.CreateOrganization(rr, req)

			require.Equal(t, http.StatusOK, rr.Code)

			var resp map[string]any
			err := json.Unmarshal(rr.Body.Bytes(), &resp)
			require.NoError(t, err)

			limits := resp["limits"].(map[string]any)
			assert.Equal(t, float64(p.expectedUsers), limits["max_users"])
			assert.Equal(t, float64(p.expectedProjects), limits["max_projects"])
			assert.Equal(t, float64(p.expectedAPIReqs), limits["max_api_requests"])
			assert.Equal(t, p.hasCustomDomain, limits["custom_domain"])
			assert.Equal(t, p.hasSSO, limits["sso"])
		})
	}
}
