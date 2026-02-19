package token

import "go.uber.org/zap"

// Signer can produce DER-encoded ECDSA signatures for Canton Interactive Submission.
// SignDER hashes the message with SHA-256 before signing (Canton returns multihash data).
// Fingerprint returns the Canton key fingerprint (multihash of SPKI public key).
type Signer interface {
	SignDER(message []byte) ([]byte, error)
	Fingerprint() (string, error)
}

// KeyResolver looks up a signer for the given Canton party ID.
// Used by Interactive Submission to sign transactions on behalf of external parties.
type KeyResolver func(partyID string) (Signer, error)

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
