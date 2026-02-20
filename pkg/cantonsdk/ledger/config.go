package ledger

import (
	"errors"
	"time"
)

// Config contains the configuration required to establish
// a connection to a Canton participant.
type Config struct {
	RPCURL         string
	LedgerID       string
	MaxMessageSize int

	TLS  *TLSConfig
	Auth *AuthConfig
}

// TLSConfig defines transport security settings for the gRPC connection.
type TLSConfig struct {
	Enabled            bool
	CertFile           string
	KeyFile            string
	CAFile             string
	InsecureSkipVerify bool
}

// AuthConfig defines OAuth2 client credentials settings
// used for authenticating against the Canton participant.
type AuthConfig struct {
	ClientID     string
	ClientSecret string //nolint:gosec // standard OAuth2 config field name
	Audience     string
	TokenURL     string

	// ExpiryLeeway specifies how long before actual token expiry
	// the token should be considered expired. If zero, a default is applied.
	ExpiryLeeway time.Duration
}

func (cfg *AuthConfig) validate() error {
	if cfg == nil {
		return errors.New("nil config")
	}
	if cfg.ClientID == "" || cfg.ClientSecret == "" || cfg.Audience == "" || cfg.TokenURL == "" {
		return errors.New("no auth configured: OAuth2 client credentials are required")
	}
	return nil
}
