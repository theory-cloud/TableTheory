package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/stretchr/testify/require"
)

func signToken(t *testing.T, secret string, header map[string]any, payload map[string]any) string {
	t.Helper()

	headerJSON, err := json.Marshal(header)
	require.NoError(t, err)
	payloadJSON, err := json.Marshal(payload)
	require.NoError(t, err)

	signingInput := base64.RawURLEncoding.EncodeToString(headerJSON) + "." +
		base64.RawURLEncoding.EncodeToString(payloadJSON)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signingInput))

	return signingInput + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func authorizerEvent(token string) events.APIGatewayCustomAuthorizerRequestTypeRequest {
	return events.APIGatewayCustomAuthorizerRequestTypeRequest{
		MethodArn: "arn:aws:execute-api:us-east-1:123456789012:api/dev/POST/posts",
		Headers:   map[string]string{"authorization": "Bearer " + token},
	}
}

func TestHandleRequestAllowsSignedToken(t *testing.T) {
	t.Setenv(secretEnv, "test-secret")

	token := signToken(t, "test-secret",
		map[string]any{"alg": "HS256", "typ": "JWT"},
		map[string]any{"sub": "author#1", "role": "admin", "exp": time.Now().Add(time.Hour).Unix()},
	)

	response, err := HandleRequest(context.Background(), authorizerEvent(token))
	require.NoError(t, err)
	require.Equal(t, "author#1", response.PrincipalID)
	require.Equal(t, "author#1", response.Context[authorIDContextKey])
	require.Equal(t, "admin", response.Context[roleContextKey])
	require.Len(t, response.PolicyDocument.Statement, 1)
	require.Equal(t, "Allow", response.PolicyDocument.Statement[0].Effect)
}

func TestHandleRequestDefaultsRoleToAuthor(t *testing.T) {
	t.Setenv(secretEnv, "test-secret")

	token := signToken(t, "test-secret",
		map[string]any{"alg": "HS256", "typ": "JWT"},
		map[string]any{"sub": "author#2", "exp": time.Now().Add(time.Hour).Unix()},
	)

	response, err := HandleRequest(context.Background(), authorizerEvent(token))
	require.NoError(t, err)
	require.Equal(t, "author", response.Context[roleContextKey])
}

func TestHandleRequestDeniesInvalidTokens(t *testing.T) {
	t.Setenv(secretEnv, "test-secret")

	valid := map[string]any{"alg": "HS256", "typ": "JWT"}
	cases := map[string]string{
		"empty token": "",
		"expired": signToken(t, "test-secret", valid,
			map[string]any{"sub": "author#1", "exp": time.Now().Add(-time.Minute).Unix()}),
		"missing expiry": signToken(t, "test-secret", valid, map[string]any{"sub": "author#1"}),
		"missing subject": signToken(t, "test-secret", valid,
			map[string]any{"exp": time.Now().Add(time.Hour).Unix()}),
		"wrong secret": signToken(t, "other-secret", valid,
			map[string]any{"sub": "author#1", "exp": time.Now().Add(time.Hour).Unix()}),
		"alg none": signToken(t, "test-secret", map[string]any{"alg": "none"},
			map[string]any{"sub": "author#1", "exp": time.Now().Add(time.Hour).Unix()}),
		"malformed": "not.a.jwt",
	}

	for name, token := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := HandleRequest(context.Background(), authorizerEvent(token))
			require.Error(t, err)
			require.EqualError(t, err, "Unauthorized")
		})
	}
}

func TestHandleRequestDeniesWithoutSecretConfigured(t *testing.T) {
	t.Setenv(secretEnv, "")

	token := signToken(t, "test-secret",
		map[string]any{"alg": "HS256", "typ": "JWT"},
		map[string]any{"sub": "author#1", "exp": time.Now().Add(time.Hour).Unix()},
	)

	_, err := HandleRequest(context.Background(), authorizerEvent(token))
	require.Error(t, err)
}
