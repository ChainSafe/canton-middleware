// Package http provides HTTP utilities including chi-compatible error handling
package http

import (
	"encoding/json"
	"errors"
	"net/http"

	apperrors "github.com/chainsafe/canton-middleware/pkg/app/errors"
)

// HandlerFunc defines a function that returns an error for clean error handling
type HandlerFunc func(http.ResponseWriter, *http.Request) error

// HandleError wraps an error-returning HandlerFunc into a standard http.HandlerFunc
// This allows using clean error-returning handlers with any router (chi, http.ServeMux, etc.)
//
// Usage with chi:
//
//	r.Post("/register", http.HandleError(handler.register))
func HandleError(h HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			DefaultErrorHandler(w, err)
		}
	}
}

// DefaultErrorHandler handles errors returned from HTTP handlers
func DefaultErrorHandler(w http.ResponseWriter, err error) {
	var svcErr *apperrors.ServiceError

	type errorResponse struct {
		ErrMsg     string `json:"error"`
		ErrMsgCode int    `json:"code"`
	}

	// Check if it's a ServiceError
	if errors.As(err, &svcErr) {
		// Write error response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(svcErr.StatusCode())
		_ = json.NewEncoder(w).Encode(&errorResponse{
			ErrMsg:     svcErr.Message,
			ErrMsgCode: svcErr.StatusCode(),
		})
		return
	}

	// Handle unknown errors
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	_ = json.NewEncoder(w).Encode(&errorResponse{
		ErrMsg:     "Unexpected Service Error",
		ErrMsgCode: http.StatusInternalServerError,
	})
}
