// SPDX-License-Identifier: Apache-2.0

package http

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

// RequestMetricsMiddleware returns a chi-compatible middleware that records
// HTTP request metrics: total count (by method/route/status), duration, and
// active connection gauge.
//
// Route patterns (e.g. /v1/tokens/{id}) are used as the endpoint label rather
// than raw paths to avoid unbounded cardinality.
func RequestMetricsMiddleware(m *HTTPMetrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			m.ActiveConnections.Inc()

			ww := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)

			defer func() {
				m.ActiveConnections.Dec()
				endpoint := ChiRoutePattern(r)
				statusCode := fmt.Sprintf("%d", ww.Status())
				elapsed := time.Since(start).Seconds()

				m.IncRequestsTotal(r.Method, endpoint, statusCode)
				m.ObserveRequestDuration(r.Method, endpoint).Observe(elapsed)
			}()

			next.ServeHTTP(ww, r)
		})
	}
}

// ChiRoutePattern extracts the matched chi route pattern from the request
// context, e.g. "/v1/tokens/{id}" rather than the raw path.
// Falls back to "unknown" for unmatched routes (404s).
func ChiRoutePattern(r *http.Request) string {
	rctx := chi.RouteContext(r.Context())
	if rctx != nil && rctx.RoutePattern() != "" {
		return rctx.RoutePattern()
	}
	return "unknown"
}

// CORSMiddleware returns a CORS middleware restricted to the given origins.
// If origins contains "*", all origins are permitted.
// Otherwise, the request Origin is reflected back only if it matches the allowlist.
func CORSMiddleware(origins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(origins))
	wildcard := false
	for _, o := range origins {
		if o == "*" {
			wildcard = true
		}
		allowed[o] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if wildcard {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else if origin != "" && allowed[origin] {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Add("Vary", "Origin")
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, X-Signature, X-Message, Authorization")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
