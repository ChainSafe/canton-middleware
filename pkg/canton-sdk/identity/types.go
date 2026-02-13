package identity

import "errors"

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

func (c CreateFingerprintMappingRequest) validate() error {
	if c.UserParty == "" {
		return errors.New("user_party ID is required")
	}
	if c.Fingerprint == "" {
		return errors.New("fingerprint ID is required")
	}
	return nil
}
