package bridge

import (
	"errors"
	"time"
)

// FingerprintMappingRef is a minimal reference required by bridge flows.
type FingerprintMappingRef struct {
	ContractID  string
	UserParty   string
	Fingerprint string
	EvmAddress  string
}

// PendingDeposit describes a created pending deposit.
type PendingDeposit struct {
	ContractID  string
	Fingerprint string
	Amount      string
	EvmTxHash   string
	CreatedAt   time.Time
}

// WithdrawalRequest describes a created withdrawal request.
type WithdrawalRequest struct {
	ContractID      string
	Amount          string
	EvmDestination  string
	UserFingerprint string
	CreatedAt       time.Time
}

// MintEvent is a decoded representation of a CIP56.Events.MintEvent.
// Ref: https://github.com/ChainSafe/canton-erc20/blob/53065ebcffa047e07cd7dc472ba9a9eed9895340/daml/cip56-token/src/CIP56/Events.daml#L21C10-L21C19
type MintEvent struct {
	ContractID     string
	Issuer         string
	Recipient      string
	Amount         string
	HoldingCid     string
	TokenSymbol    string
	EvmTxHash      string
	Fingerprint    string
	Timestamp      time.Time
	AuditObservers []string
}

// BurnEvent is a decoded representation of a CIP56.Events.BurnEvent.
// Ref: https://github.com/ChainSafe/canton-erc20/blob/53065ebcffa047e07cd7dc472ba9a9eed9895340/daml/cip56-token/src/CIP56/Events.daml#L45
type BurnEvent struct {
	ContractID     string
	Issuer         string
	BurnedFrom     string
	Amount         string
	EvmDestination string
	TokenSymbol    string
	Fingerprint    string
	Timestamp      time.Time
	AuditObservers []string
}

// CreatePendingDepositRequest contains inputs to create a PendingDeposit from an EVM deposit event.
type CreatePendingDepositRequest struct {
	Fingerprint string
	Amount      string
	EvmTxHash   string
}

func (c CreatePendingDepositRequest) validate() error {
	if c.Fingerprint == "" {
		return errors.New("fingerprint is required")
	}
	if c.Amount == "" {
		return errors.New("amount is required")
	}
	if c.EvmTxHash == "" {
		return errors.New("evm_tx_hash is required")
	}
	return nil
}

// ProcessDepositRequest contains inputs to process a PendingDeposit and mint tokens.
type ProcessDepositRequest struct {
	DepositCID string
	MappingCID string
}

func (p ProcessDepositRequest) validate() error {
	if p.DepositCID == "" {
		return errors.New("deposit_cid is required")
	}
	if p.MappingCID == "" {
		return errors.New("mapping_cid is required")
	}
	return nil
}

// InitiateWithdrawalRequest contains inputs to initiate a withdrawal.
type InitiateWithdrawalRequest struct {
	MappingCID     string
	HoldingCID     string
	Amount         string
	EvmDestination string
}

func (i InitiateWithdrawalRequest) validate() error {
	if i.MappingCID == "" {
		return errors.New("mapping_cid is required")
	}
	if i.HoldingCID == "" {
		return errors.New("holding_cid is required")
	}
	if i.Amount == "" {
		return errors.New("amount is required")
	}
	if i.EvmDestination == "" {
		return errors.New("evm_destination is required")
	}
	return nil
}

// CompleteWithdrawalRequest contains inputs to mark a withdrawal as completed.
type CompleteWithdrawalRequest struct {
	WithdrawalEventCID string
	EvmTxHash          string
}

func (c CompleteWithdrawalRequest) validate() error {
	if c.WithdrawalEventCID == "" {
		return errors.New("withdrawal_event_cid is required")
	}
	if c.EvmTxHash == "" {
		return errors.New("evm_tx_hash is required")
	}
	return nil
}
