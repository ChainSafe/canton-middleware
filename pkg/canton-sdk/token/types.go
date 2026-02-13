package token

import "fmt"

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

func (m MintRequest) validate() error {
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

func (b BurnRequest) validate() error {
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
