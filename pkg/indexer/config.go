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
}

// FilterModeAndKeys converts the config into the domain FilterMode and instrument
// key slice expected by engine.NewTokenTransferDecoder.
func (c *Config) FilterModeAndKeys() (FilterMode, []InstrumentKey) {
	if c.FilterMode == "whitelist" {
		return FilterModeWhitelist, c.Instruments
	}
	return FilterModeAll, nil
}
