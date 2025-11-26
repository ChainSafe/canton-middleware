package canton

import (
	"time"
)

// Config is imported from config package to avoid duplication
// See: pkg/config/config.go for CantonConfig definition

// DepositRequest represents a Canton deposit request event
type DepositRequest struct {
	EventID      string
	TransactionID string
	BridgeID     string
	Depositor    string
	TokenSymbol  string
	Amount       string
	EthChainID   int64
	EthRecipient string
	Mode         AssetMode
	ClientNonce  string
	CreatedAt    time.Time
}

// WithdrawalRequest represents a withdrawal to be confirmed on Canton
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
