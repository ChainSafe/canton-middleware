package relayer

import "time"

// Chain identifiers used as canonical keys throughout the relayer.
const (
	ChainCanton   = "canton"
	ChainEthereum = "ethereum"
)

// OffsetBegin is the special Canton ledger offset meaning "start from the beginning".
const OffsetBegin = "BEGIN"

// decimalPlaces is the number of decimal places used for ERC-20 token amounts.
const decimalPlaces = 18

// TransferStatus represents the current state of a cross-chain transfer.
type TransferStatus string

const (
	TransferStatusPending   TransferStatus = "pending"
	TransferStatusCompleted TransferStatus = "completed"
	TransferStatusFailed    TransferStatus = "failed"
)

// TransferDirection indicates the direction of the transfer.
type TransferDirection string

const (
	DirectionCantonToEthereum TransferDirection = "canton_to_ethereum"
	DirectionEthereumToCanton TransferDirection = "ethereum_to_canton"
)

// Transfer represents a cross-chain token transfer.
type Transfer struct {
	ID                string
	Direction         TransferDirection
	Status            TransferStatus
	SourceChain       string
	DestinationChain  string
	SourceTxHash      string
	DestinationTxHash *string
	TokenAddress      string
	Amount            string
	Sender            string
	Recipient         string
	Nonce             int64
	SourceBlockNumber int64
	RetryCount        int
	CreatedAt         time.Time
	UpdatedAt         time.Time
	CompletedAt       *time.Time
	ErrorMessage      *string
}

// ChainState tracks the last processed offset for a chain.
type ChainState struct {
	ChainID   string
	LastBlock int64  // numeric block/offset; 0 for Canton
	Offset    string // string representation of the offset (block hash or ledger offset)
	UpdatedAt time.Time
}

// Event represents a generic bridge event flowing from source to destination.
type Event struct {
	ID               string
	TransactionID    string
	SourceChain      string
	DestinationChain string
	SourceTxHash     string
	// SourceContractID carries the source-chain contract ID when applicable (e.g.
	// the Canton WithdrawalEvent ContractID used to call CompleteWithdrawal).
	SourceContractID  string
	TokenAddress      string
	Amount            string
	Sender            string
	Recipient         string
	Nonce             int64
	SourceBlockNumber int64
}
