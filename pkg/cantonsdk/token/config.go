package token

import "errors"

// Config contains the configuration required to initialize the token client.
type Config struct {
	DomainID    string
	IssuerParty string // the CIP56 token issuer
	UserID      string

	CIP56PackageID          string
	SpliceTransferPackageID string
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
	return nil
}
