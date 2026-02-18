package ledger

import (
	"net/http"

	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// Option configures ledger client settings using
// the functional options pattern.
type Option func(*settings)

// settings holds internal configurable dependencies
// used during ledger client initialization.
type settings struct {
	logger     *zap.Logger
	httpClient *http.Client
	dialOpts   []grpc.DialOption

	authProvider AuthProvider // optional override, primarily for tests
}

// WithLogger sets a custom logger for the ledger client.
func WithLogger(l *zap.Logger) Option {
	return func(s *settings) { s.logger = l }
}

// WithHTTPClient sets a custom HTTP client for authentication requests.
func WithHTTPClient(c *http.Client) Option {
	return func(s *settings) { s.httpClient = c }
}

// WithGRPCDialOptions appends additional gRPC dial options.
func WithGRPCDialOptions(opts ...grpc.DialOption) Option {
	return func(s *settings) { s.dialOpts = append(s.dialOpts, opts...) }
}

// WithAuthProvider overrides the default authentication provider.
func WithAuthProvider(p AuthProvider) Option {
	return func(s *settings) { s.authProvider = p }
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
