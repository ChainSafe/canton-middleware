package bridge

import "fmt"

// Config contains bridge-client configuration.
type Config struct {
	// DomainID is the Canton synchronizer/domain ID to submit commands against.
	DomainID string `yaml:"domain_id"`
	// UserID is the Canton user id (JWT subject) used for command submission.
	UserID string `yaml:"user_id"`
	// OperatorParty is the party that controls WayfinderBridgeConfig (the bridge operator).
	OperatorParty string `yaml:"operator_party"`
	// PackageID is the package id that contains Wayfinder.Bridge templates/choices.
	PackageID string `yaml:"package_id" validate:"required"`
	// CorePackageID is the package id for Bridge.Contracts (bridge-core package).
	// WithdrawalRequest and WithdrawalEvent live here, separate from PackageID.
	// Falls back to PackageID if empty.
	CorePackageID string `yaml:"core_package_id"`
	// Module is the DAML module name that contains WayfinderBridgeConfig template.
	Module string `yaml:"module" validate:"required"`
}

// Validate validates config for bridge operations.
func (c *Config) validate() error {
	if c == nil {
		return fmt.Errorf("nil config")
	}
	if c.DomainID == "" {
		return fmt.Errorf("domain id is required")
	}
	if c.UserID == "" {
		return fmt.Errorf("user id is required")
	}
	if c.OperatorParty == "" {
		return fmt.Errorf("operator party is required")
	}
	if c.PackageID == "" {
		return fmt.Errorf("bridge package id is required")
	}
	if c.Module == "" {
		return fmt.Errorf("bridge module is required")
	}
	return nil
}
