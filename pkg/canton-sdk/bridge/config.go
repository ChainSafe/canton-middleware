package bridge

import "fmt"

// Config contains bridge-client configuration.
type Config struct {
	// DomainID is the Canton synchronizer/domain ID to submit commands against.
	DomainID string

	// UserID is the Canton user id (JWT subject) used for command submission.
	UserID string

	// RelayerParty is the party that acts as issuer/relayer in bridge flows.
	RelayerParty string

	// BridgePackageID is the package id that contains Wayfinder.Bridge templates/choices.
	BridgePackageID string

	// BridgeModule is the DAML module name that contains WayfinderBridgeConfig template.
	BridgeModule string

	// CorePackageID is the package id that contains bridge-core templates such as WithdrawalEvent.
	// If empty, BridgePackageID may be used as a fallback by some deployments.
	CorePackageID string

	// CIP56PackageID is the package id containing CIP56.Events templates (MintEvent/BurnEvent).
	CIP56PackageID string
}

// Validate validates config for bridge operations.
func (c Config) validate() error {
	if c.DomainID == "" {
		return fmt.Errorf("domain id is required")
	}
	if c.UserID == "" {
		return fmt.Errorf("user id is required")
	}
	if c.RelayerParty == "" {
		return fmt.Errorf("relayer party is required")
	}
	if c.BridgePackageID == "" {
		return fmt.Errorf("bridge package id is required")
	}
	if c.BridgeModule == "" {
		return fmt.Errorf("bridge module is required")
	}
	if c.CIP56PackageID == "" {
		return fmt.Errorf("cip56_package id is required")
	}
	return nil
}

// effectiveCorePackageID returns the core package id, falling back to BridgePackageID when unset.
func (c Config) effectiveCorePackageID() string {
	if c.CorePackageID != "" {
		return c.CorePackageID
	}
	return c.BridgePackageID
}
