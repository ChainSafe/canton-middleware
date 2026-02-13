package identity

// Party contains the result of allocating a new Canton party.
type Party struct {
	PartyID string
	IsLocal bool
}

// FingerprintMapping represents a FingerprintMapping contract.
type FingerprintMapping struct {
	ContractID  string
	Issuer      string
	UserParty   string
	Fingerprint string
	EvmAddress  string
}

// CreateFingerprintMappingRequest contains inputs for creating a FingerprintMapping.
type CreateFingerprintMappingRequest struct {
	UserParty   string
	Fingerprint string
	EvmAddress  string
}

// todo: add validation
