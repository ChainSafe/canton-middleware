package identity

import (
	"errors"
)

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

// ExternalPartyTopology holds the intermediate state from GenerateExternalPartyTopology
// needed to complete external party allocation with a client-provided signature.
type ExternalPartyTopology struct {
	TopologyTransactions [][]byte // Serialized topology transactions
	MultiHash            []byte   // Hash to be signed by the party's key
	Fingerprint          string   // Canton key fingerprint (multihash of SPKI public key)
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
