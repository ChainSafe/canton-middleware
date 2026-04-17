package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

// requestMetricsMiddleware returns a chi-compatible middleware that records
// HTTP request metrics: total count (by method/route/status), duration, and
// active connection gauge.
//
// Route patterns (e.g. /users/{userID}) are used as the endpoint label rather
// than raw paths to avoid unbounded cardinality.
func requestMetricsMiddleware(m *Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			m.ActiveConnections.Inc()

			// Wrap the ResponseWriter so we can read the status code afterward.
			ww := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)

			defer func() {
				m.ActiveConnections.Dec()
				endpoint := chiRoutePattern(r)
				statusCode := fmt.Sprintf("%d", ww.Status())
				elapsed := time.Since(start).Seconds()

				m.IncRequestsTotal(r.Method, endpoint, statusCode)
				m.ObserveRequestDuration(r.Method, endpoint).Observe(elapsed)
			}()

			next.ServeHTTP(ww, r)
		})
	}
}

// chiRoutePattern extracts the matched chi route pattern from the request
// context, e.g. "/users/{userID}" rather than "/users/alice::123".
// Falls back to "unknown" for unmatched routes (404s).
func chiRoutePattern(r *http.Request) string {
	rctx := chi.RouteContext(r.Context())
	if rctx != nil && rctx.RoutePattern() != "" {
		return rctx.RoutePattern()
	}
	return "unknown"
}
