package ledger

import (
	"errors"
	"time"
)

// Config contains the configuration required to establish
// a connection to a Canton participant.
type Config struct {
	RPCURL         string `yaml:"rpc_url" validate:"required"`
	LedgerID       string `yaml:"ledger_id" default:""`
	MaxMessageSize int    `yaml:"max_inbound_message_size" default:"52428800"`

	TLS  *TLSConfig  `yaml:"tls" validate:"required"`
	Auth *AuthConfig `yaml:"auth" validate:"required"`
}

// TLSConfig defines transport security settings for the gRPC connection.
type TLSConfig struct {
	Enabled            bool   `yaml:"enabled" default:"false"`
	CertFile           string `yaml:"cert_file" default:""`
	KeyFile            string `yaml:"key_file" default:""`
	CAFile             string `yaml:"ca_file" default:""`
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify" default:"false"`
}

// AuthConfig defines OAuth2 client credentials settings
// used for authenticating against the Canton participant.
type AuthConfig struct {
	ClientID     string `yaml:"client_id" validate:"required"`
	ClientSecret string `yaml:"client_secret" validate:"required"`
	Audience     string `yaml:"audience" validate:"required"`
	TokenURL     string `yaml:"token_url" validate:"required"`

	// ExpiryLeeway specifies how long before actual token expiry
	// the token should be considered expired. If zero, a default is applied.
	ExpiryLeeway time.Duration `yaml:"expiry_leeway" default:"60s"`
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
