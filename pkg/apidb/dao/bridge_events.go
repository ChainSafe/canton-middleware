package dao

import (
	"time"

	"github.com/uptrace/bun"
)

// BridgeEventDao is a data access object that maps directly to the 'bridge_events' table in PostgreSQL.
type BridgeEventDao struct {
	bun.BaseModel        `bun:"table:bridge_events"`
	ID                   int64      `json:"id" bun:",pk,autoincrement"`
	EventType            string     `json:"event_type" bun:",notnull,type:varchar(20)"`
	ContractID           string     `json:"contract_id" bun:",unique,notnull,type:varchar(255)"`
	Fingerprint          *string    `json:"fingerprint,omitempty" bun:",type:varchar(128)"`
	RecipientFingerprint *string    `json:"recipient_fingerprint,omitempty" bun:",type:varchar(128)"`
	Amount               string     `json:"amount" bun:",notnull,type:numeric(38,18)"`
	EvmTxHash            *string    `json:"evm_tx_hash,omitempty" bun:",type:varchar(66)"`
	EvmDestination       *string    `json:"evm_destination,omitempty" bun:",type:varchar(42)"`
	TokenSymbol          *string    `json:"token_symbol,omitempty" bun:",type:varchar(20)"`
	CantonTimestamp      *time.Time `json:"canton_timestamp,omitempty" bun:"canton_timestamp"`
	ProcessedAt          time.Time  `json:"processed_at" bun:",nullzero,default:current_timestamp"`
}
