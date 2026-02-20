package httpserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// ServeAndWait starts srv in a goroutine and blocks until either:
//   - ctx is canceled, or
//   - the server fails unexpectedly.
//
// It then performs a graceful shutdown with the given timeout.
//
// Returns a non-nil error if:
//   - the server exits unexpectedly (not ErrServerClosed), or
//   - shutdown fails.
func ServeAndWait(ctx context.Context, logger *zap.Logger, srv *http.Server, shutdownTimeout time.Duration) error {
	if srv == nil {
		return fmt.Errorf("nil http server")
	}
	if shutdownTimeout <= 0 {
		shutdownTimeout = 30 * time.Second
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
