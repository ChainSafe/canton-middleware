// Package client provides the high-level Canton SDK client.
//
// It exposes a unified interface for interacting with a Canton ledger,
// including identity, token, and optional bridge operations.
package client

import (
	"context"
	"fmt"

	"github.com/chainsafe/canton-middleware/pkg/canton-sdk/bridge"
	"github.com/chainsafe/canton-middleware/pkg/canton-sdk/identity"
	"github.com/chainsafe/canton-middleware/pkg/canton-sdk/ledger"
	"github.com/chainsafe/canton-middleware/pkg/canton-sdk/token"
	appcfg "github.com/chainsafe/canton-middleware/pkg/config"
)

// Client is the SDK facade.
type Client struct {
	Ledger   ledger.Ledger
	Identity identity.Identity
	Token    token.Token
	Bridge   bridge.Bridge // optional; nil when disabled
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

	sub := ""
	if cfg.Identity.UserID == "" {
		sub, err = l.JWTSubject(ctx)
		if err != nil {
			return nil, err
		}
		cfg.Identity.UserID = sub
	}

	id, err := identity.New(cfg.Identity, l, identity.WithLogger(s.logger))
	if err != nil {
		_ = l.Close()
		return nil, err
	}

	tk, err := token.New(cfg.Token, l, id, token.WithLogger(s.logger))
	if err != nil {
		_ = l.Close()
		return nil, err
	}

	var br bridge.Bridge
	bridgeCfg := cfg.Bridge
	if s.bridgeCfg != nil {
		bridgeCfg = s.bridgeCfg
	}
	if bridgeCfg != nil {
		bridgeCfg.UserID = sub
		br, err = bridge.New(*bridgeCfg, l, id, bridge.WithLogger(s.logger))
		if err != nil {
			_ = l.Close()
			return nil, err
		}
	}

	return &Client{
		Ledger:   l,
		Identity: id,
		Token:    tk,
		Bridge:   br,
	}, nil
}

// Close closes the underlying ledger connection.
func (c *Client) Close() error {
	if c == nil || c.Ledger == nil {
		return nil
	}
	return c.Ledger.Close()
}

// NewFromAppConfig is a convenience adapter for existing config.CantonConfig.
// This keeps SDK clean but makes migration easy.
func NewFromAppConfig(ctx context.Context, cfg *appcfg.CantonConfig, opts ...Option) (*Client, error) {
	_ = ctx // reserved for future (e.g. eager connectivity check)

	if cfg == nil {
		return nil, fmt.Errorf("nil canton config")
	}
	s := applyOptions(opts)

	sdkCfg := Config{
		Ledger: ledger.Config{
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
		},
		Identity: identity.Config{
			DomainID:        cfg.DomainID,
			RelayerParty:    cfg.RelayerParty,
			CommonPackageID: cfg.CommonPackageID,
		},
		Token: token.Config{
			DomainID:       cfg.DomainID,
			RelayerParty:   cfg.RelayerParty,
			CIP56PackageID: cfg.CIP56PackageID,
		},
	}

	// Bridge is optional. Enable only when bridge config exists.
	if cfg.BridgePackageID != "" && cfg.BridgeModule != "" && cfg.CorePackageID != "" {
		sdkCfg.Bridge = &bridge.Config{
			DomainID:        cfg.DomainID,
			RelayerParty:    cfg.RelayerParty,
			BridgePackageID: cfg.BridgePackageID,
			CorePackageID:   cfg.CorePackageID,
			BridgeModule:    cfg.BridgeModule,
			CIP56PackageID:  cfg.CIP56PackageID,
			// TODO: why bridge config needs the common package id
		}
	}

	return New(ctx, sdkCfg,
		WithLogger(s.logger),
		WithHTTPClient(s.httpClient),
		WithBridgeConfig(sdkCfg.Bridge),
	)
}
