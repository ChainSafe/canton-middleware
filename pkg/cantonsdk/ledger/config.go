package ledger

import (
	"errors"
	"time"
)

// Config contains the configuration required to establish
// a connection to a Canton participant.
type Config struct {
	RPCURL         string `yaml:"rpc_url"`
	LedgerID       string `yaml:"ledger_id"`
	MaxMessageSize int    `yaml:"max_inbound_message_size"`

	TLS  *TLSConfig  `yaml:"tls"`
	Auth *AuthConfig `yaml:"auth"`
}

// TLSConfig defines transport security settings for the gRPC connection.
type TLSConfig struct {
	Enabled            bool   `yaml:"enabled"`
	CertFile           string `yaml:"cert_file"`
	KeyFile            string `yaml:"key_file"`
	CAFile             string `yaml:"ca_file"`
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify"`
}

// AuthConfig defines OAuth2 client credentials settings
// used for authenticating against the Canton participant.
type AuthConfig struct {
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	Audience     string `yaml:"audience"`
	TokenURL     string `yaml:"token_url"`

	// ExpiryLeeway specifies how long before actual token expiry
	// the token should be considered expired. If zero, a default is applied.
	ExpiryLeeway time.Duration `yaml:"expiry_leeway"`
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
