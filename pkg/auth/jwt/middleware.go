// SPDX-License-Identifier: Apache-2.0

package jwt

import (
	"net/http"
	"slices"
	"strings"

	gojwt "github.com/golang-jwt/jwt/v5"

	apperrors "github.com/chainsafe/canton-middleware/pkg/app/errors"
	apphttp "github.com/chainsafe/canton-middleware/pkg/app/http"
	"github.com/chainsafe/canton-middleware/pkg/auth"
)

// TokenValidator verifies a bearer token and returns its claims. Satisfied by
// Validator.
type TokenValidator interface {
	ValidateToken(token string) (gojwt.MapClaims, error)
}

// RequireAuth returns middleware that rejects requests without a valid bearer token
// for the expected audience and populates the request context with the
// authenticated identity (EVM address + Canton party) from the token claims.
func RequireAuth(validator TokenValidator, audience string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			info, err := authenticate(validator, audience, r)
			if err != nil {
				apphttp.DefaultErrorHandler(w, err)
				return
			}
			ctx := auth.WithAuthInfo(r.Context(), info)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func authenticate(validator TokenValidator, audience string, r *http.Request) (*auth.AuthInfo, error) {
	const prefix = "Bearer "
	header := r.Header.Get("Authorization")
	// RFC 7235: the auth-scheme token is case-insensitive, so accept "bearer" too.
	if len(header) < len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return nil, apperrors.UnAuthorizedError(nil, "bearer token required")
	}

	token := strings.TrimSpace(header[len(prefix):])
	claims, err := validator.ValidateToken(token)
	if err != nil {
		return nil, apperrors.UnAuthorizedError(err, "invalid or expired token")
	}

	if !hasAudience(claims, audience) {
		return nil, apperrors.UnAuthorizedError(nil, "token audience mismatch")
	}

	evmAddress, _ := claims[EVMAddressClaim].(string)
	party, _ := claims["sub"].(string)
	if evmAddress == "" || party == "" {
		return nil, apperrors.UnAuthorizedError(nil, "token missing identity claims")
	}

	return &auth.AuthInfo{EVMAddress: evmAddress, CantonParty: party}, nil
}

// hasAudience reports whether the token's aud claim contains want. The claim may be
// a single string or an array of strings per RFC 7519.
func hasAudience(claims gojwt.MapClaims, want string) bool {
	switch aud := claims["aud"].(type) {
	case string:
		return aud == want
	case []any:
		for _, a := range aud {
			if s, ok := a.(string); ok && s == want {
				return true
			}
		}
	case []string:
		return slices.Contains(aud, want)
	}
	return false
}
