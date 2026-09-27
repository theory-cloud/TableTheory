package main

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests exercise the entrypoint's own adapter and authorizer code — the
// parts that do not need a database. The API path's routing is shared with the
// local server, which the example's DynamoDB-backed tests cover.

func TestDispatch_JWTAuthorizerAllowsBearerToken(t *testing.T) {
	t.Setenv("FUNCTION_TYPE", "jwt_authorizer")

	const methodARN = "arn:aws:execute-api:us-east-1:123456789012:api/dev/GET/organizations"
	raw, err := json.Marshal(events.APIGatewayCustomAuthorizerRequestTypeRequest{
		Type:      "REQUEST",
		MethodArn: methodARN,
		Headers:   map[string]string{"Authorization": "Bearer user123:orgabc"},
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

func TestDispatch_JWTAuthorizerDefaultsOrgWhenTokenHasNoColon(t *testing.T) {
	t.Setenv("FUNCTION_TYPE", "jwt_authorizer")

	raw, err := json.Marshal(events.APIGatewayCustomAuthorizerRequestTypeRequest{
		MethodArn: "arn:aws:execute-api:us-east-1:123456789012:api/dev/GET/organizations",
		// Lower-case header, to prove the lookup is case-insensitive like API
		// Gateway's.
		Headers: map[string]string{"authorization": "user123"},
	})
	require.NoError(t, err)

	out, err := dispatch(context.Background(), raw)
	require.NoError(t, err)

	var response events.APIGatewayCustomAuthorizerResponse
	require.NoError(t, json.Unmarshal(out, &response))
	assert.Equal(t, "user#user123", response.PrincipalID)
	assert.Equal(t, "org#demo", response.Context["org_id"])
}

func TestDispatch_JWTAuthorizerDeniesMissingToken(t *testing.T) {
	t.Setenv("FUNCTION_TYPE", "jwt_authorizer")

	raw, err := json.Marshal(events.APIGatewayCustomAuthorizerRequestTypeRequest{
		MethodArn: "arn:aws:execute-api:us-east-1:123456789012:api/dev/GET/organizations",
	})
	require.NoError(t, err)

	_, err = dispatch(context.Background(), raw)
	require.Error(t, err)
}

func TestDispatch_RejectsUnimplementedFunctionType(t *testing.T) {
	t.Setenv("FUNCTION_TYPE", "billing")

	_, err := dispatch(context.Background(), json.RawMessage(`{}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "billing")
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
