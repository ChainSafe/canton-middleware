package identity

import "errors"

// Config contains the configuration required to initialize the identity client.
type Config struct {
	DomainID    string `yaml:"domain_id"`
	IssuerParty string `yaml:"issuer_party"`
	UserID      string `yaml:"user_id"`
	PackageID   string `yaml:"package_id" validate:"required"` // package ID for FingerprintMapping (Common.FingerprintAuth)
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
	if c.PackageID == "" {
		return errors.New("package_id is required")
	}
	return nil
}
