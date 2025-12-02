package canton

import (
	"time"
)

// Config is imported from config package to avoid duplication
// See: pkg/config/config.go for CantonConfig definition

// =============================================================================
// ISSUER-CENTRIC MODEL TYPES
// =============================================================================

// FingerprintMapping represents a registered user's fingerprint → Party mapping
// This is created by the issuer when onboarding a user
type FingerprintMapping struct {
	ContractID  string
	Issuer      string
	UserParty   string
	Fingerprint string // 32-byte fingerprint as hex (64-68 chars)
	EvmAddress  string // Optional EVM address for withdrawals
}

// PendingDeposit represents a deposit waiting to be processed
// Created by middleware when EVM deposit event is detected
type PendingDeposit struct {
	ContractID  string
	Issuer      string
	Fingerprint string
	Amount      string
	EvmTxHash   string
	TokenID     string
	CreatedAt   time.Time
}

// DepositReceipt represents a successfully processed deposit
type DepositReceipt struct {
	ContractID  string
	Issuer      string
	Recipient   string // Resolved Party
	Fingerprint string
	Amount      string
	EvmTxHash   string
	TokenID     string
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

// =============================================================================
// REQUEST TYPES (for submitting commands)
// =============================================================================

// RegisterUserRequest represents a request to register a user's fingerprint
type RegisterUserRequest struct {
	UserParty   string
	Fingerprint string
	EvmAddress  string // Optional
}

// CreatePendingDepositRequest represents a request to create a pending deposit
type CreatePendingDepositRequest struct {
	Fingerprint string
	Amount      string
	EvmTxHash   string
	Timestamp   time.Time
}

// ProcessDepositRequest represents a request to process a deposit
type ProcessDepositRequest struct {
	DepositCid string
	MappingCid string
}

// InitiateWithdrawalRequest represents a request to start a withdrawal
type InitiateWithdrawalRequest struct {
	MappingCid     string
	HoldingCid     string
	Amount         string
	EvmDestination string
}

// CompleteWithdrawalRequest represents a request to mark withdrawal complete
type CompleteWithdrawalRequest struct {
	WithdrawalEventCid string
	EvmTxHash          string
}

// =============================================================================
// LEGACY TYPES (for backwards compatibility)
// =============================================================================

// MintProposalRequest represents a legacy mint proposal request
// DEPRECATED: Use CreatePendingDepositRequest + ProcessDepositRequest
type MintProposalRequest struct {
	Recipient string
	Amount    string
	Reference string // EVM Tx Hash
}

// BurnEvent represents a legacy burn event on Canton
// DEPRECATED: Use WithdrawalEvent
type BurnEvent struct {
	EventID       string
	TransactionID string
	Operator      string
	Owner         string
	Amount        string
	Destination   string // EVM Address
	Reference     string
}

// =============================================================================
// OTHER TYPES
// =============================================================================

// DepositRequest represents a Canton deposit request event
type DepositRequest struct {
	EventID       string
	TransactionID string
	BridgeID      string
	Depositor     string
	TokenSymbol   string
	Amount        string
	EthChainID    int64
	EthRecipient  string
	Mode          AssetMode
	ClientNonce   string
	CreatedAt     time.Time
}

// WithdrawalRequest represents a withdrawal to be confirmed on Canton
// Note: Different from InitiateWithdrawalRequest which is the new flow
type WithdrawalRequest struct {
	EthTxHash   string
	EthSender   string
	Recipient   string
	TokenSymbol string
	Amount      string
	Nonce       int64
	EthChainID  int64
}

// AssetMode represents the bridge mode
type AssetMode string

const (
	AssetModeLockUnlock AssetMode = "LockUnlock"
	AssetModeMintBurn   AssetMode = "MintBurn"
)

// TokenRef represents a Canton token reference
type TokenRef struct {
	Issuer string
	Symbol string
}

// ProcessedEvent tracks processed Ethereum events
type ProcessedEvent struct {
	ChainID int64
	TxHash  string
}
