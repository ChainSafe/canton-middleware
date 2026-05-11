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

// PendingOfferDao maps to the 'indexer_pending_offers' table.
// Rows are written on TransferOffer CREATED events and updated (status→ACCEPTED)
// on ARCHIVED events. Rows are never deleted — the table is a full audit log.
type PendingOfferDao struct {
	bun.BaseModel   `bun:"table:indexer_pending_offers"`
	ContractID      string    `bun:",pk,type:varchar(255)"`
	Status          string    `bun:",notnull,type:varchar(20),default:'PENDING'"`
	ReceiverPartyID string    `bun:",notnull,type:varchar(255)"`
	SenderPartyID   string    `bun:",notnull,type:varchar(255)"`
	InstrumentAdmin string    `bun:",notnull,type:varchar(255)"`
	InstrumentID    string    `bun:",notnull,type:varchar(255)"`
	Amount          string    `bun:",notnull,type:text"`
	LedgerOffset    int64     `bun:",notnull"`
	CreatedAt       time.Time `bun:",notnull"`
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

func toPendingOfferDao(o *indexer.PendingOffer) *PendingOfferDao {
	return &PendingOfferDao{
		ContractID:      o.ContractID,
		Status:          string(o.Status),
		ReceiverPartyID: o.ReceiverPartyID,
		SenderPartyID:   o.SenderPartyID,
		InstrumentAdmin: o.InstrumentAdmin,
		InstrumentID:    o.InstrumentID,
		Amount:          o.Amount,
		LedgerOffset:    o.LedgerOffset,
		CreatedAt:       o.CreatedAt,
	}
}

func fromPendingOfferDao(d *PendingOfferDao) indexer.PendingOffer {
	return indexer.PendingOffer{
		ContractID:      d.ContractID,
		Status:          indexer.OfferStatus(d.Status),
		ReceiverPartyID: d.ReceiverPartyID,
		SenderPartyID:   d.SenderPartyID,
		InstrumentAdmin: d.InstrumentAdmin,
		InstrumentID:    d.InstrumentID,
		Amount:          d.Amount,
		LedgerOffset:    d.LedgerOffset,
		CreatedAt:       d.CreatedAt,
	}
}
