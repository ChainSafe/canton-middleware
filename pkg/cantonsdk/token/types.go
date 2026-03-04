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

// Holding represents a CIP56Holding contract (Splice-compliant).
type Holding struct {
	ContractID      string
	Issuer          string
	Owner           string
	Amount          string
	Symbol          string // derived from Metadata["splice.chainsafe.io/symbol"]
	InstrumentAdmin string
	InstrumentID    string
	Locked          bool
	Metadata        map[string]string
}

// MintRequest represents an issuer mint request via TokenConfig.
type MintRequest struct {
	RecipientParty string
	Amount         string
	TokenSymbol    string // needed for GetTokenConfigCID lookup
	ConfigCID      string
	EventMeta      map[string]string // bridge context; nil for native mints
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
	return nil
}

// BurnRequest represents an issuer burn request via TokenConfig.
type BurnRequest struct {
	HoldingCID  string
	Amount      string
	TokenSymbol string            // needed for GetTokenConfigCID lookup
	EventMeta   map[string]string // bridge context; nil for native burns
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
	return nil
}

// TokenTransferEvent is a decoded representation of a CIP56.Events.TokenTransferEvent.
// Unified event for all token mutations (mint, burn, transfer).
// See canton-erc20 repository: daml/cip56-token/src/CIP56/Events.daml
type TokenTransferEvent struct {
	ContractID      string
	Issuer          string
	FromParty       string // empty = mint (no sender)
	ToParty         string // empty = burn (no receiver)
	Amount          string
	InstrumentAdmin string
	InstrumentID    string
	Timestamp       time.Time
	Meta            map[string]string // bridge context; nil for native ops
	AuditObservers  []string
}

// EventType derives the event type from fromParty/toParty.
func (e *TokenTransferEvent) EventType() string {
	if e.FromParty == "" {
		return "MINT"
	}
	if e.ToParty == "" {
		return "BURN"
	}
	return "TRANSFER"
}

// EvmTxHash returns the bridge deposit tx hash from metadata.
func (e *TokenTransferEvent) EvmTxHash() string { return e.Meta["bridge.externalTxId"] }

// EvmDestination returns the bridge withdrawal address from metadata.
func (e *TokenTransferEvent) EvmDestination() string { return e.Meta["bridge.externalAddress"] }

// UserFingerprint returns the bridge audit fingerprint from metadata.
func (e *TokenTransferEvent) UserFingerprint() string { return e.Meta["bridge.fingerprint"] }

// TransferFactoryInfo contains the CIP56TransferFactory contract details
// required by external wallets for Splice-compliant explicit contract disclosure.
type TransferFactoryInfo struct {
	ContractID       string
	CreatedEventBlob []byte
	TemplateID       TemplateIdentifier
}

// TemplateIdentifier is a portable representation of a Daml template/interface reference.
type TemplateIdentifier struct {
	PackageID  string `json:"package_id"`
	ModuleName string `json:"module_name"`
	EntityName string `json:"entity_name"`
}
