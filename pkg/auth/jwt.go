package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWTValidator validates JWT tokens using JWKS
type JWTValidator struct {
	jwksURL string
	issuer  string
	keys    map[string]interface{}
	keysMu  sync.RWMutex
	client  *http.Client
}

// JWKS represents a JSON Web Key Set
type JWKS struct {
	Keys []JWK `json:"keys"`
}

// JWK represents a JSON Web Key
type JWK struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// NewJWTValidator creates a new JWT validator
func NewJWTValidator(jwksURL, issuer string) *JWTValidator {
	return &JWTValidator{
		jwksURL: jwksURL,
		issuer:  issuer,
		keys:    make(map[string]interface{}),
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// ValidateToken validates a JWT token and returns the claims
func (v *JWTValidator) ValidateToken(tokenString string) (jwt.MapClaims, error) {
	// Parse the token
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Validate the algorithm
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		// Get the key ID
		kid, ok := token.Header["kid"].(string)
		if !ok {
			return nil, fmt.Errorf("missing kid in token header")
		}

		// Get the key
		key, err := v.getKey(kid)
		if err != nil {
			return nil, err
		}
		return key, nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("invalid claims type")
	}

	// Validate issuer if configured
	if v.issuer != "" {
		iss, ok := claims["iss"].(string)
		if !ok || iss != v.issuer {
			return nil, fmt.Errorf("invalid issuer")
		}
	}

	return claims, nil
}

// getKey retrieves a key by ID, refreshing from JWKS if needed
func (v *JWTValidator) getKey(kid string) (interface{}, error) {
	v.keysMu.RLock()
	key, exists := v.keys[kid]
	v.keysMu.RUnlock()

	if exists {
		return key, nil
	}

	// Refresh keys from JWKS
	if err := v.refreshKeys(); err != nil {
		return nil, err
	}

	v.keysMu.RLock()
	key, exists = v.keys[kid]
	v.keysMu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("key not found: %s", kid)
	}

	return key, nil
}

// refreshKeys fetches and parses the JWKS
func (v *JWTValidator) refreshKeys() error {
	if v.jwksURL == "" {
		return fmt.Errorf("JWKS URL not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := v.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch JWKS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("JWKS endpoint returned status %d", resp.StatusCode)
	}

	var jwks JWKS
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return fmt.Errorf("failed to decode JWKS: %w", err)
	}

	v.keysMu.Lock()
	defer v.keysMu.Unlock()

	for _, key := range jwks.Keys {
		if key.Kty == "RSA" {
			pubKey, err := parseRSAPublicKey(key.N, key.E)
			if err != nil {
				continue // Skip invalid keys
			}
			v.keys[key.Kid] = pubKey
		}
	}

	return nil
}

// parseRSAPublicKey parses RSA public key components from base64url-encoded strings
func parseRSAPublicKey(nStr, eStr string) (*rsa.PublicKey, error) {
	// Decode base64url-encoded modulus (n)
	nBytes, err := base64.RawURLEncoding.DecodeString(nStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode modulus: %w", err)
	}

	// Decode base64url-encoded exponent (e)
	eBytes, err := base64.RawURLEncoding.DecodeString(eStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode exponent: %w", err)
	}

	// Convert to big.Int for modulus
	n := new(big.Int).SetBytes(nBytes)

	// Convert exponent bytes to int
	e := int(new(big.Int).SetBytes(eBytes).Int64())

	return &rsa.PublicKey{N: n, E: e}, nil
}

// IsConfigured returns true if JWKS validation is configured
func (v *JWTValidator) IsConfigured() bool {
	return v.jwksURL != ""
}
