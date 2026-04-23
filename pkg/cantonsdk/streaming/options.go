package streaming

import "go.uber.org/zap"

// Option configures a streaming Client.
type Option func(*settings)

type settings struct {
	logger *zap.Logger
	party  *string // nil = not set (wildcard default); non-nil = WithParty called
}

// WithLogger sets a custom logger on the streaming Client.
func WithLogger(l *zap.Logger) Option {
	return func(s *settings) { s.logger = l }
}

// WithParty configures the client to use FiltersByParty, scoping the stream to
// contracts where the given party is a stakeholder. When omitted the client
// defaults to FiltersForAnyParty (requires CanReadAsAnyParty rights).
func WithParty(party string) Option {
	return func(s *settings) { s.party = &party }
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
