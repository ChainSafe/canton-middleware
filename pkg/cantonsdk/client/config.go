package client

import (
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/bridge"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/identity"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/ledger"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/token"
)

// Config is the top-level Canton SDK config; YAML-importable directly into app configs.
type Config struct {
	// Common fields propagated to all sub-components by New().
	DomainID    string `yaml:"domain_id" validate:"required"`
	IssuerParty string `yaml:"issuer_party" validate:"required"` // maps to IssuerParty (identity/token) and OperatorParty (bridge)

	// Sub-component configs. Each only carries its unique fields in YAML.
	Ledger   *ledger.Config   `yaml:"ledger" validate:"required"`
	Identity *identity.Config `yaml:"identity" validate:"required"`
	Token    *token.Config    `yaml:"token" validate:"required"`
	Bridge   *bridge.Config   `yaml:"bridge" default:"-"` // nil = bridge disabled
}
