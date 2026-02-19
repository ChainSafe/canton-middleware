package token

import "go.uber.org/zap"

type settings struct {
	logger      *zap.Logger
	keyResolver KeyResolver
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

func applyOptions(opts []Option) settings {
	s := settings{logger: zap.NewNop()}
	for _, opt := range opts {
		if opt != nil {
			opt(&s)
		}
	}
	return s
}
