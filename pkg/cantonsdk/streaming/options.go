package streaming

import "go.uber.org/zap"

// Option configures a streaming Client.
type Option func(*settings)

type settings struct {
	logger *zap.Logger
}

// WithLogger sets a custom logger on the streaming Client.
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
