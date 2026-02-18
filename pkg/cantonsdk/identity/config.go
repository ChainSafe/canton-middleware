package identity

import "errors"

// Config contains the configuration required to initialize the identity client.
type Config struct {
	DomainID     string
	RelayerParty string
	UserID       string

	// CommonPackageID is the preferred package ID for FingerprintMapping.
	CommonPackageID string

	// BridgePackageID is a fallback package ID when CommonPackageID is not configured.
	BridgePackageID string
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
	if c.GetPackageID() == "" {
		return errors.New("one of common_package_id or bridge_package_id is required")
	}
	return nil
}

// GetPackageID return the CommonPackageID or BridgePackageID based on the preference.
func (c *Config) GetPackageID() string {
	if c.CommonPackageID != "" {
		return c.CommonPackageID
	}
	// Fallback BridgePackageID  if common package id is missing
	return c.BridgePackageID
}
