package ethereum

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

// Config is imported from config package to avoid duplication
// See: pkg/config/config.go for EthereumConfig definition

// DepositEvent represents a deposit to Canton event from Ethereum
type DepositEvent struct {
	Token           common.Address
	Sender          common.Address
	CantonRecipient [32]byte
	Amount          *big.Int
	Nonce           *big.Int
	IsWrapped       bool
	BlockNumber     uint64
	TxHash          common.Hash
	LogIndex        uint
}

// WithdrawalEvent represents a withdrawal from Canton event on Ethereum
type WithdrawalEvent struct {
	Token           common.Address
	Recipient       common.Address
	Amount          *big.Int
	Nonce           *big.Int
	CantonTxHash    [32]byte
	BlockNumber     uint64
	TxHash          common.Hash
}
