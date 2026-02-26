package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/chainsafe/canton-middleware/pkg/config"
)

// ServeAndWait starts an HTTP server with the given handler and config in a goroutine
// and blocks until either:
//   - ctx is canceled, or
//   - the server fails unexpectedly.
//
// It then performs a graceful shutdown with the configured timeout.
//
// Returns a non-nil error if:
//   - the server exits unexpectedly (not ErrServerClosed), or
//   - shutdown fails.
func ServeAndWait(ctx context.Context, handler http.Handler, logger *zap.Logger, cfg *config.ServerConfig) error {
	if handler == nil {
		return fmt.Errorf("nil handler")
	}
	if cfg == nil {
		return fmt.Errorf("nil server config")
	}

	shutdownTimeout := cfg.ShutdownTimeout
	if shutdownTimeout <= 0 {
		shutdownTimeout = 30 * time.Second
	}

	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Handler:      handler,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		if logger != nil {
			logger.Info("HTTP server listening", zap.String("address", srv.Addr))
		}
		err := srv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	var runErr error
	select {
	case <-ctx.Done():
		if logger != nil {
			logger.Info("Shutdown signal received")
		}
	case runErr = <-errCh:
		if runErr != nil && logger != nil {
			logger.Error("HTTP server error", zap.Error(runErr))
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if logger != nil {
		logger.Info("Shutting down HTTP server", zap.Duration("timeout", shutdownTimeout))
	}

	if err := srv.Shutdown(shutdownCtx); err != nil {
		if logger != nil {
			logger.Error("HTTP server shutdown error", zap.Error(err))
		}
		return fmt.Errorf("http shutdown: %w", err)
	}

	// If server crashed unexpectedly, return that after shutdown attempt
	if runErr != nil {
		return fmt.Errorf("http server failed: %w", runErr)
	}

	if logger != nil {
		logger.Info("HTTP server stopped")
	}
	return nil
}
