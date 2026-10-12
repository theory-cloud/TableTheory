package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSecret = "test-secret"

func testClaims() Claims {
	now := time.Now()
	return Claims{
		Subject:   "user#user123",
		OrgID:     "org#orgabc",
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(time.Hour).Unix(),
	}
}

func TestSignAndVerifyRoundTrip(t *testing.T) {
	token, err := Sign(testSecret, testClaims())
	require.NoError(t, err)

	// Segments are unpadded base64url and there are exactly three.
	require.Len(t, strings.Split(token, "."), 3)
	assert.NotContains(t, token, "=")

	claims, err := Verify(testSecret, token)
	require.NoError(t, err)
	assert.Equal(t, "user#user123", claims.Subject)
	assert.Equal(t, "org#orgabc", claims.OrgID)
}

func TestSignRejectsEmptySecret(t *testing.T) {
	_, err := Sign("", testClaims())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidToken)
}

// rawToken builds a token by hand so the hostile cases can produce a header,
// payload, or signature that Sign would never emit. It signs with the given
// secret, letting tests construct a token under the wrong key.
func rawToken(t *testing.T, header, payload map[string]any, secret string) string {
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

func futureExp() int64 { return time.Now().Add(time.Hour).Unix() }

func TestVerifyRejects(t *testing.T) {
	valid := testClaims()
	validToken, err := Sign(testSecret, valid)
	require.NoError(t, err)

	tests := []struct {
		name  string
		token string
	}{
		{
			name:  "tampered payload keeps the old signature",
			token: forgePayload(t, validToken),
		},
		{
			// Same header and payload, but a signature over different bytes.
			name:  "tampered signature",
			token: replaceSignature(validToken, []byte("not-the-right-signature-bytes")),
		},
		{
			name: "unsigned alg none",
			token: base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`)) + "." +
				base64.RawURLEncoding.EncodeToString(mustJSON(t, map[string]any{"sub": "user#user123", "org": "org#orgabc", "exp": futureExp()})) + ".",
		},
		{
			name:  "wrong secret",
			token: rawToken(t, map[string]any{"alg": "HS256", "typ": "JWT"}, map[string]any{"sub": "user#user123", "org": "org#orgabc", "exp": futureExp()}, "other-secret"),
		},
		{
			name:  "wrong algorithm header with a valid HMAC",
			token: rawToken(t, map[string]any{"alg": "RS256", "typ": "JWT"}, map[string]any{"sub": "user#user123", "org": "org#orgabc", "exp": futureExp()}, testSecret),
		},
		{
			name:  "expired",
			token: mustSign(t, Claims{Subject: "user#user123", OrgID: "org#orgabc", ExpiresAt: time.Now().Add(-time.Minute).Unix()}),
		},
		{
			name:  "missing expiry",
			token: mustSign(t, Claims{Subject: "user#user123", OrgID: "org#orgabc"}),
		},
		{
			name:  "empty subject",
			token: mustSign(t, Claims{Subject: "", OrgID: "org#orgabc", ExpiresAt: futureExp()}),
		},
		{
			name:  "empty organization",
			token: mustSign(t, Claims{Subject: "user#user123", OrgID: "", ExpiresAt: futureExp()}),
		},
		{
			name:  "too few segments",
			token: "only.two",
		},
		{
			name:  "too many segments",
			token: "a.b.c.d",
		},
		{
			name:  "header is not valid base64",
			token: "!!!." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"user#user123"}`)) + ".sig",
		},
		{
			name: "header is not valid JSON",
			token: base64.RawURLEncoding.EncodeToString([]byte("not-json")) + "." +
				base64.RawURLEncoding.EncodeToString([]byte(`{}`)) + ".sig",
		},
		{
			name:  "empty token",
			token: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Verify(testSecret, tc.token)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalidToken)
		})
	}
}

func TestVerifyRejectsEmptySecret(t *testing.T) {
	token, err := Sign(testSecret, testClaims())
	require.NoError(t, err)

	_, err = Verify("", token)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidToken)
	assert.True(t, errors.Is(err, ErrInvalidToken))
}

// forgePayload re-encodes a valid token's payload (granting a different org)
// while leaving the original signature in place.
func forgePayload(t *testing.T, token string) string {
	t.Helper()
	segments := strings.Split(token, ".")
	require.Len(t, segments, 3)
	payload := base64.RawURLEncoding.EncodeToString(mustJSON(t, map[string]any{
		"sub": "user#user123",
		"org": "org#victim",
		"exp": futureExp(),
	}))
	return segments[0] + "." + payload + "." + segments[2]
}

// replaceSignature keeps a token's header and payload but swaps in a different
// signature segment, encoded the same way a real one would be.
func replaceSignature(token string, signature []byte) string {
	segments := strings.Split(token, ".")
	return segments[0] + "." + segments[1] + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func mustSign(t *testing.T, claims Claims) string {
	t.Helper()
	token, err := Sign(testSecret, claims)
	require.NoError(t, err)
	return token
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	return encoded
}
