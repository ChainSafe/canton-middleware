package client

import (
	"net/http"

	sharedmetrics "github.com/chainsafe/canton-middleware/internal/metrics"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/bridge"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/token"

	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// Option configures client settings using the functional options pattern.
type Option func(*settings)

type settings struct {
	logger         *zap.Logger
	httpClient     *http.Client
	bridgeCfg      *bridge.Config
	keyResolver    token.KeyResolver
	dialOpts       []grpc.DialOption
	promRegisterer sharedmetrics.NamespacedRegisterer
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

// WithKeyResolver provides a function to look up signing keys by party ID.
// Required for transfers involving external parties (Interactive Submission).
func WithKeyResolver(kr token.KeyResolver) Option {
	return func(s *settings) { s.keyResolver = kr }
}

// WithGRPCDialOptions appends additional gRPC dial options to the underlying
// ledger connection. Use this to inject interceptors, custom credentials, etc.
func WithGRPCDialOptions(opts ...grpc.DialOption) Option {
	return func(s *settings) { s.dialOpts = append(s.dialOpts, opts...) }
}

// WithPrometheusRegisterer enables Canton client metrics and registers them
// against the provided NamespacedRegisterer. When set, a gRPC unary interceptor
// is automatically installed that records <namespace>_client_rpc_duration_seconds.
func WithPrometheusRegisterer(reg sharedmetrics.NamespacedRegisterer) Option {
	return func(s *settings) { s.promRegisterer = reg }
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
