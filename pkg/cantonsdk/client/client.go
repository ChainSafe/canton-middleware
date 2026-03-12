// Package client provides the high-level Canton SDK client.
//
// It exposes a unified interface for interacting with a Canton ledger,
// including identity, token, and optional bridge operations.
package client

import (
	"context"
	"fmt"

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

	// Propagate common config to sub-components.
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

	l, err := ledger.New(cfg.Ledger,
		ledger.WithLogger(s.logger),
		ledger.WithHTTPClient(s.httpClient),
	)
	if err != nil {
		return nil, err
	}

	sub := ""
	if cfg.Identity != nil && cfg.Identity.UserID == "" {
		sub, err = l.JWTSubject(ctx)
		if err != nil {
			return nil, err
		}
		cfg.Identity.UserID = sub
	}
	if cfg.Token != nil && cfg.Token.UserID == "" {
		if sub == "" {
			sub, err = l.JWTSubject(ctx)
			if err != nil {
				return nil, err
			}
		}
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
		bridgeUserID := sub
		if bridgeUserID == "" {
			switch {
			case cfg.Identity != nil && cfg.Identity.UserID != "":
				bridgeUserID = cfg.Identity.UserID
			case cfg.Token != nil && cfg.Token.UserID != "":
				bridgeUserID = cfg.Token.UserID
			default:
				bridgeUserID, err = l.JWTSubject(ctx)
				if err != nil {
					_ = l.Close()
					return nil, err
				}
			}
		}
		bridgeCfg.UserID = bridgeUserID
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

// Close closes the underlying ledger connection.
func (c *Client) Close() error {
	if c == nil || c.Ledger == nil {
		return nil
	}
	return c.Ledger.Close()
}
