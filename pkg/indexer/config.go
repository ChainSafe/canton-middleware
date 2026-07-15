// SPDX-License-Identifier: Apache-2.0

package indexer

// Config holds stream-specific settings for the indexer process.
// It lives in the indexer domain package so that app-level config
// (pkg/config) can embed it without creating a god-config pattern.
type Config struct {
	// CIP56PackageID is the DAML package ID containing CIP56.Events.TokenTransferEvent.
	// Setting this pins the indexer to a specific package version; leave empty to
	// match the template across all package versions.
	CIP56PackageID string `yaml:"cip56_package_id" validate:"required"`

	// FilterMode controls which instruments are indexed.
	// "all"       — index every TokenTransferEvent on the stream.
	// "whitelist" — only index instruments listed in Instruments.
	FilterMode string `yaml:"filter_mode" default:"all" validate:"required,oneof=all whitelist"`

	// Instruments is the whitelist of CIP-56 instruments to index.
	// Only consulted when FilterMode is "whitelist".
	Instruments []InstrumentKey `yaml:"instruments"`

	// UtilityRegistryPackageID is the DAML package ID for the Utility Registry app
	// (Utility.Registry.App.V0.Model.Transfer.TransferOffer).
	// Leave empty to disable TransferOffer tracking.
	UtilityRegistryPackageID string `yaml:"utility_registry_package_id"`

	// UtilityRegistryHoldingPackageID is the DAML package ID for the Utility Registry
	// Holding template (Utility.Registry.Holding.V0.Holding). Required to track
	// USDCx-style balances — without it the indexer never sees Holding contract
	// create/archive events and balances for AllocationFactory-based instruments
	// stay at 0. Leave empty to disable Holding tracking.
	UtilityRegistryHoldingPackageID string `yaml:"utility_registry_holding_package_id"`
}

// InstrumentKey is the Canton equivalent of an ERC-20 contract address.
// It uniquely identifies a CIP56 token deployment.
// Corresponds to the DAML InstrumentId{admin: Party, id: Text} record.
//
// instrumentId.id alone is NOT unique — two different issuers can both deploy
// a token with id="DEMO". The full {Admin, ID} pair IS unique and is the correct
// key for whitelisting specific token deployments.
//
// It is part of the deployed YAML config schema (Config.Instruments), so it
// lives in config.go where the CI config gate watches for schema changes.
type InstrumentKey struct {
	Admin string `yaml:"admin"` // instrumentId.admin — the token admin/issuer party
	ID    string `yaml:"id"`    // instrumentId.id   — the token identifier (e.g. "DEMO")
}

// FilterModeAndKeys converts the config into the domain FilterMode and instrument
// key slice expected by engine.NewTokenTransferDecoder.
func (c *Config) FilterModeAndKeys() (FilterMode, []InstrumentKey) {
	if c.FilterMode == "whitelist" {
		return FilterModeWhitelist, c.Instruments
	}
	return FilterModeAll, nil
}
