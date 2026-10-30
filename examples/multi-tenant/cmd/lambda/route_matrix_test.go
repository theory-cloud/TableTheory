package main

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/theory-cloud/tabletheory/v6/examples/multi-tenant/handlers"
	"github.com/theory-cloud/tabletheory/v6/examples/multi-tenant/models"
	"github.com/theory-cloud/tabletheory/v6/internal/dynamodbprobe"
	"github.com/theory-cloud/tabletheory/v6/pkg/core"
)

// TestMain points the entrypoint's cached database handle at the local
// DynamoDB endpoint the route matrix probes. The tests never use hosted AWS:
// each fixture skips unless the endpoint is loopback.
func TestMain(m *testing.M) {
	if strings.TrimSpace(os.Getenv("DYNAMODB_ENDPOINT")) == "" {
		endpoint := strings.TrimSpace(os.Getenv("AWS_ENDPOINT_URL"))
		if endpoint == "" {
			endpoint = dynamodbprobe.DefaultEndpoint
		}
		_ = os.Setenv("DYNAMODB_ENDPOINT", endpoint)
	}
	if strings.TrimSpace(os.Getenv("REGION")) == "" && strings.TrimSpace(os.Getenv("AWS_REGION")) == "" {
		_ = os.Setenv("AWS_REGION", "us-east-1")
	}
	_ = os.Setenv("AWS_ACCESS_KEY_ID", "dummy")
	_ = os.Setenv("AWS_SECRET_ACCESS_KEY", "dummy")
	os.Exit(m.Run())
}

const (
	headerTestUserID   = "X-Test-User-ID"
	headerTestAPIKeyID = "X-Test-API-Key-ID"
)

// routeMatrix owns one isolated tenant in DynamoDB Local plus the durable
// identities the two entrypoints can carry: users in every status and API keys
// with project, organization-wide, and revoked bindings.
type routeMatrix struct {
	db core.ExtendedDB

	orgID string

	adminID     string
	memberID    string
	viewerID    string
	suspendedID string
	invitedID   string
	deletedID   string
	missingID   string

	assignedProject string
	otherProject    string

	scopedKeyID     string
	scopedKeySecret string
	orgKeyID        string
	orgKeySecret    string
	revokedKeyID    string
}

func newRouteMatrix(t *testing.T) *routeMatrix {
	t.Helper()

	endpoint := strings.TrimSpace(os.Getenv("DYNAMODB_ENDPOINT"))
	if !isLoopbackEndpoint(endpoint) {
		t.Skipf("route matrix requires a loopback DynamoDB endpoint, got %q", endpoint)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := dynamodbprobe.CheckEndpoint(ctx, endpoint); err != nil {
		t.Skipf("DynamoDB Local not reachable: %v", err)
	}

	database, err := db()
	require.NoError(t, err)
	for _, model := range []any{
		&models.Organization{},
		&models.User{},
		&models.Project{},
		&models.Resource{},
		&models.APIKey{},
		&models.UsageReport{},
	} {
		if err := database.CreateTable(model); err != nil && !strings.Contains(err.Error(), "ResourceInUse") {
			require.NoErrorf(t, err, "create table for %T", model)
		}
	}

	nonce := strconv.FormatInt(time.Now().UnixNano(), 36)
	f := &routeMatrix{
		db:              database,
		orgID:           "org#m5-" + nonce,
		adminID:         "user#admin-" + nonce,
		memberID:        "user#member-" + nonce,
		viewerID:        "user#viewer-" + nonce,
		suspendedID:     "user#suspended-" + nonce,
		invitedID:       "user#invited-" + nonce,
		deletedID:       "user#deleted-" + nonce,
		missingID:       "user#missing-" + nonce,
		assignedProject: "project#assigned-" + nonce,
		otherProject:    "project#other-" + nonce,
		scopedKeyID:     "key#scoped-" + nonce,
		orgKeyID:        "key#org-" + nonce,
		revokedKeyID:    "key#revoked-" + nonce,
	}

	require.NoError(t, database.Model(&models.Organization{
		ID:     f.orgID,
		Slug:   "m5-" + nonce,
		Name:   "M5 Tenant",
		Plan:   models.PlanFree,
		Status: "active",
	}).Create())

	member := f.user(f.memberID, models.RoleMember, models.UserStatusActive)
	member.Projects = []string{f.assignedProject}
	users := []*models.User{
		f.user(f.adminID, models.RoleAdmin, models.UserStatusActive),
		member,
		f.user(f.viewerID, models.RoleViewer, models.UserStatusActive),
		f.user(f.suspendedID, models.RoleAdmin, models.UserStatusSuspended),
		f.user(f.invitedID, models.RoleMember, models.UserStatusInvited),
		f.user(f.deletedID, models.RoleMember, models.UserStatusDeleted),
	}
	for _, user := range users {
		require.NoError(t, database.Model(user).Create())
	}

	for id, name := range map[string]string{
		f.assignedProject: "Assigned",
		f.otherProject:    "Other",
	} {
		require.NoError(t, database.Model(&models.Project{
			ID:          f.orgID + "#" + id,
			OrgID:       f.orgID,
			ProjectID:   id,
			Name:        name,
			ProjectName: name,
			Status:      "active",
		}).Create())
	}

	f.scopedKeySecret = f.createKey(t, f.scopedKeyID, f.assignedProject, true)
	f.orgKeySecret = f.createKey(t, f.orgKeyID, "", true)
	f.createKey(t, f.revokedKeyID, f.assignedProject, false)

	return f
}

func (f *routeMatrix) user(userID, role, status string) *models.User {
	return &models.User{
		ID:       f.orgID + "#" + userID,
		OrgID:    f.orgID,
		UserID:   userID,
		Email:    userID + "@m5.example.test",
		OrgEmail: f.orgID + "#" + userID + "@m5.example.test",
		Role:     role,
		Status:   status,
	}
}

func (f *routeMatrix) createKey(t *testing.T, keyID, projectID string, active bool) string {
	t.Helper()

	secret := "s3cret-" + keyID
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.MinCost)
	require.NoError(t, err)
	require.NoError(t, f.db.Model(&models.APIKey{
		ID:        f.orgID + "#" + keyID,
		OrgID:     f.orgID,
		KeyID:     keyID,
		KeyHash:   string(hash),
		ProjectID: projectID,
		RateLimit: 1000,
		Active:    active,
	}).Create())
	return secret
}

func (f *routeMatrix) wireKey(keyID, secret string) string {
	return "sk_test_" + strings.TrimPrefix(keyID, "key#") + "." + secret
}

func (f *routeMatrix) orgPath(parts ...string) string {
	segments := append([]string{"/organizations", url.PathEscape(f.orgID)}, parts...)
	return strings.Join(segments, "/")
}

func (f *routeMatrix) proxyDispatch(t *testing.T, functionType, method, path string, authorizer map[string]interface{}, body string) (int, string) {
	t.Helper()
	t.Setenv("FUNCTION_TYPE", functionType)

	raw, err := json.Marshal(events.APIGatewayProxyRequest{
		HTTPMethod:            method,
		Path:                  path,
		Body:                  body,
		Headers:               map[string]string{"Content-Type": "application/json"},
		QueryStringParameters: map[string]string{},
		RequestContext:        events.APIGatewayProxyRequestContext{Authorizer: authorizer},
	})
	require.NoError(t, err)

	out, err := dispatch(context.Background(), raw)
	require.NoError(t, err, "dispatch must return an API Gateway response")

	var response events.APIGatewayProxyResponse
	require.NoError(t, json.Unmarshal(out, &response))
	return response.StatusCode, response.Body
}

func (f *routeMatrix) userContext(userID string) map[string]interface{} {
	return map[string]interface{}{"user_id": userID, "org_id": f.orgID}
}

func (f *routeMatrix) keyContext(keyID string) map[string]interface{} {
	return map[string]interface{}{"org_id": f.orgID, "api_key_id": keyID}
}

// localHTTPServer serves the shared router over a real loopback HTTP socket
// and attaches the trusted tenant context exactly as the local entrypoint does
// after it authenticates the caller.
func (f *routeMatrix) localHTTPServer(t *testing.T) *httptest.Server {
	t.Helper()

	trusted := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), "org_id", f.orgID)
		if userID := r.Header.Get(headerTestUserID); userID != "" {
			ctx = context.WithValue(ctx, "user_id", userID)
		}
		if keyID := r.Header.Get(headerTestAPIKeyID); keyID != "" {
			ctx = context.WithValue(ctx, "api_key_id", keyID)
		}
		handlers.NewRouter(f.db).ServeHTTP(w, r.WithContext(ctx))
	})

	server := httptest.NewServer(trusted)
	t.Cleanup(server.Close)
	return server
}

func (f *routeMatrix) doHTTP(t *testing.T, server *httptest.Server, method, path, userID, keyID, body string) (int, string) {
	t.Helper()

	request, err := http.NewRequest(method, server.URL+path, strings.NewReader(body))
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	if userID != "" {
		request.Header.Set(headerTestUserID, userID)
	}
	if keyID != "" {
		request.Header.Set(headerTestAPIKeyID, keyID)
	}

	response, err := server.Client().Do(request)
	require.NoError(t, err)
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	return response.StatusCode, string(payload)
}

// TestAPIGatewayRouteMatrix drives API Gateway proxy events through dispatch,
// including the authorizer context shape the template produces. It covers
// active admin/self/project-scoped success, every non-active user status,
// directory and usage denial, wrong-project keys, and missing key identity.
func TestAPIGatewayRouteMatrix(t *testing.T) {
	f := newRouteMatrix(t)

	userRoutes := []struct {
		name   string
		method string
		parts  []string
		body   string
		userID string
		want   int
	}{
		{"admin lists directory", http.MethodGet, []string{"users"}, "", f.adminID, http.StatusOK},
		{"member lists directory", http.MethodGet, []string{"users"}, "", f.memberID, http.StatusForbidden},
		{"viewer lists directory", http.MethodGet, []string{"users"}, "", f.viewerID, http.StatusForbidden},
		{"suspended admin lists directory", http.MethodGet, []string{"users"}, "", f.suspendedID, http.StatusForbidden},
		{"invited member lists directory", http.MethodGet, []string{"users"}, "", f.invitedID, http.StatusForbidden},
		{"deleted member lists directory", http.MethodGet, []string{"users"}, "", f.deletedID, http.StatusForbidden},
		{"missing user lists directory", http.MethodGet, []string{"users"}, "", f.missingID, http.StatusForbidden},
		{"member reads self", http.MethodGet, []string{"users", url.PathEscape(f.memberID)}, "", f.memberID, http.StatusOK},
		{"member reads another user", http.MethodGet, []string{"users", url.PathEscape(f.adminID)}, "", f.memberID, http.StatusForbidden},
		{"admin reads another user", http.MethodGet, []string{"users", url.PathEscape(f.memberID)}, "", f.adminID, http.StatusOK},
		{"admin reads organization usage", http.MethodGet, []string{"usage"}, "", f.adminID, http.StatusOK},
		{"member reads organization usage", http.MethodGet, []string{"usage"}, "", f.memberID, http.StatusForbidden},
		{"viewer reads organization usage", http.MethodGet, []string{"usage"}, "", f.viewerID, http.StatusForbidden},
		{"member reads assigned project", http.MethodGet, []string{"projects", url.PathEscape(f.assignedProject)}, "", f.memberID, http.StatusOK},
		{"member reads other project", http.MethodGet, []string{"projects", url.PathEscape(f.otherProject)}, "", f.memberID, http.StatusNotFound},
		{"admin reads other project", http.MethodGet, []string{"projects", url.PathEscape(f.otherProject)}, "", f.adminID, http.StatusOK},
	}
	for _, tc := range userRoutes {
		t.Run("user/"+tc.name, func(t *testing.T) {
			status, _ := f.proxyDispatch(t, "user", tc.method, f.orgPath(tc.parts...), f.userContext(tc.userID), tc.body)
			require.Equal(t, tc.want, status)
		})
	}

	keyRoutes := []struct {
		name   string
		parts  []string
		body   string
		keyID  string
		userID string
		want   int
	}{
		{"scoped key reads its project", []string{"projects", url.PathEscape(f.assignedProject)}, "", f.scopedKeyID, "", http.StatusOK},
		{"scoped key reads another project", []string{"projects", url.PathEscape(f.otherProject)}, "", f.scopedKeyID, "", http.StatusNotFound},
		{"org key reads any project", []string{"projects", url.PathEscape(f.otherProject)}, "", f.orgKeyID, "", http.StatusOK},
		{"revoked key reads its project", []string{"projects", url.PathEscape(f.assignedProject)}, "", f.revokedKeyID, "", http.StatusForbidden},
		{"scoped key records its project", []string{"resources"}, `{"project_id":"` + f.assignedProject + `","type":"api_call","quantity":1}`, f.scopedKeyID, "", http.StatusOK},
		{"scoped key records another project", []string{"resources"}, `{"project_id":"` + f.otherProject + `","type":"api_call","quantity":1}`, f.scopedKeyID, "", http.StatusNotFound},
		{"scoped key records missing project", []string{"resources"}, `{"project_id":"project#missing","type":"api_call","quantity":1}`, f.scopedKeyID, "", http.StatusNotFound},
		{"org key records another project", []string{"resources"}, `{"project_id":"` + f.otherProject + `","type":"api_call","quantity":1}`, f.orgKeyID, "", http.StatusOK},
		{"user identity cannot record usage", []string{"resources"}, `{"project_id":"` + f.assignedProject + `","type":"api_call","quantity":1}`, "", f.adminID, http.StatusForbidden},
	}
	for _, tc := range keyRoutes {
		t.Run("key/"+tc.name, func(t *testing.T) {
			authorizer := f.keyContext(tc.keyID)
			if tc.userID != "" {
				authorizer = f.userContext(tc.userID)
			}
			method := http.MethodGet
			if tc.body != "" {
				method = http.MethodPost
			}
			status, _ := f.proxyDispatch(t, "resource", method, f.orgPath(tc.parts...), authorizer, tc.body)
			require.Equal(t, tc.want, status)
		})
	}

	// A missing key identity must produce a controlled response, never a panic.
	body := `{"project_id":"` + f.assignedProject + `","type":"api_call","quantity":1}`
	require.NotPanics(t, func() {
		status, _ := f.proxyDispatch(t, "resource", http.MethodPost, f.orgPath("resources"),
			map[string]interface{}{"org_id": f.orgID}, body)
		require.Equal(t, http.StatusForbidden, status)
	})
}

// TestLocalHTTPRouteMatrix sends real HTTP requests over a loopback socket to
// the shared router backed by DynamoDB Local. It mirrors the API Gateway matrix
// at the transport boundary the local entrypoint serves.
func TestLocalHTTPRouteMatrix(t *testing.T) {
	f := newRouteMatrix(t)
	server := f.localHTTPServer(t)

	cases := []struct {
		name   string
		method string
		path   string
		body   string
		userID string
		keyID  string
		want   int
	}{
		{"active admin lists directory", http.MethodGet, f.orgPath("users"), "", f.adminID, "", http.StatusOK},
		{"suspended admin lists directory", http.MethodGet, f.orgPath("users"), "", f.suspendedID, "", http.StatusForbidden},
		{"invited member lists directory", http.MethodGet, f.orgPath("users"), "", f.invitedID, "", http.StatusForbidden},
		{"deleted member lists directory", http.MethodGet, f.orgPath("users"), "", f.deletedID, "", http.StatusForbidden},
		{"missing user lists directory", http.MethodGet, f.orgPath("users"), "", f.missingID, "", http.StatusForbidden},
		{"member lists directory", http.MethodGet, f.orgPath("users"), "", f.memberID, "", http.StatusForbidden},
		{"viewer reads organization usage", http.MethodGet, f.orgPath("usage"), "", f.viewerID, "", http.StatusForbidden},
		{"admin reads organization usage", http.MethodGet, f.orgPath("usage"), "", f.adminID, "", http.StatusOK},
		{"member reads self", http.MethodGet, f.orgPath("users", url.PathEscape(f.memberID)), "", f.memberID, "", http.StatusOK},
		{"member reads another user", http.MethodGet, f.orgPath("users", url.PathEscape(f.adminID)), "", f.memberID, "", http.StatusForbidden},
		{"member reads assigned project", http.MethodGet, f.orgPath("projects", url.PathEscape(f.assignedProject)), "", f.memberID, "", http.StatusOK},
		{"member reads other project", http.MethodGet, f.orgPath("projects", url.PathEscape(f.otherProject)), "", f.memberID, "", http.StatusNotFound},
		{"scoped key reads its project", http.MethodGet, f.orgPath("projects", url.PathEscape(f.assignedProject)), "", "", f.scopedKeyID, http.StatusOK},
		{"scoped key reads another project", http.MethodGet, f.orgPath("projects", url.PathEscape(f.otherProject)), "", "", f.scopedKeyID, http.StatusNotFound},
		{"scoped key records its project", http.MethodPost, f.orgPath("resources"), `{"project_id":"` + f.assignedProject + `","type":"api_call","quantity":1}`, "", f.scopedKeyID, http.StatusOK},
		{"scoped key records another project", http.MethodPost, f.orgPath("resources"), `{"project_id":"` + f.otherProject + `","type":"api_call","quantity":1}`, "", f.scopedKeyID, http.StatusNotFound},
		{"missing key identity records nothing", http.MethodPost, f.orgPath("resources"), `{"project_id":"` + f.assignedProject + `","type":"api_call","quantity":1}`, "", "", http.StatusForbidden},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, _ := f.doHTTP(t, server, tc.method, tc.path, tc.userID, tc.keyID, tc.body)
			require.Equal(t, tc.want, status)
		})
	}
}

// TestWarmWrongSecretsReuseTheCachedVerifier drives the authorizer across
// invocations with wrong secrets for a real stored key. The cached handler is
// the same instance before and after the burst, so the per-key failure history
// and bcrypt slots that bound the work in handlers survive warm invocations.
// A different valid key still authenticates, proving the lockout is per key ID
// and that successful credentials are not cached.
func TestWarmWrongSecretsReuseTheCachedVerifier(t *testing.T) {
	f := newRouteMatrix(t)

	before, err := apiKeyHandler()
	require.NoError(t, err)

	wrong := "sk_test_" + strings.TrimPrefix(f.scopedKeyID, "key#") + ".wrong-secret"
	event := func(apiKey string) events.APIGatewayCustomAuthorizerRequestTypeRequest {
		return events.APIGatewayCustomAuthorizerRequestTypeRequest{
			Type:      "REQUEST",
			MethodArn: "arn:aws:execute-api:us-east-1:123456789012:api/dev/POST/organizations",
			Headers:   map[string]string{"X-API-Key": apiKey},
		}
	}

	for i := 0; i < 8; i++ {
		_, err := authorizeAPIKey(event(wrong))
		require.Error(t, err, "wrong secret %d must be denied", i)
		assert.Contains(t, err.Error(), "Unauthorized")
	}

	after, err := apiKeyHandler()
	require.NoError(t, err)
	require.Same(t, before, after, "warm invocations must keep the verifier that recorded the failures")

	allowed, err := authorizeAPIKey(event(f.wireKey(f.orgKeyID, f.orgKeySecret)))
	require.NoError(t, err, "an unrelated valid key must still authenticate")
	assert.Equal(t, f.orgKeyID, allowed.PrincipalID)
	assert.Equal(t, f.orgID, allowed.Context["org_id"])

	// Revocation is observed on the next invocation: a successful verification
	// is never cached.
	var orgKey models.APIKey
	require.NoError(t, f.db.Model(&models.APIKey{}).
		Where("ID", "=", f.orgID+"#"+f.orgKeyID).
		First(&orgKey))
	orgKey.Active = false
	require.NoError(t, f.db.Model(&orgKey).Update())

	_, err = authorizeAPIKey(event(f.wireKey(f.orgKeyID, f.orgKeySecret)))
	require.Error(t, err, "a revoked key must stop authenticating on the next invocation")
	assert.Contains(t, err.Error(), "Unauthorized")
}

func isLoopbackEndpoint(endpoint string) bool {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" {
		return false
	}
	host := parsed.Hostname()
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
