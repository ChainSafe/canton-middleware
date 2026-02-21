package dao

import "time"

// BridgeEventDao is a data access object that maps directly to the 'bridge_events' table in PostgreSQL.
type BridgeEventDao struct {
	tableName            struct{}   `pg:"bridge_events"` // nolint
	ID                   int64      `json:"id" pg:",pk"`
	EventType            string     `json:"event_type" pg:",notnull"`
	ContractID           string     `json:"contract_id" pg:",unique,notnull"`
	Fingerprint          *string    `json:"fingerprint,omitempty" pg:"fingerprint"`
	RecipientFingerprint *string    `json:"recipient_fingerprint,omitempty" pg:"recipient_fingerprint"`
	Amount               string     `json:"amount" pg:",notnull"`
	EvmTxHash            *string    `json:"evm_tx_hash,omitempty" pg:"evm_tx_hash"`
	EvmDestination       *string    `json:"evm_destination,omitempty" pg:"evm_destination"`
	TokenSymbol          *string    `json:"token_symbol,omitempty" pg:"token_symbol"`
	CantonTimestamp      *time.Time `json:"canton_timestamp,omitempty" pg:"canton_timestamp"`
	ProcessedAt          time.Time  `json:"processed_at" pg:"default:now()"`
}
