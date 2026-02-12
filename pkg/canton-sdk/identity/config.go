package identity

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
