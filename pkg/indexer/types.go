package indexer

import "time"

// OfferStatus is the lifecycle state of a TransferOffer contract.
type OfferStatus string

const (
	// OfferStatusPending is the initial state: the offer exists and awaits receiver acceptance.
	OfferStatusPending OfferStatus = "PENDING"

	// OfferStatusAccepted means the Canton ledger emitted an ARCHIVED event for the
	// TransferOffer contract — either because the receiver exercised TransferInstruction_Accept
	// or because the offer was rejected/expired. The row is kept for audit history.
	OfferStatusAccepted OfferStatus = "ACCEPTED"
)

// PendingOffer represents a TransferOffer contract on the Canton ledger.
// Rows are written on CREATED events and updated to ACCEPTED on ARCHIVED events.
// Rows are never deleted so the full transfer history is preserved.
type PendingOffer struct {
	ContractID      string      `json:"contract_id"`
	Status          OfferStatus `json:"status"`
	ReceiverPartyID string      `json:"receiver_party_id"`
	SenderPartyID   string      `json:"sender_party_id"`
	InstrumentAdmin string      `json:"instrument_admin"`
	InstrumentID    string      `json:"instrument_id"`
	Amount          string      `json:"amount"`
	LedgerOffset    int64       `json:"ledger_offset"`
	CreatedAt       time.Time   `json:"created_at"`

	// IsArchived is a decode-time signal only — not persisted.
	IsArchived bool `json:"-"`
}

// EventType classifies a TokenTransferEvent as MINT, BURN, or TRANSFER.
// Derived from the fromParty/toParty Optional fields — mirrors ERC-20 Transfer semantics:
//
//	MINT:     fromParty = None,        toParty = Some(recipient)
//	BURN:     fromParty = Some(owner), toParty = None
//	TRANSFER: fromParty = Some(sender), toParty = Some(receiver)
type EventType string

const (
	EventMint     EventType = "MINT"
	EventBurn     EventType = "BURN"
	EventTransfer EventType = "TRANSFER"
)

// ParsedEvent is a fully decoded TokenTransferEvent ready for the processor.
//
// Fields map directly to the DAML TokenTransferEvent template in CIP56.Events:
//
//	issuer       → Issuer
//	instrumentId → InstrumentID (id field) + InstrumentAdmin (admin field)
//	fromParty    → FromPartyID (*string, nil for mints)
//	toParty      → ToPartyID  (*string, nil for burns)
//	amount       → Amount (decimal string)
//	timestamp    → Timestamp (contract-level time, from the DAML event)
//	meta.values  → ExternalTxID, ExternalAddress, Fingerprint (bridge context, nil for transfers)
//
// ContractID (the TokenTransferEvent contract ID) is the idempotency key used
// as event_id in the store — guaranteed unique across the ledger.
//
// Primary identity throughout is canton_party_id — no EVM address at this layer.
type ParsedEvent struct {
	// Instrument identification — fully qualified by both fields.
	InstrumentID    string `json:"instrument_id"`    // instrumentId.id  — token identifier (e.g. "DEMO", "PROMPT")
	InstrumentAdmin string `json:"instrument_admin"` // instrumentId.admin — token admin/issuer party

	// Issuer of the TokenTransferEvent contract (the token config issuer).
	Issuer string `json:"issuer"`

	// Transfer semantics, mirroring ERC-20 Transfer(from, to, value).
	EventType   EventType `json:"event_type"`
	Amount      string    `json:"amount"`                  // decimal string, e.g. "1.500000000000000000"
	FromPartyID *string   `json:"from_party_id,omitempty"` // nil for mints
	ToPartyID   *string   `json:"to_party_id,omitempty"`   // nil for burns

	// Bridge audit context extracted from meta.values (nil for native peer-to-peer transfers).
	ExternalTxID    *string `json:"external_tx_id,omitempty"`   // meta["bridge.externalTxId"]    — EVM transaction hash
	ExternalAddress *string `json:"external_address,omitempty"` // meta["bridge.externalAddress"] — EVM destination address
	Fingerprint     *string `json:"fingerprint,omitempty"`      // meta["bridge.fingerprint"]     — user fingerprint

	// Provenance.
	ContractID    string    `json:"contract_id"`    // TokenTransferEvent contract ID — idempotency key (event_id in store)
	TxID          string    `json:"tx_id"`          // Ledger transaction UpdateId
	LedgerOffset  int64     `json:"ledger_offset"`  // Ledger offset of the containing transaction
	Timestamp     time.Time `json:"timestamp"`      // Contract-level time from TokenTransferEvent.timestamp
	EffectiveTime time.Time `json:"effective_time"` // Ledger transaction effective time
}

// HoldingChange is a Utility.Registry.Holding.V0.Holding lifecycle event.
// Each CREATED event becomes a synthetic MINT-style balance increment for the owner;
// each ARCHIVED event becomes the symmetric decrement (looked up from the store using
// ContractID since archive events carry no field payload). Unlike CIP-56 — which emits
// dedicated TokenTransferEvent contracts — Utility.Registry tokens have no separate
// event template, so the indexer derives balance deltas from the Holding contracts
// themselves to keep indexer_balances consistent for USDCx and similar instruments.
type HoldingChange struct {
	ContractID   string
	IsArchived   bool
	LedgerOffset int64

	// Only populated for CREATED events. ARCHIVED events leave these empty and the
	// processor reads the matching row from indexer_holdings by ContractID.
	Owner           string
	InstrumentAdmin string
	InstrumentID    string
	Amount          string
}

// InstrumentKey is the Canton equivalent of an ERC-20 contract address.
// It uniquely identifies a CIP56 token deployment.
// Corresponds to the DAML InstrumentId{admin: Party, id: Text} record.
//
// instrumentId.id alone is NOT unique — two different issuers can both deploy
// a token with id="DEMO". The full {Admin, ID} pair IS unique and is the correct
// key for whitelisting specific token deployments.
type InstrumentKey struct {
	Admin string `yaml:"admin"` // instrumentId.admin — the token admin/issuer party
	ID    string `yaml:"id"`    // instrumentId.id   — the token identifier (e.g. "DEMO")
}

// Token represents a CIP56 token deployment, uniquely identified by {InstrumentAdmin, InstrumentID}.
// A Token record is created the first time the indexer observes a TokenTransferEvent for a given
// instrument pair. It tracks the ERC-20-equivalent on-chain state derivable from transfer events.
//
// ERC-20 parallel:
//
//	symbol()      → InstrumentID
//	owner/minter  → InstrumentAdmin, Issuer
//	totalSupply() → TotalSupply   (maintained: +amount on MINT, -amount on BURN)
//	                HolderCount   (non-standard but shown on all block explorers)
type Token struct {
	// Identity — canonical composite key.
	InstrumentAdmin string `json:"instrument_admin"` // instrumentId.admin — token admin/issuer party (ERC-20: deployer)
	InstrumentID    string `json:"instrument_id"`    // instrumentId.id   — token symbol/identifier (ERC-20: symbol, e.g. "DEMO")

	// Roles.
	Issuer string `json:"issuer"` // issuer party on the TokenTransferEvent contract (ERC-20: minter role)

	// Supply (ERC-20: totalSupply()).
	// Running total, always ≥ 0. Incremented by each MINT amount, decremented by each BURN amount.
	// Updated atomically with every mint/burn via Store.ApplySupplyDelta.
	TotalSupply string `json:"total_supply"` // decimal string, e.g. "1000000.000000000000000000"

	// Holders (ERC-20: no standard equivalent, but a standard block-explorer metric).
	// Count of distinct parties currently holding a non-zero balance.
	// The store increments this when a balance first becomes positive, decrements when it returns to zero.
	HolderCount int64 `json:"holder_count"`

	// Provenance.
	FirstSeenOffset int64     `json:"first_seen_offset"` // ledger offset when this token was first indexed
	FirstSeenAt     time.Time `json:"first_seen_at"`     // ledger effective time when this token was first indexed
}

// Balance is a party's current token holding for a specific instrument.
// (ERC-20: the per-address entry in the balances mapping, i.e. balanceOf(address).)
//
// Amount is a non-negative decimal string representing the live balance,
// e.g. "1500.000000000000000000". Updated by the store via delta arithmetic
// (Store.ApplyBalanceDelta) — the store adds the signed delta to the persisted value.
type Balance struct {
	PartyID         string `json:"party_id"`         // canton party (ERC-20: address)
	InstrumentAdmin string `json:"instrument_admin"` // instrumentId.admin
	InstrumentID    string `json:"instrument_id"`    // instrumentId.id
	Amount          string `json:"amount"`           // current balance, decimal string ≥ 0
}

// FilterMode controls which token instruments the Parser processes.
type FilterMode int

const (
	// FilterModeAll indexes events from every instrument — equivalent to a global
	// ERC-20 Transfer log covering all CIP56 token deployments visible to the indexer.
	FilterModeAll FilterMode = iota

	// FilterModeWhitelist indexes only events whose InstrumentKey{Admin, ID} is in
	// the allowed set. Use this for an operator who manages a fixed set of tokens.
	// Both Admin and ID must match — this is the Canton equivalent of whitelisting
	// by ERC-20 contract address.
	FilterModeWhitelist
)

// Pagination holds a 1-based page number and page size for list queries.
type Pagination struct {
	Page  int
	Limit int
}

// EventFilter narrows event list queries. Zero-value fields are ignored by the store.
type EventFilter struct {
	InstrumentAdmin string
	InstrumentID    string
	PartyID         string
	EventType       EventType // empty = all types
}

// Page is the generic paginated response envelope.
type Page[T any] struct {
	Items []T   `json:"items"`
	Total int64 `json:"total"`
	Page  int   `json:"page"`
	Limit int   `json:"limit"`
}
