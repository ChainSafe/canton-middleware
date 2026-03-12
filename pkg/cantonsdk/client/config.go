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
	DomainID    string `yaml:"domain_id"`
	IssuerParty string `yaml:"issuer_party"` // maps to IssuerParty (identity/token) and OperatorParty (bridge)

	// Sub-component configs. Each only carries its unique fields in YAML.
	Ledger   *ledger.Config   `yaml:"ledger"`
	Identity *identity.Config `yaml:"identity"`
	Token    *token.Config    `yaml:"token"`
	Bridge   *bridge.Config   `yaml:"bridge"` // nil = bridge disabled
}
