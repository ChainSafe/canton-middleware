// SPDX-License-Identifier: Apache-2.0

package jwt

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"
)

const jwksFetchTimeout = 10 * time.Second

// Validator verifies RS256 JWTs. Keys come either from a single in-process public
// key (NewValidatorWithKey) or from a remote JWKS endpoint fetched on demand and
// cached by key id (NewValidator).
type Validator struct {
	jwksURL string
	issuer  string
	keys    map[string]any
	keysMu  sync.RWMutex
	client  *http.Client
}

// NewValidator creates a Validator that fetches signing keys from a JWKS endpoint.
// If the issuer is non-empty, tokens must carry a matching "iss" claim.
func NewValidator(jwksURL, issuer string) *Validator {
	return &Validator{
		jwksURL: jwksURL,
		issuer:  issuer,
		keys:    make(map[string]any),
		client:  &http.Client{Timeout: jwksFetchTimeout},
	}
}

// NewValidatorWithKey creates a Validator that trusts a single in-process RSA
// public key, keyed by kid, and performs no network fetch. The token issuer's own
// process uses this to validate the tokens it mints; external services validate the
// same tokens via the published JWKS URL using NewValidator.
func NewValidatorWithKey(kid string, pub *rsa.PublicKey, issuer string) *Validator {
	return &Validator{
		issuer: issuer,
		keys:   map[string]any{kid: pub},
	}
}

// ValidateToken parses and verifies a token, returning its claims. It enforces the
// RS256 signing method, a known key id, standard time claims, and (when configured)
// the expected issuer.
func (v *Validator) ValidateToken(tokenString string) (gojwt.MapClaims, error) {
	token, err := gojwt.Parse(tokenString, func(token *gojwt.Token) (any, error) {
		if _, ok := token.Method.(*gojwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		kid, ok := token.Header["kid"].(string)
		if !ok {
			return nil, fmt.Errorf("missing kid in token header")
		}
		return v.getKey(kid)
	})
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}
	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	claims, ok := token.Claims.(gojwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("invalid claims type")
	}

	if v.issuer != "" {
		iss, ok := claims["iss"].(string)
		if !ok || iss != v.issuer {
			return nil, fmt.Errorf("invalid issuer")
		}
	}

	return claims, nil
}

// IsConfigured reports whether the validator can obtain keys (in-process or JWKS).
func (v *Validator) IsConfigured() bool {
	v.keysMu.RLock()
	defer v.keysMu.RUnlock()
	return v.jwksURL != "" || len(v.keys) > 0
}

// getKey returns the cached key for kid, refreshing from JWKS once on a miss.
func (v *Validator) getKey(kid string) (any, error) {
	v.keysMu.RLock()
	key, exists := v.keys[kid]
	v.keysMu.RUnlock()
	if exists {
		return key, nil
	}

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

// refreshKeys fetches and caches the RSA keys from the JWKS endpoint.
func (v *Validator) refreshKeys() error {
	if v.jwksURL == "" {
		return fmt.Errorf("JWKS URL not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), jwksFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return fmt.Errorf("create JWKS request: %w", err)
	}

	resp, err := v.client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch JWKS: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("JWKS endpoint returned status %d", resp.StatusCode)
	}

	var set JWKS
	if err := json.NewDecoder(resp.Body).Decode(&set); err != nil {
		return fmt.Errorf("decode JWKS: %w", err)
	}

	v.keysMu.Lock()
	defer v.keysMu.Unlock()
	for _, key := range set.Keys {
		if key.Kty != "RSA" {
			continue
		}
		pubKey, err := parseRSAPublicKey(key.N, key.E)
		if err != nil {
			continue // skip invalid keys
		}
		v.keys[key.Kid] = pubKey
	}
	return nil
}
