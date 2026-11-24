// SPDX-License-Identifier: Apache-2.0

package http

import (
	"context"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// ReadyPath is where services serve their readiness check. Liveness stays on the health check URL.
const ReadyPath = "/ready"

const readinessTimeout = 3 * time.Second

// ReadinessCheck is one dependency a service needs before it can take traffic.
type ReadinessCheck struct {
	Name  string
	Check func(ctx context.Context) error
}

// ReadyHandler serves 200 READY when every check passes, and 503 naming the first failing
// check otherwise. The error itself is only logged, since the endpoint may be public.
func ReadyHandler(logger *zap.Logger, checks ...ReadinessCheck) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
		defer cancel()

		for _, c := range checks {
			if err := c.Check(ctx); err != nil {
				logger.Warn("readiness check failed", zap.String("check", c.Name), zap.Error(err))
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte("NOT_READY: " + c.Name))
				return
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("READY"))
	}
}
