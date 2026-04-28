// Package client provides the high-level Canton SDK client.
//
// It exposes a unified interface for interacting with a Canton ledger,
// including identity, token, and optional bridge operations.
package client

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/bridge"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/identity"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/ledger"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/token"
)

// Client is the SDK facade.
type Client struct {
	Ledger   ledger.Ledger
	Identity identity.Identity
	Token    token.Token
	Bridge   bridge.Bridge // optional; nil when disabled
}

// New creates an SDK client from SDK-native config.
func New(ctx context.Context, cfg *Config, opts ...Option) (*Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("nil canton config")
	}
	_ = ctx // reserved for future (e.g. eager connectivity check)
	s := applyOptions(opts)

	propagateCommonConfig(cfg)

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
		cfg.Token.UserID = sub
	}

	id, err := identity.New(cfg.Identity, l, identity.WithLogger(s.logger))
	if err != nil {
		_ = l.Close()
		return nil, err
	}

	tokenOpts := []token.Option{token.WithLogger(s.logger)}
	if s.keyResolver != nil {
		tokenOpts = append(tokenOpts, token.WithKeyResolver(s.keyResolver))
	}
	if len(cfg.Token.ExternalTokens) > 0 {
		tokenOpts = append(tokenOpts, token.WithRegistryClient(
			token.NewRegistryClient(&http.Client{Timeout: 10 * time.Second}),
		))
	}
	tk, err := token.New(cfg.Token, l, id, tokenOpts...)
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
		bridgeCfg.UserID = cfg.Identity.UserID
		br, err = bridge.New(bridgeCfg, l, id, bridge.WithLogger(s.logger))
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

// propagateCommonConfig copies top-level common fields (DomainID, IssuerParty) into
// each sub-client config. Note that sub-config validation is intentionally two-phase:
// the startup validator (called from LoadAPIServer/LoadRelayerServer) cannot validate
// fields that are populated here (DomainID, IssuerParty, UserID) because they have no
// YAML tags and are set after YAML decode. Those fields are validated later by each
// sub-client's own validate() call inside its New() constructor.
func propagateCommonConfig(cfg *Config) {
	if cfg.Identity != nil {
		cfg.Identity.DomainID = cfg.DomainID
		cfg.Identity.IssuerParty = cfg.IssuerParty
	}
	if cfg.Token != nil {
		cfg.Token.DomainID = cfg.DomainID
		cfg.Token.IssuerParty = cfg.IssuerParty
	}
	if cfg.Bridge != nil {
		cfg.Bridge.DomainID = cfg.DomainID
		cfg.Bridge.OperatorParty = cfg.IssuerParty
	}
}

// Close closes the underlying ledger connection.
func (c *Client) Close() error {
	if c == nil || c.Ledger == nil {
		return nil
	}
	return c.Ledger.Close()
}
