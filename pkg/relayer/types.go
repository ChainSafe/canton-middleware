package relayer

import "time"

// Chain identifiers used as canonical keys throughout the relayer.
const (
	ChainCanton   = "canton"
	ChainEthereum = "ethereum"
)

// OffsetBegin is the special Canton ledger offset meaning "start from the beginning".
const OffsetBegin = "BEGIN"

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
	ID                string            `json:"id"`
	Direction         TransferDirection `json:"direction"`
	Status            TransferStatus    `json:"status"`
	SourceChain       string            `json:"source_chain"`
	DestinationChain  string            `json:"destination_chain"`
	SourceTxHash      string            `json:"source_tx_hash"`
	DestinationTxHash *string           `json:"destination_tx_hash"`
	TokenAddress      string            `json:"token_address"`
	Amount            string            `json:"amount"`
	Sender            string            `json:"sender"`
	Recipient         string            `json:"recipient"`
	Nonce             int64             `json:"nonce"`
	SourceBlockNumber uint64            `json:"source_block_number"`
	RetryCount        int               `json:"retry_count"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
	CompletedAt       *time.Time        `json:"completed_at"`
	ErrorMessage      *string           `json:"error_message"`
}

// ChainState tracks the last processed offset for a chain.
type ChainState struct {
	ChainID   string
	LastBlock uint64 // numeric block/offset; 0 for Canton
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
	SourceBlockNumber uint64
}
