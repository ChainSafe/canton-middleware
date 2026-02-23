package dao

import "time"

// BridgeEventDao is a data access object that maps directly to the 'bridge_events' table in PostgreSQL.
type BridgeEventDao struct {
	tableName            struct{}   `pg:"bridge_events"` // nolint
	ID                   int64      `json:"id" pg:",pk"`
	EventType            string     `json:"event_type" pg:",notnull,type:VARCHAR(50)"`
	ContractID           string     `json:"contract_id" pg:",unique,notnull,type:VARCHAR(255)"`
	Fingerprint          *string    `json:"fingerprint,omitempty" pg:"fingerprint,type:VARCHAR(64)"`
	RecipientFingerprint *string    `json:"recipient_fingerprint,omitempty" pg:"recipient_fingerprint,type:VARCHAR(64)"`
	Amount               string     `json:"amount" pg:",notnull,type:NUMERIC(38,18)"`
	EvmTxHash            *string    `json:"evm_tx_hash,omitempty" pg:"evm_tx_hash,type:VARCHAR(66)"`
	EvmDestination       *string    `json:"evm_destination,omitempty" pg:"evm_destination,type:VARCHAR(42)"`
	TokenSymbol          *string    `json:"token_symbol,omitempty" pg:"token_symbol,type:VARCHAR(10)"`
	CantonTimestamp      *time.Time `json:"canton_timestamp,omitempty" pg:"canton_timestamp"`
	ProcessedAt          time.Time  `json:"processed_at" pg:"default:now()"`
}
