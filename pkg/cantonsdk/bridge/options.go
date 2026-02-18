package bridge

import "go.uber.org/zap"

type Option func(*settings)

type settings struct {
	logger *zap.Logger
}

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
