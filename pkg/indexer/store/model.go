// SPDX-License-Identifier: Apache-2.0

package store

import (
	"time"

	"github.com/uptrace/bun"

	"github.com/chainsafe/canton-middleware/pkg/indexer"
)

// EventDao maps to the 'indexer_events' table.
// ContractID is the idempotency key — one row per TokenTransferEvent contract.
type EventDao struct {
	bun.BaseModel   `bun:"table:indexer_events"`
	ContractID      string    `bun:",pk,type:varchar(255)"`
	InstrumentID    string    `bun:",notnull,type:varchar(255)"`
	InstrumentAdmin string    `bun:",notnull,type:varchar(255)"`
	Issuer          string    `bun:",notnull,type:varchar(255)"`
	EventType       string    `bun:",notnull,type:varchar(20)"`
	Amount          string    `bun:",notnull,type:text"`
	FromPartyID     *string   `bun:",type:varchar(255)"`
	ToPartyID       *string   `bun:",type:varchar(255)"`
	ExternalTxID    *string   `bun:",type:varchar(255)"`
	ExternalAddress *string   `bun:",type:varchar(255)"`
	Fingerprint     *string   `bun:",type:varchar(255)"`
	TxID            string    `bun:",notnull,type:varchar(255)"`
	LedgerOffset    int64     `bun:",notnull"`
	Timestamp       time.Time `bun:",notnull"`
	EffectiveTime   time.Time `bun:",notnull"`
}

// TokenDao maps to the 'indexer_tokens' table.
// The composite key (InstrumentAdmin, InstrumentID) uniquely identifies a token.
type TokenDao struct {
	bun.BaseModel   `bun:"table:indexer_tokens"`
	InstrumentAdmin string    `bun:",pk,type:varchar(255)"`
	InstrumentID    string    `bun:",pk,type:varchar(255)"`
	Issuer          string    `bun:",notnull,type:varchar(255)"`
	TotalSupply     string    `bun:",notnull,type:text,default:'0'"`
	HolderCount     int64     `bun:",notnull,default:0"`
	FirstSeenOffset int64     `bun:",notnull"`
	FirstSeenAt     time.Time `bun:",notnull"`
}

// BalanceDao maps to the 'indexer_balances' table.
// The composite key (PartyID, InstrumentAdmin, InstrumentID) is unique per holding.
type BalanceDao struct {
	bun.BaseModel   `bun:"table:indexer_balances"`
	PartyID         string `bun:",pk,type:varchar(255)"`
	InstrumentAdmin string `bun:",pk,type:varchar(255)"`
	InstrumentID    string `bun:",pk,type:varchar(255)"`
	Amount          string `bun:",notnull,type:text"`
}

// OffsetDao maps to the 'indexer_offsets' table.
// A single row (ID=1) holds the latest persisted ledger offset.
type OffsetDao struct {
	bun.BaseModel `bun:"table:indexer_offsets"`
	ID            int   `bun:",pk,default:1"`
	LedgerOffset  int64 `bun:",notnull,default:0"`
}

// HoldingDao maps to the 'indexer_holdings' table.
// One row per active Utility.Registry.Holding.V0.Holding contract. Inserted on
// CREATED events and deleted on ARCHIVED events; the stored amount is needed at
// archive time to decrement balances since archive events carry only contract_id.
type HoldingDao struct {
	bun.BaseModel   `bun:"table:indexer_holdings"`
	ContractID      string `bun:",pk,type:varchar(255)"`
	Owner           string `bun:",notnull,type:varchar(255)"`
	InstrumentAdmin string `bun:",notnull,type:varchar(255)"`
	InstrumentID    string `bun:",notnull,type:varchar(255)"`
	Amount          string `bun:",notnull,type:text"`
	LedgerOffset    int64  `bun:",notnull"`
}

// TransferDao maps to the 'indexer_transfers' table — the generalized transfer
// log holding both direct (atomic CIP-56) and offer (2-step) transfers with a
// mutable status lifecycle. Rows are never deleted; the table is a full history.
//   - direct: written on a CIP-56 TRANSFER event, always Status "completed".
//   - offer:  written "pending" on a TransferOffer CREATE, set "completed" on its
//     ARCHIVE. ExpiresAt carries the offer's executeBefore; "expired" is derived
//     at read time (pending + ExpiresAt in the past) and never stored.
type TransferDao struct {
	bun.BaseModel   `bun:"table:indexer_transfers"`
	ContractID      string     `bun:",pk,type:varchar(255)"`
	Kind            string     `bun:",notnull,type:varchar(20)"`
	Status          string     `bun:",notnull,type:varchar(20)"`
	FromPartyID     string     `bun:",notnull,type:varchar(255)"`
	ToPartyID       string     `bun:",notnull,type:varchar(255)"`
	InstrumentAdmin string     `bun:",notnull,type:varchar(255)"`
	InstrumentID    string     `bun:",notnull,type:varchar(255)"`
	Amount          string     `bun:",notnull,type:text"`
	ExpiresAt       *time.Time `bun:",nullzero"` // offer executeBefore; NULL for direct / never-expires
	TxID            string     `bun:",type:varchar(255)"`
	LedgerOffset    int64      `bun:",notnull"`
	CreatedAt       time.Time  `bun:",notnull"`
}

func toEventDao(e *indexer.ParsedEvent) *EventDao {
	return &EventDao{
		ContractID:      e.ContractID,
		InstrumentID:    e.InstrumentID,
		InstrumentAdmin: e.InstrumentAdmin,
		Issuer:          e.Issuer,
		EventType:       string(e.EventType),
		Amount:          e.Amount,
		FromPartyID:     e.FromPartyID,
		ToPartyID:       e.ToPartyID,
		ExternalTxID:    e.ExternalTxID,
		ExternalAddress: e.ExternalAddress,
		Fingerprint:     e.Fingerprint,
		TxID:            e.TxID,
		LedgerOffset:    e.LedgerOffset,
		Timestamp:       e.Timestamp,
		EffectiveTime:   e.EffectiveTime,
	}
}

func fromEventDao(d *EventDao) *indexer.ParsedEvent {
	return &indexer.ParsedEvent{
		ContractID:      d.ContractID,
		InstrumentID:    d.InstrumentID,
		InstrumentAdmin: d.InstrumentAdmin,
		Issuer:          d.Issuer,
		EventType:       indexer.EventType(d.EventType),
		Amount:          d.Amount,
		FromPartyID:     d.FromPartyID,
		ToPartyID:       d.ToPartyID,
		ExternalTxID:    d.ExternalTxID,
		ExternalAddress: d.ExternalAddress,
		Fingerprint:     d.Fingerprint,
		TxID:            d.TxID,
		LedgerOffset:    d.LedgerOffset,
		Timestamp:       d.Timestamp,
		EffectiveTime:   d.EffectiveTime,
	}
}

func fromTokenDao(d *TokenDao) *indexer.Token {
	return &indexer.Token{
		InstrumentAdmin: d.InstrumentAdmin,
		InstrumentID:    d.InstrumentID,
		Issuer:          d.Issuer,
		TotalSupply:     d.TotalSupply,
		HolderCount:     d.HolderCount,
		FirstSeenOffset: d.FirstSeenOffset,
		FirstSeenAt:     d.FirstSeenAt,
	}
}

func fromBalanceDao(d *BalanceDao) *indexer.Balance {
	return &indexer.Balance{
		PartyID:         d.PartyID,
		InstrumentAdmin: d.InstrumentAdmin,
		InstrumentID:    d.InstrumentID,
		Amount:          d.Amount,
	}
}

func toTransferDao(t *indexer.Transfer) *TransferDao {
	return &TransferDao{
		ContractID:      t.ContractID,
		Kind:            t.Kind,
		Status:          t.Status,
		FromPartyID:     t.FromPartyID,
		ToPartyID:       t.ToPartyID,
		InstrumentAdmin: t.InstrumentAdmin,
		InstrumentID:    t.InstrumentID,
		Amount:          t.Amount,
		ExpiresAt:       t.ExpiresAt,
		TxID:            t.TxID,
		LedgerOffset:    t.LedgerOffset,
		CreatedAt:       t.CreatedAt,
	}
}

func fromTransferDao(d *TransferDao) indexer.Transfer {
	return indexer.Transfer{
		ContractID:      d.ContractID,
		Kind:            d.Kind,
		Status:          d.Status,
		FromPartyID:     d.FromPartyID,
		ToPartyID:       d.ToPartyID,
		InstrumentAdmin: d.InstrumentAdmin,
		InstrumentID:    d.InstrumentID,
		Amount:          d.Amount,
		ExpiresAt:       d.ExpiresAt,
		TxID:            d.TxID,
		LedgerOffset:    d.LedgerOffset,
		CreatedAt:       d.CreatedAt,
	}
}
