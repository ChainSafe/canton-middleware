package token

import "go.uber.org/zap"

type settings struct {
	logger        *zap.Logger
	keyResolver   KeyResolver
	preparedCache *PreparedTransferCache
}

// Option configures the token client.
type Option func(*settings)

// WithLogger sets a custom logger for the token client.
func WithLogger(l *zap.Logger) Option {
	return func(s *settings) { s.logger = l }
}

// WithKeyResolver provides a function to look up signing keys by party ID.
// Required for transfers involving external parties (Interactive Submission).
func WithKeyResolver(kr KeyResolver) Option {
	return func(s *settings) { s.keyResolver = kr }
}

// WithPreparedTransferCache sets the cache used by PrepareTransfer/ExecuteTransfer.
// Required for non-custodial transfer support.
func WithPreparedTransferCache(c *PreparedTransferCache) Option {
	return func(s *settings) { s.preparedCache = c }
}

func applyOptions(opts []Option) settings {
	s := settings{logger: zap.NewNop()}
	for _, opt := range opts {
		if opt != nil {
			opt(&s)
		}
	}
	return s
}
