package indexer

import "time"

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
	InstrumentID    string // instrumentId.id  — token identifier (e.g. "DEMO", "PROMPT")
	InstrumentAdmin string // instrumentId.admin — token admin/issuer party

	// Issuer of the TokenTransferEvent contract (the token config issuer).
	Issuer string

	// Transfer semantics, mirroring ERC-20 Transfer(from, to, value).
	EventType   EventType
	Amount      string  // decimal string, e.g. "1.500000000000000000"
	FromPartyID *string // nil for mints
	ToPartyID   *string // nil for burns

	// Bridge audit context extracted from meta.values (nil for native peer-to-peer transfers).
	ExternalTxID    *string // meta["bridge.externalTxId"]    — EVM transaction hash
	ExternalAddress *string // meta["bridge.externalAddress"] — EVM destination address
	Fingerprint     *string // meta["bridge.fingerprint"]     — user fingerprint

	// Provenance.
	ContractID    string    // TokenTransferEvent contract ID — idempotency key (event_id in store)
	TxID          string    // Ledger transaction UpdateId
	LedgerOffset  int64     // Ledger offset of the containing transaction
	Timestamp     time.Time // Contract-level time from TokenTransferEvent.timestamp
	EffectiveTime time.Time // Ledger transaction effective time
}

// InstrumentKey is the Canton equivalent of an ERC-20 contract address.
// It uniquely identifies a CIP56 token deployment.
// Corresponds to the DAML InstrumentId{admin: Party, id: Text} record.
//
// instrumentId.id alone is NOT unique — two different issuers can both deploy
// a token with id="DEMO". The full {Admin, ID} pair IS unique and is the correct
// key for whitelisting specific token deployments.
type InstrumentKey struct {
	Admin string // instrumentId.admin — the token admin/issuer party
	ID    string // instrumentId.id   — the token identifier (e.g. "DEMO")
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
	InstrumentAdmin string // instrumentId.admin — token admin/issuer party (ERC-20: deployer)
	InstrumentID    string // instrumentId.id   — token symbol/identifier (ERC-20: symbol, e.g. "DEMO")

	// Roles.
	Issuer string // issuer party on the TokenTransferEvent contract (ERC-20: minter role)

	// Supply (ERC-20: totalSupply()).
	// Running total, always ≥ 0. Incremented by each MINT amount, decremented by each BURN amount.
	// Updated atomically with every mint/burn via Store.ApplySupplyDelta.
	TotalSupply string // decimal string, e.g. "1000000.000000000000000000"

	// Holders (ERC-20: no standard equivalent, but a standard block-explorer metric).
	// Count of distinct parties currently holding a non-zero balance.
	// The store increments this when a balance first becomes positive, decrements when it returns to zero.
	HolderCount int64

	// Provenance.
	FirstSeenOffset int64     // ledger offset when this token was first indexed
	FirstSeenAt     time.Time // ledger effective time when this token was first indexed
}

// Balance is a party's current token holding for a specific instrument.
// (ERC-20: the per-address entry in the balances mapping, i.e. balanceOf(address).)
//
// Amount is a non-negative decimal string representing the live balance,
// e.g. "1500.000000000000000000". It is computed by the processor from the
// prior balance plus the event amount and stored as a snapshot — no delta
// arithmetic is performed in the database.
type Balance struct {
	PartyID         string // canton party (ERC-20: address)
	InstrumentAdmin string // instrumentId.admin
	InstrumentID    string // instrumentId.id
	Amount          string // current balance, decimal string ≥ 0
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
