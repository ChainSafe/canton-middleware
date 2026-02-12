package client

import (
	"net/http"

	"go.uber.org/zap"
)

// Option configures client settings using the functional options pattern.
type Option func(*settings)

// settings holds internal configurable dependencies for the client.
type settings struct {
	logger     *zap.Logger
	httpClient *http.Client
}

// WithLogger sets a custom logger for the client.
// If not provided, a no-op logger is used.
func WithLogger(l *zap.Logger) Option {
	return func(s *settings) { s.logger = l }
}

// WithHTTPClient sets a custom HTTP client for outbound requests.
// If not provided, http.DefaultClient is used.
func WithHTTPClient(c *http.Client) Option {
	return func(s *settings) { s.httpClient = c }
}

// applyOptions applies the provided options and returns the resulting settings.
// Defaults are applied before user-defined options.
func applyOptions(opts []Option) settings {
	s := settings{
		logger:     zap.NewNop(),
		httpClient: http.DefaultClient,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&s)
		}
	}
	return s
}
