package user

import (
	"errors"
	"time"
)

// Key mode constants for user registration type.
const (
	KeyModeCustodial = "custodial"
	KeyModeExternal  = "external"
)

// User represents the domain model for a registered user.
type User struct {
	EVMAddress                 string
	CantonParty                string
	Fingerprint                string
	MappingCID                 string
	CantonPartyID              string
	CantonKeyCreatedAt         *time.Time
	CantonPrivateKeyEncrypted  string
	KeyMode                    string // "custodial" or "external"
	CantonPublicKeyFingerprint string // For external users: Canton multihash fingerprint
}

// New creates a custodial User from the given parameters.
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
		KeyMode:                   KeyModeCustodial,
	}
}

// NewExternal creates a non-custodial (external) User. No private key is stored.
func NewExternal(evmAddress, cantonPartyID, fingerprint, mappingCID, publicKeyFingerprint string) *User {
	return &User{
		EVMAddress:                 evmAddress,
		CantonParty:                cantonPartyID,
		Fingerprint:                fingerprint,
		MappingCID:                 mappingCID,
		CantonPartyID:              cantonPartyID,
		KeyMode:                    KeyModeExternal,
		CantonPublicKeyFingerprint: publicKeyFingerprint,
	}
}

// RegisterRequest represents a registration request
// Supports three registration modes:
// 1. Web3 user (custodial): signature + message (EIP-191 signature from MetaMask)
// 2. Canton native user: canton_party_id + canton_signature + message (from Loop wallet signMessage)
// 3. External user (non-custodial): key_mode="external" + canton_public_key + registration_token + topology_signature
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

	// External (non-custodial) user registration
	KeyMode           string `json:"key_mode,omitzero"`           // "external" for non-custodial
	CantonPublicKey   string `json:"canton_public_key,omitzero"`  // hex compressed secp256k1 public key
	RegistrationToken string `json:"registration_token,omitzero"` // from prepare-topology response
	TopologySignature string `json:"topology_signature,omitzero"` // DER sig of topology hash (hex)
}

// RegisterResponse represents a registration response
type RegisterResponse struct {
	Party       string `json:"party"`
	Fingerprint string `json:"fingerprint"`
	MappingCID  string `json:"mapping_cid,omitzero"`
	EVMAddress  string `json:"evm_address,omitzero"` // Returned for Canton native users
	PrivateKey  string `json:"private_key,omitzero"` // Returned for Canton native users (for MetaMask import)
	KeyMode     string `json:"key_mode,omitzero"`    // "custodial" or "external"
}

// PrepareTopologyResponse is the response from the prepare-topology step of external user registration.
type PrepareTopologyResponse struct {
	TopologyHash         string `json:"topology_hash"`
	PublicKeyFingerprint string `json:"public_key_fingerprint"`
	RegistrationToken    string `json:"registration_token"`
}

var ErrKeyNotFound = errors.New("key not found")
var ErrUserNotFound = errors.New("user not found")
