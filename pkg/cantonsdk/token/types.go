package token

import (
	"fmt"
	"time"
)

// Signer can produce DER-encoded ECDSA signatures for Canton Interactive Submission.
// SignDER hashes the message with SHA-256 before signing (Canton returns multihash data).
// Fingerprint returns the Canton key fingerprint (multihash of SPKI public key).
type Signer interface {
	SignDER(message []byte) ([]byte, error)
	Fingerprint() (string, error)
}

// KeyResolver looks up a signer for the given Canton party ID.
// Used by Interactive Submission to sign transactions on behalf of external parties.
type KeyResolver func(partyID string) (Signer, error)

// Holding represents a CIP56Holding contract.
type Holding struct {
	ContractID string
	Issuer     string
	Owner      string
	Amount     string
	Symbol     string
}

// MintRequest represents an issuer mint request via TokenConfig.
type MintRequest struct {
	RecipientParty  string
	Amount          string
	UserFingerprint string
	TokenSymbol     string
	ConfigCID       string
	EvmTxHash       string
}

func (m *MintRequest) validate() error {
	if m.RecipientParty == "" {
		return fmt.Errorf("recipient_party is required")
	}
	if m.Amount == "" {
		return fmt.Errorf("amount is required")
	}
	if m.TokenSymbol == "" {
		return fmt.Errorf("token_symbol is required")
	}
	if m.UserFingerprint == "" {
		return fmt.Errorf("user_fingerprint is required")
	}
	return nil
}

// BurnRequest represents an issuer burn request via TokenConfig.
type BurnRequest struct {
	HoldingCID      string
	Amount          string
	UserFingerprint string
	TokenSymbol     string
	EvmDestination  string
}

func (b *BurnRequest) validate() error {
	if b.HoldingCID == "" {
		return fmt.Errorf("holding_cid is required")
	}
	if b.Amount == "" {
		return fmt.Errorf("amount is required")
	}
	if b.TokenSymbol == "" {
		return fmt.Errorf("token_symbol is required")
	}
	if b.UserFingerprint == "" {
		return fmt.Errorf("user_fingerprint is required")
	}
	return nil
}

// MintEvent is a decoded representation of a CIP56.Events.MintEvent.
// See canton-erc20 repository:
// daml/cip56-token/src/CIP56/Events.daml
type MintEvent struct {
	ContractID      string
	Issuer          string
	Recipient       string
	Amount          string
	HoldingCid      string
	TokenSymbol     string
	EvmTxHash       string
	UserFingerprint string
	Timestamp       time.Time
	AuditObservers  []string
}

// BurnEvent is a decoded representation of a CIP56.Events.BurnEvent.
// See canton-erc20 repository:
// daml/cip56-token/src/CIP56/Events.daml
type BurnEvent struct {
	ContractID      string
	Issuer          string
	BurnedFrom      string
	Amount          string
	EvmDestination  string
	TokenSymbol     string
	UserFingerprint string
	Timestamp       time.Time
	AuditObservers  []string
}
