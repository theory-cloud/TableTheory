package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v4/examples/multi-tenant/models"
)

// requestWithContext builds a GET request for /organizations/{org} carrying the
// given tenant context values, mirroring what the entrypoints attach.
func requestWithContext(method, urlOrg, ctxOrg, ctxUser string) *http.Request {
	req := httptest.NewRequest(method, "/organizations/"+urlOrg, nil)
	req = mux.SetURLVars(req, map[string]string{"org_id": urlOrg})
	ctx := req.Context()
	if ctxOrg != "" {
		ctx = context.WithValue(ctx, "org_id", ctxOrg)
	}
	if ctxUser != "" {
		ctx = context.WithValue(ctx, "user_id", ctxUser)
	}
	return req.WithContext(ctx)
}

// TestGetOrganization_CrossOrgAccessIsDenied proves the authorization boundary
// runs before any database work: the handler is built with a nil db, so a
// request that reached the store would panic. A caller bound to one organization
// asking for another gets a 404 — the same answer as a missing organization, so
// existence is not revealed.
func TestGetOrganization_CrossOrgAccessIsDenied(t *testing.T) {
	handler := NewOrganizationHandler(nil)

	req := requestWithContext(http.MethodGet, "org#victim", "org#attacker", "user#attacker")
	rec := httptest.NewRecorder()

	handler.GetOrganization(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestGetOrganization_WithoutBoundOrgIsForbidden covers the api-key/authorizer
// case where no organization was bound at all. The request is refused before
// the store is touched.
func TestGetOrganization_WithoutBoundOrgIsForbidden(t *testing.T) {
	handler := NewOrganizationHandler(nil)

	req := requestWithContext(http.MethodGet, "org#victim", "", "user#attacker")
	rec := httptest.NewRecorder()

	handler.GetOrganization(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// TestRedactOrganization_StripsSensitiveFields pins the response shape: billing
// details, the subscription ID, and the webhook secret must never be encoded,
// while the organization's public identity survives.
func TestRedactOrganization_StripsSensitiveFields(t *testing.T) {
	org := &models.Organization{
		ID:     "org#demo",
		Slug:   "demo",
		Name:   "Demo Corp",
		Plan:   models.PlanPro,
		Status: "active",
		BillingInfo: models.BillingInfo{
			CustomerID:      "cus_secret",
			PaymentMethodID: "pm_secret",
			BillingEmail:    "billing@example.com",
			CompanyName:     "Demo Inc",
			TaxID:           "TAX-secret",
			Address: models.Address{
				Line1: "1 Secret St",
				City:  "Testville",
			},
		},
		Settings: models.OrgSettings{
			RequireMFA:    true,
			WebhookURL:    "https://example.com/hook",
			WebhookSecret: "whsec_secret",
		},
		SubscriptionID: "sub_secret",
	}

	encoded, err := json.Marshal(redactOrganization(org))
	require.NoError(t, err)
	text := string(encoded)

	for _, leaked := range []string{
		"cus_secret", "pm_secret", "billing@example.com", "Demo Inc",
		"TAX-secret", "1 Secret St", "whsec_secret", "sub_secret",
	} {
		assert.NotContains(t, text, leaked, "sensitive value must not be encoded")
	}

	// The public identity of the organization remains.
	assert.Contains(t, text, `"name":"Demo Corp"`)
	assert.Contains(t, text, `"slug":"demo"`)
	assert.Contains(t, text, `"plan":"pro"`)
	assert.Contains(t, text, `"id":"org#demo"`)

	// Redaction returns a copy, so the stored model is untouched.
	assert.Equal(t, "cus_secret", org.BillingInfo.CustomerID)
	assert.Equal(t, "whsec_secret", org.Settings.WebhookSecret)
	assert.Equal(t, "sub_secret", org.SubscriptionID)
}

// TestRedactOrganizations_RedactsEveryElement covers the list response, which
// encodes many organizations at once.
func TestRedactOrganizations_RedactsEveryElement(t *testing.T) {
	orgs := []models.Organization{
		{
			ID:             "org#one",
			Name:           "One",
			Plan:           models.PlanFree,
			Settings:       models.OrgSettings{WebhookSecret: "whsec_one"},
			BillingInfo:    models.BillingInfo{CustomerID: "cus_one"},
			SubscriptionID: "sub_one",
		},
		{
			ID:             "org#two",
			Name:           "Two",
			Plan:           models.PlanStarter,
			Settings:       models.OrgSettings{WebhookSecret: "whsec_two"},
			BillingInfo:    models.BillingInfo{CustomerID: "cus_two"},
			SubscriptionID: "sub_two",
		},
	}

	encoded, err := json.Marshal(redactOrganizations(orgs))
	require.NoError(t, err)
	text := string(encoded)

	for _, leaked := range []string{"whsec_one", "cus_one", "sub_one", "whsec_two", "cus_two", "sub_two"} {
		assert.NotContains(t, text, leaked)
	}
	assert.Contains(t, text, `"name":"One"`)
	assert.Contains(t, text, `"name":"Two"`)

	// A nil listing stays nil, so an empty result still encodes as null.
	assert.Nil(t, redactOrganizations(nil))
}

func TestOrgAndUserFromContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), "org_id", "org#demo")
	ctx = context.WithValue(ctx, "user_id", "user#someone")

	org, ok := orgIDFromContext(ctx)
	require.True(t, ok)
	assert.Equal(t, "org#demo", org)

	user, ok := userIDFromContext(ctx)
	require.True(t, ok)
	assert.Equal(t, "user#someone", user)

	// Wrong-typed or blank values are reported as absent, never a panic.
	for _, value := range []any{42, "", "   ", nil} {
		bad := context.WithValue(context.Background(), "org_id", value)
		_, ok := orgIDFromContext(bad)
		assert.False(t, ok)
		_, ok = userIDFromContext(bad)
		assert.False(t, ok)
	}
}

func TestNormalizeOrgID(t *testing.T) {
	assert.Equal(t, "org#demo", normalizeOrgID("demo"))
	assert.Equal(t, "org#demo", normalizeOrgID("org#demo"))
	assert.Equal(t, "org#demo", normalizeOrgID("  demo  "))
	assert.Equal(t, "", normalizeOrgID(""))
}
