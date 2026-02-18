package bridge

import (
	"errors"
	"time"
)

// PendingDeposit describes a created pending deposit.
type PendingDeposit struct {
	ContractID  string
	MappingCID  string
	Fingerprint string
	CreatedAt   time.Time
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

type ProcessedDeposit struct {
	ContractID string
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

// WithdrawalRequest describes a created withdrawal request.
type WithdrawalRequest struct {
	ContractID      string
	Amount          string
	EvmDestination  string
	UserFingerprint string
	CreatedAt       time.Time
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

// WithdrawalEvent represents a withdrawal ready for EVM processing
type WithdrawalEvent struct {
	ContractID     string
	EventID        string
	TransactionID  string
	Issuer         string
	UserParty      string
	EvmDestination string
	Amount         string
	Fingerprint    string
	Status         WithdrawalStatus
}

// WithdrawalStatus represents the state of a withdrawal
type WithdrawalStatus string

const (
	WithdrawalStatusPending   WithdrawalStatus = "Pending"
	WithdrawalStatusCompleted WithdrawalStatus = "Completed"
	WithdrawalStatusFailed    WithdrawalStatus = "Failed"
)
