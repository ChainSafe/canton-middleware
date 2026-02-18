package token

import "errors"

// Config contains the configuration required to initialize the token client.
type Config struct {
	DomainID     string
	RelayerParty string
	UserID       string

	// CIP56PackageID is the package ID containing CIP-56 templates.
	CIP56PackageID string
}

func (c *Config) validate() error {
	if c == nil {
		return errors.New("nil config")
	}
	if c.DomainID == "" {
		return errors.New("domain_id is required")
	}
	if c.RelayerParty == "" {
		return errors.New("relayer_party is required")
	}
	if c.UserID == "" {
		return errors.New("user_id is required")
	}
	if c.CIP56PackageID == "" {
		return errors.New("cip56_package_id is required")
	}
	return nil
}
