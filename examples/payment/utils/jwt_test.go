package utils

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func signTestToken(t *testing.T, secret string, claims JWTClaims) string {
	t.Helper()

	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload, err := json.Marshal(claims)
	require.NoError(t, err)
	body := header + "." + base64.RawURLEncoding.EncodeToString(payload)

	mac := hmac.New(sha256.New, []byte(secret))
	_, err = mac.Write([]byte(body))
	require.NoError(t, err)
	return body + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func TestSimpleJWTValidatorRequiresExpiration(t *testing.T) {
	const secret = "test-secret"
	validator := NewSimpleJWTValidator(secret, "", "")

	_, err := validator.ValidateToken(signTestToken(t, secret, JWTClaims{MerchantID: "merchant-1"}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing expiration")

	valid := signTestToken(t, secret, JWTClaims{
		MerchantID: "merchant-1",
		ExpiresAt:  time.Now().Add(time.Minute).Unix(),
	})
	claims, err := validator.ValidateToken(valid)
	require.NoError(t, err)
	assert.Equal(t, "merchant-1", claims.MerchantID)

	expired := signTestToken(t, secret, JWTClaims{
		MerchantID: "merchant-1",
		ExpiresAt:  time.Now().Add(-time.Minute).Unix(),
	})
	_, err = validator.ValidateToken(expired)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token expired")
}

func TestSimpleJWTValidatorRequiresConfiguredAudience(t *testing.T) {
	const secret = "test-secret"
	validator := NewSimpleJWTValidator(secret, "", "payments-api")

	missingAudience := signTestToken(t, secret, JWTClaims{
		MerchantID: "merchant-1",
		ExpiresAt:  time.Now().Add(time.Minute).Unix(),
	})
	_, err := validator.ValidateToken(missingAudience)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing audience")

	wrongAudience := signTestToken(t, secret, JWTClaims{
		MerchantID: "merchant-1",
		Audience:   []string{"other-api"},
		ExpiresAt:  time.Now().Add(time.Minute).Unix(),
	})
	_, err = validator.ValidateToken(wrongAudience)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid audience")

	valid := signTestToken(t, secret, JWTClaims{
		MerchantID: "merchant-1",
		Audience:   []string{"payments-api"},
		ExpiresAt:  time.Now().Add(time.Minute).Unix(),
	})
	_, err = validator.ValidateToken(valid)
	require.NoError(t, err)
}

func TestSimpleJWTValidatorRejectsInvalidSignature(t *testing.T) {
	validator := NewSimpleJWTValidator("expected-secret", "", "")
	token := signTestToken(t, "other-secret", JWTClaims{
		MerchantID: "merchant-1",
		ExpiresAt:  time.Now().Add(time.Minute).Unix(),
	})

	_, err := validator.ValidateToken(token)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid signature")
}
