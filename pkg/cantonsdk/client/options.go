package client

import (
	"net/http"

	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/bridge"

	"go.uber.org/zap"
)

// Option configures client settings using the functional options pattern.
type Option func(*settings)

type settings struct {
	logger     *zap.Logger
	httpClient *http.Client
	bridgeCfg  *bridge.Config
}

// WithLogger sets a custom logger for the SDK client.
func WithLogger(l *zap.Logger) Option {
	return func(s *settings) { s.logger = l }
}

// WithHTTPClient sets a custom HTTP client for the SDK client.
func WithHTTPClient(c *http.Client) Option {
	return func(s *settings) { s.httpClient = c }
}

// WithBridgeConfig enables and configures the optional bridge client.
// If nil, the bridge client is not initialized.
func WithBridgeConfig(cfg *bridge.Config) Option {
	return func(s *settings) { s.bridgeCfg = cfg }
}

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
