package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v4/examples/multi-tenant/auth"
)

// These tests exercise the entrypoint's own adapter and authorizer code — the
// parts that do not need a database. The API path's routing is shared with the
// local server, which the example's DynamoDB-backed tests cover.

const testJWTSecret = "test-secret"

// validClaims returns an unexpired token payload for the standard test caller.
func validClaims() auth.Claims {
	return auth.Claims{
		Subject:   "user123",
		OrgID:     "orgabc",
		IssuedAt:  time.Now().Unix(),
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}
}

// signedToken signs claims with the test secret, so the authorizer accepts it.
func signedToken(t *testing.T, claims auth.Claims) string {
	t.Helper()
	token, err := auth.Sign(testJWTSecret, claims)
	require.NoError(t, err)
	return token
}

// unsignedToken builds an "alg":"none" token by hand, the classic JWT bypass.
func unsignedToken(t *testing.T) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(
		`{"sub":"user123","org":"orgabc","exp":` + strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10) + `}`))
	return header + "." + payload + "."
}

func TestDispatch_JWTAuthorizerAllowsBearerToken(t *testing.T) {
	t.Setenv("FUNCTION_TYPE", "jwt_authorizer")
	t.Setenv("JWT_SECRET", testJWTSecret)

	const methodARN = "arn:aws:execute-api:us-east-1:123456789012:api/dev/GET/organizations"
	raw, err := json.Marshal(events.APIGatewayCustomAuthorizerRequestTypeRequest{
		Type:      "REQUEST",
		MethodArn: methodARN,
		Headers:   map[string]string{"Authorization": "Bearer " + signedToken(t, validClaims())},
	})
	require.NoError(t, err)

	out, err := dispatch(context.Background(), raw)
	require.NoError(t, err)

	var response events.APIGatewayCustomAuthorizerResponse
	require.NoError(t, json.Unmarshal(out, &response))

	assert.Equal(t, "user#user123", response.PrincipalID)
	assert.Equal(t, "org#orgabc", response.Context["org_id"])
	assert.Equal(t, "user#user123", response.Context["user_id"])

	require.Len(t, response.PolicyDocument.Statement, 1)
	statement := response.PolicyDocument.Statement[0]
	assert.Equal(t, "Allow", statement.Effect)
	assert.Equal(t, []string{methodARN}, statement.Resource)
	assert.Contains(t, statement.Action, "execute-api:Invoke")
}

// TestDispatch_JWTAuthorizerDenies drives hostile and malformed tokens through
// the same dispatch path a real API Gateway authorizer invocation uses. Every
// one must fail with the opaque "Unauthorized" error.
func TestDispatch_JWTAuthorizerDenies(t *testing.T) {
	t.Setenv("FUNCTION_TYPE", "jwt_authorizer")
	t.Setenv("JWT_SECRET", testJWTSecret)

	forged := signedToken(t, validClaims())
	forgedParts := strings.Split(forged, ".")
	require.Len(t, forgedParts, 3)
	// Keep the header and payload but attach a different signature segment.
	forged = forgedParts[0] + "." + forgedParts[1] + "." + base64.RawURLEncoding.EncodeToString([]byte("forged-signature"))

	denied := []struct {
		name    string
		headers map[string]string
	}{
		{"missing authorization header", nil},
		{"empty token after the scheme", map[string]string{"Authorization": "Bearer "}},
		{"not a JWT", map[string]string{"Authorization": "Bearer user123:orgabc"}},
		{"malformed, too few segments", map[string]string{"Authorization": "Bearer a.b"}},
		{"forged signature", map[string]string{"Authorization": "Bearer " + forged}},
		{"unsigned alg none", map[string]string{"Authorization": "Bearer " + unsignedToken(t)}},
		{"wrong secret", map[string]string{"Authorization": "Bearer " + signedTokenWithSecret(t, "other-secret", validClaims())}},
		{"expired", map[string]string{"Authorization": "Bearer " + signedToken(t, auth.Claims{Subject: "user123", OrgID: "orgabc", ExpiresAt: time.Now().Add(-time.Minute).Unix()})}},
		{"empty organization", map[string]string{"Authorization": "Bearer " + signedToken(t, auth.Claims{Subject: "user123", OrgID: "", ExpiresAt: time.Now().Add(time.Hour).Unix()})}},
	}

	for _, tc := range denied {
		t.Run(tc.name, func(t *testing.T) {
			_, err := dispatch(context.Background(), requestAuthorizerEvent(t,
				"arn:aws:execute-api:us-east-1:123456789012:api/dev/GET/organizations", tc.headers))
			require.Error(t, err, "%s: a request without a valid signed token must be denied", tc.name)
			assert.Contains(t, err.Error(), "Unauthorized", "%s", tc.name)
		})
	}
}

func signedTokenWithSecret(t *testing.T, secret string, claims auth.Claims) string {
	t.Helper()
	token, err := auth.Sign(secret, claims)
	require.NoError(t, err)
	return token
}

func TestDispatch_JWTAuthorizerDeniesWhenSecretMissing(t *testing.T) {
	t.Setenv("FUNCTION_TYPE", "jwt_authorizer")
	t.Setenv("JWT_SECRET", "")

	raw, err := json.Marshal(events.APIGatewayCustomAuthorizerRequestTypeRequest{
		MethodArn: "arn:aws:execute-api:us-east-1:123456789012:api/dev/GET/organizations",
		Headers:   map[string]string{"Authorization": "Bearer " + signedToken(t, validClaims())},
	})
	require.NoError(t, err)

	_, err = dispatch(context.Background(), raw)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Unauthorized")
}

func TestDispatch_RejectsUnimplementedFunctionType(t *testing.T) {
	t.Setenv("FUNCTION_TYPE", "not_a_declared_function")

	_, err := dispatch(context.Background(), json.RawMessage(`{}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not_a_declared_function")
}

func TestHTTPRequest_MapsEventToRequest(t *testing.T) {
	event := events.APIGatewayProxyRequest{
		HTTPMethod:            "GET",
		Path:                  "/organizations/org%23demo/users",
		QueryStringParameters: map[string]string{"limit": "10"},
		Headers:               map[string]string{"X-API-Key": "sk_test_key.secret"},
	}

	request, err := httpRequest(context.Background(), event)
	require.NoError(t, err)

	assert.Equal(t, http.MethodGet, request.Method)
	// The encoded tenant separator is decoded onto the path, not treated as a
	// URL fragment, so the router can match it.
	assert.Equal(t, "/organizations/org#demo/users", request.URL.Path)
	assert.Empty(t, request.URL.Fragment)
	assert.Equal(t, "10", request.URL.Query().Get("limit"))
	assert.Equal(t, "sk_test_key.secret", request.Header.Get("X-API-Key"))
}

// TestHTTPRequest_CopiesAuthorizerContext proves the Lambda adapter carries the
// authorizer's tenant IDs onto the request context. The handlers read these
// exact string keys; without the copy the deployed path would lose the caller's
// organization and could not enforce it.
func TestHTTPRequest_CopiesAuthorizerContext(t *testing.T) {
	event := events.APIGatewayProxyRequest{
		HTTPMethod: "GET",
		Path:       "/organizations/org%23demo",
		RequestContext: events.APIGatewayProxyRequestContext{
			Authorizer: map[string]interface{}{
				"user_id": "user#user123",
				"org_id":  "org#orgabc",
				"ignored": 42,
			},
		},
	}

	request, err := httpRequest(context.Background(), event)
	require.NoError(t, err)

	assert.Equal(t, "user#user123", request.Context().Value("user_id"))
	assert.Equal(t, "org#orgabc", request.Context().Value("org_id"))
	// Non-string values are not copied.
	assert.Nil(t, request.Context().Value("ignored"))
}

func TestHTTPRequest_NoAuthorizerContext(t *testing.T) {
	request, err := httpRequest(context.Background(), events.APIGatewayProxyRequest{
		HTTPMethod: "GET",
		Path:       "/organizations/org%23demo",
	})
	require.NoError(t, err)

	assert.Nil(t, request.Context().Value("org_id"))
	assert.Nil(t, request.Context().Value("user_id"))
}

func TestRecorder_CapturesStatusHeadersAndBody(t *testing.T) {
	rec := &recorder{}
	rec.Header().Set("Content-Type", "application/json")
	rec.WriteHeader(http.StatusCreated)
	_, err := rec.Write([]byte(`{"ok":true}`))
	require.NoError(t, err)

	assert.Equal(t, http.StatusCreated, rec.statusCode())
	assert.Equal(t, `{"ok":true}`, rec.body.String())
	assert.Equal(t, "application/json", rec.firstHeaders()["Content-Type"])
}

func TestRecorder_ImplicitWriteIsOK(t *testing.T) {
	rec := &recorder{}
	_, err := rec.Write([]byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.statusCode())
}

func TestAuthorizeAPIKey_DeniesWithoutKey(t *testing.T) {
	// No key is present, so the denial happens before any database access.
	_, err := authorizeAPIKey(events.APIGatewayCustomAuthorizerRequestTypeRequest{
		MethodArn: "arn:aws:execute-api:us-east-1:123456789012:api/dev/GET/organizations",
	})
	require.Error(t, err)
}

// TestDispatch_RequestAuthorizerEventsAllowAndDeny drives the authorizers with
// the event the template actually configures. Both authorizers declare
// FunctionPayloadType: REQUEST, so API Gateway sends the REQUEST payload (path,
// method, headers) and never a TOKEN payload; this test uses that shape, with
// the method ARN of a route that inherits DefaultAuthorizer: JWTAuthorizer.
func TestDispatch_RequestAuthorizerEventsAllowAndDeny(t *testing.T) {
	t.Setenv("FUNCTION_TYPE", "jwt_authorizer")
	t.Setenv("JWT_SECRET", testJWTSecret)

	// A default-authorized route: no Auth override in the template, so it runs
	// through JWTAuthorizer.
	const methodARN = "arn:aws:execute-api:us-east-1:123456789012:api/dev/GET/organizations/org%23demo/users"

	allowed, err := dispatch(context.Background(), requestAuthorizerEvent(t, methodARN, map[string]string{
		"Authorization": "Bearer " + signedToken(t, validClaims()),
	}))
	require.NoError(t, err)

	var response events.APIGatewayCustomAuthorizerResponse
	require.NoError(t, json.Unmarshal(allowed, &response))
	require.Len(t, response.PolicyDocument.Statement, 1)
	assert.Equal(t, "Allow", response.PolicyDocument.Statement[0].Effect)
	assert.Equal(t, []string{methodARN}, response.PolicyDocument.Statement[0].Resource)
	assert.Equal(t, "user#user123", response.PrincipalID)
	assert.Equal(t, "org#orgabc", response.Context["org_id"])

	denied := []struct {
		name    string
		headers map[string]string
	}{
		{"no authorization header at all", nil},
		{"empty token after the scheme", map[string]string{"Authorization": "Bearer "}},
		{"token with an empty tenant", map[string]string{"Authorization": "Bearer " + signedToken(t, auth.Claims{Subject: "user123", OrgID: "", ExpiresAt: time.Now().Add(time.Hour).Unix()})}},
	}
	for _, tc := range denied {
		_, err := dispatch(context.Background(), requestAuthorizerEvent(t, methodARN, tc.headers))
		require.Errorf(t, err, "%s: a request without a usable token must be denied", tc.name)
		assert.Contains(t, err.Error(), "Unauthorized", "%s", tc.name)
	}
}

// requestAuthorizerEvent builds the REQUEST-type authorizer event the template
// produces, including the fields a real invocation carries.
func requestAuthorizerEvent(t *testing.T, methodARN string, headers map[string]string) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(events.APIGatewayCustomAuthorizerRequestTypeRequest{
		Type:                  "REQUEST",
		MethodArn:             methodARN,
		Resource:              "/organizations/org%23demo/users",
		Path:                  "/organizations/org%23demo/users",
		HTTPMethod:            "GET",
		Headers:               headers,
		QueryStringParameters: map[string]string{"limit": "10"},
	})
	require.NoError(t, err)
	return raw
}

// TestTemplate_DeclaresWhatTheEntrypointServes pins the deployment template to
// the entrypoint and to the removals this example makes: the authorizers must
// be REQUEST authorizers (the event shape dispatch decodes), the runtime must
// be the provided one the Makefile builds for, and the functions this example
// does not implement must not be declared at all.
func TestTemplate_DeclaresWhatTheEntrypointServes(t *testing.T) {
	template, err := os.ReadFile(filepath.Join("..", "..", "deployment", "template.yaml"))
	require.NoError(t, err)
	text := string(template)

	assert.Contains(t, text, "Runtime: provided.al2023")
	assert.NotContains(t, text, "go1.x", "go1.x is retired and ignores a bootstrap handler")
	assert.Equal(t, 2, strings.Count(text, "FunctionPayloadType: REQUEST"),
		"both authorizers must declare the REQUEST payload type the entrypoint decodes")
	assert.NotContains(t, text, "StripeSecretKey")
	assert.NotContains(t, text, "BillingFunction")
	assert.NotContains(t, text, "AuditCleanupFunction")
	assert.NotContains(t, text, "audit_cleanup")
	assert.NotContains(t, text, "FUNCTION_TYPE: billing")
}
