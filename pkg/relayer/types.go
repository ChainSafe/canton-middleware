// SPDX-License-Identifier: Apache-2.0

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
	TransferStatusPending    TransferStatus = "pending"
	TransferStatusInProgress TransferStatus = "in_progress"
	TransferStatusCompleted  TransferStatus = "completed"
	TransferStatusFailed     TransferStatus = "failed"
)

// IsTerminal reports whether the status is final and the transfer needs no
// further driver steps.
func (s TransferStatus) IsTerminal() bool {
	return s == TransferStatusCompleted || s == TransferStatusFailed
}

// TransferDirection indicates the direction of the transfer.
type TransferDirection string

const (
	DirectionCantonToEthereum TransferDirection = "canton_to_ethereum"
	DirectionEthereumToCanton TransferDirection = "ethereum_to_canton"
)

// Transfer represents a cross-chain token transfer.
type Transfer struct {
	ID string `json:"id"`
	// BridgeKey identifies the TokenBridge adapter that owns this transfer.
	// Rows created by the legacy single-token pipeline carry "wayfinder".
	BridgeKey string `json:"bridge_key"`
	// TokenSymbol is the configured symbol of the bridged token (e.g. "USDCX").
	TokenSymbol string            `json:"token_symbol"`
	Direction   TransferDirection `json:"direction"`
	Status      TransferStatus    `json:"status"`
	// Stage is the mechanism-defined progress marker within Status.
	Stage             string     `json:"stage"`
	SourceChain       string     `json:"source_chain"`
	DestinationChain  string     `json:"destination_chain"`
	SourceTxHash      string     `json:"source_tx_hash"`
	DestinationTxHash *string    `json:"destination_tx_hash"`
	TokenAddress      string     `json:"token_address"`
	Amount            string     `json:"amount"`
	Sender            string     `json:"sender"`
	Recipient         string     `json:"recipient"`
	Nonce             int64      `json:"nonce"`
	SourceBlockNumber uint64     `json:"source_block_number"`
	RetryCount        int        `json:"retry_count"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	CompletedAt       *time.Time `json:"completed_at"`
	ErrorMessage      *string    `json:"error_message"`
	// Metadata carries mechanism-specific breadcrumbs (attestation IDs,
	// request UUIDs) accumulated across Step calls.
	Metadata map[string]string `json:"metadata,omitempty"`
	// NextStepAt is when the driver should step this transfer next.
	// Nil means due immediately. Unused by the legacy pipeline.
	NextStepAt *time.Time `json:"next_step_at,omitempty"`
}

// RegisterTransferRequest registers an externally-initiated transfer for
// observer-mechanism tracking (e.g. an xreserve deposit the dapp just sent).
type RegisterTransferRequest struct {
	// ID uniquely identifies the transfer; defaults to SourceTxHash.
	ID          string            `json:"id"`
	BridgeKey   string            `json:"bridge_key"`
	TokenSymbol string            `json:"token_symbol"`
	Direction   TransferDirection `json:"direction"`
	// SourceTxHash is the initiating transaction (EVM tx hash for deposits,
	// burn identifier for withdrawals).
	SourceTxHash string `json:"source_tx_hash"`
	TokenAddress string `json:"token_address"`
	// Amount is a decimal string in token units (not base units).
	Amount    string            `json:"amount"`
	Sender    string            `json:"sender"`
	Recipient string            `json:"recipient"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// RegisterTransferResponse reports the registered transfer and whether this
// call created it (false when the registration was an idempotent replay).
type RegisterTransferResponse struct {
	Transfer *Transfer `json:"transfer"`
	Created  bool      `json:"created"`
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
	ID string
	// BridgeKey, TokenSymbol, and Direction identify the adapter-owned
	// pipeline for events produced by TokenBridge sources; unset for events
	// from the legacy pipeline (which derives them from its processor wiring).
	BridgeKey        string
	TokenSymbol      string
	Direction        TransferDirection
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

	// Checkpoint marks a progress watermark rather than a real transfer: the source
	// has scanned through SourceBlockNumber and emitted every event up to it. The
	// processor persists the offset and skips transfer processing. Emitted in-order
	// after a slice's events so persisting it cannot skip an unprocessed event.
	Checkpoint bool
}
