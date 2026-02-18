package identity

import "go.uber.org/zap"

type settings struct {
	logger *zap.Logger
}

// Option configures the identity client.
type Option func(*settings)

// WithLogger sets a custom logger for the identity client.
func WithLogger(l *zap.Logger) Option {
	return func(s *settings) { s.logger = l }
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
