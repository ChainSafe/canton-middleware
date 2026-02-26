package user

import "time"

// User represents the domain model for a registered user.
type User struct {
	EVMAddress                string
	CantonParty               string
	Fingerprint               string
	MappingCID                string
	CantonPartyID             string
	CantonKeyCreatedAt        *time.Time
	CantonPrivateKeyEncrypted string
	PromptBalance             string
	DemoBalance               string
	BalanceUpdatedAt          *time.Time
}

// New creates a User from the given parameters.
func New(evmAddress, cantonPartyID, fingerprint, mappingCID, encryptedPKey string) *User {
	now := time.Now()
	return &User{
		EVMAddress:                evmAddress,
		CantonParty:               cantonPartyID,
		Fingerprint:               fingerprint,
		MappingCID:                mappingCID,
		CantonPartyID:             cantonPartyID,
		CantonKeyCreatedAt:        &now,
		CantonPrivateKeyEncrypted: encryptedPKey,
	}
}

// RegisterRequest represents a registration request
// Supports two registration modes:
// 1. Web3 user: signature + message (EIP-191 signature from MetaMask)
// 2. Canton native user: canton_party_id + canton_signature + message (from Loop wallet signMessage)
type RegisterRequest struct {
	// Web3 user registration (EIP-191 signature)
	Signature string `json:"signature,omitzero"`
	Message   string `json:"message,omitzero"`

	// Canton native user registration (Loop wallet signMessage)
	CantonPartyID   string `json:"canton_party_id,omitzero"`
	CantonSignature string `json:"canton_signature,omitzero"`

	// Optional: hex-encoded 32-byte Canton signing key. When provided for native
	// registration, the handler stores it so the API server can sign Interactive
	// Submission transactions on the user's behalf (e.g. transfers via /eth).
	CantonPrivateKey string `json:"canton_private_key,omitzero"`
}

// RegisterResponse represents a registration response
type RegisterResponse struct {
	Party       string `json:"party"`
	Fingerprint string `json:"fingerprint"`
	MappingCID  string `json:"mapping_cid,omitzero"`
	EVMAddress  string `json:"evm_address,omitzero"` // Returned for Canton native users
	PrivateKey  string `json:"private_key,omitzero"` // Returned for Canton native users (for MetaMask import)
}
