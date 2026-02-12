// Package client provides the high-level Canton SDK client.
//
// It exposes a unified interface for interacting with a Canton ledger,
// including identity, token, bridge, and ledger operations.
package client

import (
	"context"
	"fmt"

	appcfg "github.com/chainsafe/canton-middleware/pkg/config"

	"github.com/chainsafe/canton-middleware/pkg/canton-sdk/ledger"
)

// Client provides a high-level SDK facade for interacting with Canton.
//
// It aggregates all underlying service clients (e.g., Ledger, Identity,
// Token, Bridge) and exposes a cohesive API surface to applications.
// TODO: Later issues add Identity/Token/Bridge clients here.
type Client struct {
	Ledger ledger.Ledger
}

// New creates an SDK client from SDK-native config.
func New(ctx context.Context, cfg Config, opts ...Option) (*Client, error) {
	_ = ctx // reserved for future (e.g. eager connectivity check)
	s := applyOptions(opts)

	l, err := ledger.New(cfg.Ledger,
		ledger.WithLogger(s.logger),
		ledger.WithHTTPClient(s.httpClient),
	)
	if err != nil {
		return nil, err
	}

	return &Client{Ledger: l}, nil
}

// NewFromAppConfig is a convenience adapter for existing config.CantonConfig.
// This keeps SDK clean but makes migration easy.
func NewFromAppConfig(ctx context.Context, cfg *appcfg.CantonConfig, opts ...Option) (*Client, error) {
	_ = ctx // reserved for future (e.g. eager connectivity check)
	if cfg == nil {
		return nil, fmt.Errorf("nil canton config")
	}
	s := applyOptions(opts)

	l, err := ledger.New(ledger.Config{
		RPCURL:         cfg.RPCURL,
		LedgerID:       cfg.LedgerID,
		MaxMessageSize: cfg.MaxMessageSize,
		TLS: ledger.TLSConfig{
			Enabled:  cfg.TLS.Enabled,
			CertFile: cfg.TLS.CertFile,
			KeyFile:  cfg.TLS.KeyFile,
			CAFile:   cfg.TLS.CAFile,
		},
		Auth: ledger.AuthConfig{
			ClientID:     cfg.Auth.ClientID,
			ClientSecret: cfg.Auth.ClientSecret,
			Audience:     cfg.Auth.Audience,
			TokenURL:     cfg.Auth.TokenURL,
		},
	}, ledger.WithLogger(s.logger), ledger.WithHTTPClient(s.httpClient))
	if err != nil {
		return nil, err
	}

	return &Client{Ledger: l}, nil
}
