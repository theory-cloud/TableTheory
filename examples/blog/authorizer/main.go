// Command authorizer is the API Gateway TOKEN authorizer used by the blog
// example's protected routes.
//
// It verifies an HS256 bearer token signed with JWT_SECRET and returns the
// caller's author ID and role in the authorizer context:
//
//	requestContext.authorizer.authorId
//	requestContext.authorizer.role
//
// Handlers treat that context as the only trusted identity source. The
// X-Author-ID / X-Author-Role headers are never trusted on a deployed stage;
// see examples/blog/README.md.
package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

const (
	authorIDContextKey = "authorId"
	roleContextKey     = "role"

	algorithm = "HS256"
	// secretEnv matches the shared JWT_SECRET the SAM template resolves from
	// Secrets Manager for every function in this stack.
	secretEnv = "JWT_SECRET"
)

// claims is the token payload the blog authorizer accepts. ExpiresAt is
// mandatory: a token without an expiry is rejected rather than treated as
// non-expiring.
type claims struct {
	Subject   string `json:"sub"`
	Role      string `json:"role"`
	ExpiresAt int64  `json:"exp"`
	NotBefore int64  `json:"nbf,omitempty"`
}

func main() {
	lambda.Start(HandleRequest)
}

// HandleRequest verifies the bearer token and returns an allow policy that
// carries the authenticated identity in the authorizer context. Every failure
// is reported as a bare "Unauthorized" so a caller learns nothing about why.
func HandleRequest(_ context.Context, event events.APIGatewayCustomAuthorizerRequestTypeRequest) (events.APIGatewayCustomAuthorizerResponse, error) {
	secret := strings.TrimSpace(os.Getenv(secretEnv))
	if secret == "" {
		return events.APIGatewayCustomAuthorizerResponse{}, errors.New("Unauthorized")
	}

	parsed, err := verifyToken(secret, bearerToken(event.Headers), time.Now())
	if err != nil {
		return events.APIGatewayCustomAuthorizerResponse{}, errors.New("Unauthorized")
	}

	role := strings.TrimSpace(parsed.Role)
	if role == "" {
		role = "author"
	}

	return events.APIGatewayCustomAuthorizerResponse{
		PrincipalID: parsed.Subject,
		PolicyDocument: events.APIGatewayCustomAuthorizerPolicy{
			Version: "2012-10-17",
			Statement: []events.IAMPolicyStatement{
				{
					Action:   []string{"execute-api:Invoke"},
					Effect:   "Allow",
					Resource: []string{event.MethodArn},
				},
			},
		},
		Context: map[string]any{
			authorIDContextKey: parsed.Subject,
			roleContextKey:     role,
		},
	}, nil
}

func bearerToken(headers map[string]string) string {
	value := headerValue(headers, "Authorization")
	value = strings.TrimSpace(value)
	if len(value) >= 7 && strings.EqualFold(value[:7], "Bearer ") {
		return strings.TrimSpace(value[7:])
	}
	return ""
}

func headerValue(headers map[string]string, name string) string {
	if value := headers[name]; value != "" {
		return value
	}
	for key, value := range headers {
		if strings.EqualFold(key, name) {
			return value
		}
	}
	return ""
}

func verifyToken(secret, token string, now time.Time) (*claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid token format")
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, errors.New("invalid token header")
	}
	var header struct {
		Alg string `json:"alg"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, errors.New("invalid token header")
	}
	// Matching the algorithm exactly rejects "none", RS256, and every other
	// key-confusion attempt.
	if header.Alg != algorithm {
		return nil, errors.New("unsupported token algorithm")
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	expected := mac.Sum(nil)

	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, errors.New("invalid token signature")
	}
	if !hmac.Equal(expected, signature) {
		return nil, errors.New("invalid token signature")
	}

	claimsBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("invalid token claims")
	}
	var parsed claims
	if err := json.Unmarshal(claimsBytes, &parsed); err != nil {
		return nil, errors.New("invalid token claims")
	}

	if strings.TrimSpace(parsed.Subject) == "" {
		return nil, errors.New("token missing subject")
	}
	if parsed.ExpiresAt <= 0 {
		return nil, errors.New("token missing expiration")
	}
	if now.Unix() >= parsed.ExpiresAt {
		return nil, errors.New("token expired")
	}
	if parsed.NotBefore > 0 && now.Unix() < parsed.NotBefore {
		return nil, errors.New("token not yet valid")
	}

	parsed.Subject = strings.TrimSpace(parsed.Subject)
	return &parsed, nil
}
