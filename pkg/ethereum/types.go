// SPDX-License-Identifier: Apache-2.0

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
	BlockNumber     uint64
	TxHash          common.Hash
	LogIndex        uint

	// Checkpoint marks a scan-progress signal rather than a real deposit: the poller
	// emits one (with only BlockNumber set) after each fully scanned block range —
	// including ranges with no deposits — so the consumer can persist scan progress.
	// It rides the same in-order handler path as deposits, so it is only observed
	// after every deposit in the range it covers.
	Checkpoint bool
}

// WithdrawalEvent represents a withdrawal from Canton event on Ethereum
type WithdrawalEvent struct {
	Token        common.Address
	Recipient    common.Address
	Amount       *big.Int
	Nonce        *big.Int
	CantonTxHash [32]byte
	BlockNumber  uint64
	TxHash       common.Hash
}
