// Package auth implements the multi-tenant example's bearer-token format: an
// HS256 JWT whose payload carries the caller's user and organization IDs. It
// uses only the standard library, so the example needs no JWT dependency and
// both the Lambda authorizer and the local server verify tokens the same way.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrInvalidToken is the sentinel every rejection wraps, so a caller can decide
// with errors.Is without depending on (or leaking) the specific reason.
var ErrInvalidToken = errors.New("invalid token")

// Claims is the payload the example's tokens carry. Subject is the user ID and
// OrgID is the organization ID; Verify requires both to be non-empty and the
// token to be unexpired.
type Claims struct {
	Subject   string `json:"sub"`
	OrgID     string `json:"org"`
	IssuedAt  int64  `json:"iat,omitempty"`
	ExpiresAt int64  `json:"exp"`
}

// algorithm is the only signing algorithm Verify accepts. Matching the header
// exactly rejects "none", "RS256", and any other confusion attempt.
const algorithm = "HS256"

// base64URL is the unpadded URL-safe encoding JWT segments use.
var base64URL = base64.RawURLEncoding

// Sign returns a signed HS256 JWT for the given claims: the URL-safe base64 of
// the header, a dot, the URL-safe base64 of the payload, a dot, and the URL-safe
// base64 of the HMAC-SHA256 signature over the first two segments. Segments are
// unpadded ("base64url" without "=").
func Sign(secret string, claims Claims) (string, error) {
	if secret == "" {
		return "", fmt.Errorf("%w: signing secret is empty", ErrInvalidToken)
	}

	header, err := json.Marshal(struct {
		Alg string `json:"alg"`
		Typ string `json:"typ"`
	}{Alg: algorithm, Typ: "JWT"})
	if err != nil {
		return "", fmt.Errorf("encode header: %w", err)
	}

	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("encode claims: %w", err)
	}

	signingInput := base64URL.EncodeToString(header) + "." + base64URL.EncodeToString(payload)
	return signingInput + "." + base64URL.EncodeToString(sign(secret, signingInput)), nil
}

// Verify checks an HS256 token's signature and payload and returns its claims.
// It fails closed: every rejected token wraps ErrInvalidToken, and a caller must
// treat any error as "deny". The checks are, in order:
//
//   - an empty secret is rejected (the authorizer is unconfigured);
//   - the token must have exactly three dot-separated segments;
//   - the header must decode as JSON and its "alg" must be exactly "HS256"
//     (so "none", "RS256", and friends are rejected);
//   - the signature must equal the HMAC-SHA256 of the first two segments,
//     compared with hmac.Equal;
//   - the payload must decode as JSON claims;
//   - "sub" and "org" must be non-empty;
//   - "exp" must be present and in the future.
func Verify(secret, token string) (Claims, error) {
	if secret == "" {
		return Claims{}, fmt.Errorf("%w: signing secret is not configured", ErrInvalidToken)
	}

	segments := strings.Split(token, ".")
	if len(segments) != 3 {
		return Claims{}, fmt.Errorf("%w: token must have three segments", ErrInvalidToken)
	}

	headerJSON, err := base64URL.DecodeString(segments[0])
	if err != nil {
		return Claims{}, fmt.Errorf("%w: header is not valid base64url", ErrInvalidToken)
	}
	var header struct {
		Alg string `json:"alg"`
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return Claims{}, fmt.Errorf("%w: header is not valid JSON", ErrInvalidToken)
	}
	if header.Alg != algorithm {
		return Claims{}, fmt.Errorf("%w: unsupported algorithm %q", ErrInvalidToken, header.Alg)
	}

	providedSignature, err := base64URL.DecodeString(segments[2])
	if err != nil {
		return Claims{}, fmt.Errorf("%w: signature is not valid base64url", ErrInvalidToken)
	}
	if !hmac.Equal(sign(secret, segments[0]+"."+segments[1]), providedSignature) {
		return Claims{}, fmt.Errorf("%w: signature does not match", ErrInvalidToken)
	}

	// Only after the signature is proven valid is the payload trusted.
	payloadJSON, err := base64URL.DecodeString(segments[1])
	if err != nil {
		return Claims{}, fmt.Errorf("%w: payload is not valid base64url", ErrInvalidToken)
	}
	var claims Claims
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return Claims{}, fmt.Errorf("%w: payload is not valid JSON", ErrInvalidToken)
	}
	if strings.TrimSpace(claims.Subject) == "" {
		return Claims{}, fmt.Errorf("%w: subject is empty", ErrInvalidToken)
	}
	if strings.TrimSpace(claims.OrgID) == "" {
		return Claims{}, fmt.Errorf("%w: organization is empty", ErrInvalidToken)
	}
	if claims.ExpiresAt == 0 {
		return Claims{}, fmt.Errorf("%w: expiry is missing", ErrInvalidToken)
	}
	if claims.ExpiresAt <= time.Now().Unix() {
		return Claims{}, fmt.Errorf("%w: token is expired", ErrInvalidToken)
	}

	return claims, nil
}

// sign returns the raw HMAC-SHA256 of the signing input under the secret.
func sign(secret, signingInput string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signingInput))
	return mac.Sum(nil)
}
