package main

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
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
	var document yaml.Node
	require.NoError(t, yaml.Unmarshal(body, &document))
	root := document.Content[0]

	resources := templateMapping(t, root, "Resources")
	api := templateMapping(t, resources, "ServerlessRestApi")
	require.Equal(t, "AWS::Serverless::Api", templateScalar(t, api, "Type"))

	apiProperties := templateMapping(t, api, "Properties")
	authorizers := templateMapping(t, templateMapping(t, apiProperties, "Auth"), "Authorizers")
	authorizer := templateMapping(t, authorizers, "BlogAuthorizer")
	require.Equal(t, "REQUEST", templateScalar(t, authorizer, "FunctionPayloadType"))
	require.Equal(t, "Authorization", templateScalar(t, templateMapping(t, authorizer, "Identity"), "Header"))

	functionArn := templateMapping(t, authorizer, "FunctionArn")
	require.Equal(t, "!GetAtt", functionArn.Tag)
	require.Equal(t, "AuthorizerFunction.Arn", functionArn.Value)
	authorizerFunction := templateMapping(t, resources, "AuthorizerFunction")
	require.Equal(t,
		"../authorizer.zip",
		templateScalar(t, templateMapping(t, authorizerFunction, "Properties"), "CodeUri"),
	)

	protectedRoutes := 0
	for i := 0; i+1 < len(resources.Content); i += 2 {
		resource := resources.Content[i+1]
		require.NotEqual(t, "AWS::ApiGateway::Authorizer", templateScalar(t, resource, "Type"),
			"the authorizer must be declared in the API Auth.Authorizers block, not as a raw authorizer")

		if templateScalar(t, resource, "Type") != "AWS::Serverless::Function" {
			continue
		}
		events, ok := templateOptionalMapping(t, templateMapping(t, resource, "Properties"), "Events")
		if !ok {
			continue
		}
		for j := 0; j+1 < len(events.Content); j += 2 {
			event := events.Content[j+1]
			if templateScalar(t, event, "Type") != "Api" {
				continue
			}
			eventProperties := templateMapping(t, event, "Properties")
			apiRef := templateMapping(t, eventProperties, "RestApiId")
			require.Equal(t, "!Ref", apiRef.Tag)
			require.Equal(t, "ServerlessRestApi", apiRef.Value)
			auth, protected := templateOptionalMapping(t, eventProperties, "Auth")
			if !protected {
				continue
			}
			require.Equal(t, "BlogAuthorizer", templateScalar(t, auth, "Authorizer"))
			protectedRoutes++
		}
	}
	require.Equal(t, 5, protectedRoutes, "every protected route must name the declared REQUEST authorizer")
}

func templateMapping(t *testing.T, node *yaml.Node, key string) *yaml.Node {
	t.Helper()
	value, ok := templateOptionalMapping(t, node, key)
	require.Truef(t, ok, "template key %q is missing", key)
	return value
}

func templateOptionalMapping(t *testing.T, node *yaml.Node, key string) (*yaml.Node, bool) {
	t.Helper()
	require.NotNil(t, node)
	require.Equal(t, yaml.MappingNode, node.Kind)
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1], true
		}
	}
	return nil, false
}

func templateScalar(t *testing.T, node *yaml.Node, key string) string {
	t.Helper()
	value := templateMapping(t, node, key)
	require.Equal(t, yaml.ScalarNode, value.Kind)
	return value.Value
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
