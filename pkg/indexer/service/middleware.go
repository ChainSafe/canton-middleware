package service

import (
	"net/http"
	"strings"

	apperrors "github.com/chainsafe/canton-middleware/pkg/app/errors"
	apphttp "github.com/chainsafe/canton-middleware/pkg/app/http"
	"github.com/chainsafe/canton-middleware/pkg/auth"
)

// JWTMiddleware extracts the Bearer token, validates it, reads the
// canton_party_id claim, and injects it via auth.WithCantonParty.
// Returns 401 JSON on any failure.
func JWTMiddleware(validator *auth.JWTValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				apphttp.DefaultErrorHandler(w, apperrors.UnAuthorizedError(nil, "missing Authorization header"))
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				apphttp.DefaultErrorHandler(w, apperrors.UnAuthorizedError(nil, "malformed Authorization header"))
				return
			}

			claims, err := validator.ValidateToken(parts[1])
			if err != nil {
				apphttp.DefaultErrorHandler(w, apperrors.UnAuthorizedError(err, "invalid token"))
				return
			}

			partyID, _ := claims["canton_party_id"].(string)
			if partyID == "" {
				apphttp.DefaultErrorHandler(w, apperrors.UnAuthorizedError(nil, "missing canton_party_id claim"))
				return
			}

			ctx := auth.WithCantonParty(r.Context(), partyID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
