package main

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/stretchr/testify/require"
)

const testMethodArn = "arn:aws:execute-api:us-east-1:123456789012:api/dev/POST/posts"

// TestDeclaredAuthorizerAcceptsDeliveredREQUESTEvent feeds the exact event JSON
// API Gateway delivers to the declared REQUEST authorizer type. The struct
// tests in main_test.go build the same shape programmatically; this test pins
// the wire shape so a template/handler mismatch is caught.
func TestDeclaredAuthorizerAcceptsDeliveredREQUESTEvent(t *testing.T) {
	t.Setenv(secretEnv, "test-secret")

	token := signToken(t, "test-secret",
		map[string]any{"alg": "HS256", "typ": "JWT"},
		map[string]any{"sub": "author#1", "role": "admin", "exp": time.Now().Add(time.Hour).Unix()},
	)

	raw := `{
		"type": "REQUEST",
		"methodArn": "` + testMethodArn + `",
		"resource": "/posts",
		"path": "/posts",
		"httpMethod": "POST",
		"headers": {"Authorization": "Bearer ` + token + `"},
		"queryStringParameters": {},
		"pathParameters": {},
		"stageVariables": {},
		"requestContext": {}
	}`

	var event events.APIGatewayCustomAuthorizerRequestTypeRequest
	require.NoError(t, json.Unmarshal([]byte(raw), &event))

	response, err := HandleRequest(context.Background(), event)
	require.NoError(t, err)
	require.Equal(t, "author#1", response.PrincipalID)
	require.Equal(t, "author#1", response.Context[authorIDContextKey])
	require.Equal(t, "admin", response.Context[roleContextKey])
	require.Equal(t, testMethodArn, response.PolicyDocument.Statement[0].Resource[0])
	require.Equal(t, "Allow", response.PolicyDocument.Statement[0].Effect)
}

// TestDeclaredAuthorizerDeniesTamperedDeliveredREQUESTEvent proves the same
// wire shape fails closed when the token signature does not verify.
func TestDeclaredAuthorizerDeniesTamperedDeliveredREQUESTEvent(t *testing.T) {
	t.Setenv(secretEnv, "test-secret")

	token := signToken(t, "other-secret",
		map[string]any{"alg": "HS256", "typ": "JWT"},
		map[string]any{"sub": "author#1", "exp": time.Now().Add(time.Hour).Unix()},
	)

	raw := `{
		"type": "REQUEST",
		"methodArn": "` + testMethodArn + `",
		"headers": {"Authorization": "Bearer ` + token + `"},
		"requestContext": {}
	}`

	var event events.APIGatewayCustomAuthorizerRequestTypeRequest
	require.NoError(t, json.Unmarshal([]byte(raw), &event))

	_, err := HandleRequest(context.Background(), event)
	require.EqualError(t, err, "Unauthorized")
}

// TestTemplateDeclaresMatchingREQUESTAuthorizer is the structural half of the
// deployment contract: the handler above decodes a REQUEST event, so the SAM
// template must declare a REQUEST authorizer with the required wiring. If the
// template regresses to TOKEN, this test fails before a deploy can hard-deny
// every protected route.
func TestTemplateDeclaresMatchingREQUESTAuthorizer(t *testing.T) {
	body, err := os.ReadFile("../deployment/sam-template.yaml")
	require.NoError(t, err)
	template := string(body)

	require.Contains(t, template, "Type: REQUEST")
	require.Contains(t, template, "IdentitySource: method.request.header.Authorization")
	require.Contains(t, template, "RestApiId: !Ref ServerlessRestApi")
	require.Contains(t, template, "CodeUri: ../authorizer.zip")
	require.NotContains(t, template, "Type: TOKEN")
}

// TestTOKENEventShapeDoesNotAuthorize documents the inverse contract. A TOKEN
// event carries authorizationToken and no headers, so the REQUEST handler
// fails closed. This is the event that would be delivered if the template
// regressed to Type: TOKEN; the structural test above prevents that.
func TestTOKENEventShapeDoesNotAuthorize(t *testing.T) {
	t.Setenv(secretEnv, "test-secret")

	token := signToken(t, "test-secret",
		map[string]any{"alg": "HS256", "typ": "JWT"},
		map[string]any{"sub": "author#1", "exp": time.Now().Add(time.Hour).Unix()},
	)

	raw := `{
		"type": "TOKEN",
		"authorizationToken": "Bearer ` + token + `",
		"methodArn": "` + testMethodArn + `"
	}`

	var event events.APIGatewayCustomAuthorizerRequestTypeRequest
	require.NoError(t, json.Unmarshal([]byte(raw), &event))

	_, err := HandleRequest(context.Background(), event)
	require.EqualError(t, err, "Unauthorized")
}
