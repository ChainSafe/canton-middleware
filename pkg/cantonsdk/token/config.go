package token

import "errors"

// ExternalTokenConfig holds the registry endpoint for an external token issuer.
// Key in the map is the InstrumentAdmin party ID (e.g., Circle's Bridge-Operator).
type ExternalTokenConfig struct {
	RegistryURL string `yaml:"registry_url" validate:"required"`
}

// Config contains the configuration required to initialize the token client.
type Config struct {
	DomainID    string `yaml:"domain_id"`
	IssuerParty string `yaml:"issuer_party"`
	UserID      string `yaml:"user_id"`

	CIP56PackageID          string `yaml:"cip56_package_id" validate:"required"`
	SpliceTransferPackageID string `yaml:"splice_transfer_package_id" validate:"required"`
	SpliceHoldingPackageID  string `yaml:"splice_holding_package_id" validate:"required"`

	// ExternalTokens maps InstrumentAdmin party IDs to their registry configuration.
	// Tokens whose InstrumentAdmin matches IssuerParty use local ACS-based factory discovery.
	// Tokens whose InstrumentAdmin is in this map use the external registry API.
	ExternalTokens map[string]ExternalTokenConfig `yaml:"external_tokens"`
}

func (c *Config) validate() error {
	if c == nil {
		return errors.New("nil config")
	}
	if c.DomainID == "" {
		return errors.New("domain_id is required")
	}
	if c.IssuerParty == "" {
		return errors.New("issuer_party is required")
	}
	if c.UserID == "" {
		return errors.New("user_id is required")
	}
	if c.CIP56PackageID == "" {
		return errors.New("cip56_package_id is required")
	}
	if c.SpliceTransferPackageID == "" {
		return errors.New("splice_transfer_package_id is required")
	}
	if c.SpliceHoldingPackageID == "" {
		return errors.New("splice_holding_package_id is required")
	}
	return nil
}
