package utils

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

// JWTClaims represents the custom claims in our JWT tokens
type JWTClaims struct {
	MerchantID  string   `json:"merchant_id"`
	Email       string   `json:"email"`
	Issuer      string   `json:"iss,omitempty"`
	Subject     string   `json:"sub,omitempty"`
	JWTID       string   `json:"jti,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
	Audience    []string `json:"aud,omitempty"`
	ExpiresAt   int64    `json:"exp,omitempty"`
	NotBefore   int64    `json:"nbf,omitempty"`
	IssuedAt    int64    `json:"iat,omitempty"`
}

// SimpleJWTValidator handles JWT validation with HMAC
type SimpleJWTValidator struct {
	issuer    string
	audience  string
	secretKey []byte
}

// NewSimpleJWTValidator creates a new JWT validator with HMAC-SHA256
func NewSimpleJWTValidator(secretKey string, issuer, audience string) *SimpleJWTValidator {
	return &SimpleJWTValidator{
		secretKey: []byte(secretKey),
		issuer:    issuer,
		audience:  audience,
	}
}

// ValidateToken validates a JWT token and returns the claims
func (v *SimpleJWTValidator) ValidateToken(tokenString string) (*JWTClaims, error) {
	// Split token into parts
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid token format")
	}

	// Decode header
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("failed to decode header: %w", err)
	}

	var header map[string]interface{}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, fmt.Errorf("failed to parse header: %w", err)
	}

	// Check algorithm
	alg, ok := header["alg"].(string)
	if !ok || alg != "HS256" {
		return nil, fmt.Errorf("unsupported algorithm: %v", alg)
	}

	// Verify signature
	message := parts[0] + "." + parts[1]
	expectedSig := v.computeSignature(message)
	if parts[2] != expectedSig {
		return nil, errors.New("invalid signature")
	}

	// Decode claims
	claimsBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to decode claims: %w", err)
	}

	var claims JWTClaims
	if err := json.Unmarshal(claimsBytes, &claims); err != nil {
		return nil, fmt.Errorf("failed to parse claims: %w", err)
	}

	// Validate claims
	if err := v.validateClaims(&claims); err != nil {
		return nil, err
	}

	return &claims, nil
}

// computeSignature computes HMAC-SHA256 signature
func (v *SimpleJWTValidator) computeSignature(message string) string {
	h := hmac.New(sha256.New, v.secretKey)
	h.Write([]byte(message))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

// validateClaims validates the JWT claims
func (v *SimpleJWTValidator) validateClaims(claims *JWTClaims) error {
	now := time.Now().Unix()

	// Check expiration
	if claims.ExpiresAt > 0 && now > claims.ExpiresAt {
		return errors.New("token expired")
	}

	// Check not before
	if claims.NotBefore > 0 && now < claims.NotBefore {
		return errors.New("token not yet valid")
	}

	// Check issuer
	if v.issuer != "" && claims.Issuer != v.issuer {
		return fmt.Errorf("invalid issuer: expected %s, got %s", v.issuer, claims.Issuer)
	}

	// Check audience
	if v.audience != "" && len(claims.Audience) > 0 {
		found := false
		for _, aud := range claims.Audience {
			if aud == v.audience {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("invalid audience: %s not found", v.audience)
		}
	}

	// Check merchant ID
	if claims.MerchantID == "" {
		return errors.New("missing merchant_id in token")
	}

	return nil
}

// ExtractMerchantID extracts the merchant ID from a JWT token
func (v *SimpleJWTValidator) ExtractMerchantID(tokenString string) (string, error) {
	claims, err := v.ValidateToken(tokenString)
	if err != nil {
		return "", err
	}
	return claims.MerchantID, nil
}

// ExtractTokenFromHeader extracts the JWT token from the Authorization header
func ExtractTokenFromHeader(authHeader string) (string, error) {
	if authHeader == "" {
		return "", errors.New("authorization header is empty")
	}

	// Check for Bearer scheme
	const bearerScheme = "Bearer "
	if !strings.HasPrefix(authHeader, bearerScheme) {
		return "", errors.New("invalid authorization header format")
	}

	token := strings.TrimPrefix(authHeader, bearerScheme)
	if token == "" {
		return "", errors.New("token is empty")
	}

	return token, nil
}

// ValidateAndExtractMerchantID is a convenience function that extracts and validates the token
func ValidateAndExtractMerchantID(authHeader string, validator *SimpleJWTValidator) (string, error) {
	token, err := ExtractTokenFromHeader(authHeader)
	if err != nil {
		return "", err
	}

	return validator.ExtractMerchantID(token)
}

// TokenError represents a JWT validation error with details
type TokenError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

func (e *TokenError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Common token error codes
const (
	TokenErrorInvalid     = "invalid_token"
	TokenErrorExpired     = "token_expired"
	TokenErrorMalformed   = "malformed_token"
	TokenErrorMissing     = "missing_token"
	TokenErrorPermissions = "insufficient_permissions"
)

// NewTokenError creates a new token error
func NewTokenError(code, message, details string) *TokenError {
	return &TokenError{
		Code:    code,
		Message: message,
		Details: details,
	}
}
