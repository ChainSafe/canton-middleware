// SPDX-License-Identifier: Apache-2.0

// Package transfer implements the non-custodial prepare/execute transfer API.
package transfer

// PrepareRequest is the HTTP request body for preparing a non-custodial transfer.
// Exactly one of To (a registered user's EVM address) or ToPartyID (an arbitrary
// Canton party id, e.g. a party on an external participant node) must be set.
type PrepareRequest struct {
	To        string `json:"to,omitempty"`          // Recipient EVM address (0x...) of a registered user
	ToPartyID string `json:"to_party_id,omitempty"` // Recipient Canton party id (<hint>::<fingerprint>)
	Amount    string `json:"amount"`                // Token amount (decimal string)
	Token     string `json:"token"`                 // "DEMO" or "PROMPT"
}

// CustodialTransferRequest is the HTTP request body for the custodial transfer
// endpoint, which sends a token to an arbitrary recipient party id in a single
// server-signed call (the middleware holds the custodial user's Canton key).
type CustodialTransferRequest struct {
	ToPartyID string `json:"to_party_id"` // Recipient Canton party id (<hint>::<fingerprint>)
	Amount    string `json:"amount"`      // Token amount (decimal string)
	Token     string `json:"token"`       // Token symbol
}

// PrepareResponse is the HTTP response body for a prepared transfer.
type PrepareResponse struct {
	TransferID      string `json:"transfer_id"`
	TransactionHash string `json:"transaction_hash"` // hex-encoded hash to sign
	PartyID         string `json:"party_id"`
	ExpiresAt       string `json:"expires_at"` // RFC3339
}

// ExecuteRequest is the HTTP request body for executing a prepared transfer.
type ExecuteRequest struct {
	TransferID string `json:"transfer_id"`
	Signature  string `json:"signature"` // hex-encoded DER signature
	SignedBy   string `json:"signed_by"` // Canton multihash fingerprint
}

// ExecuteResponse is the HTTP response body for a completed transfer.
type ExecuteResponse struct {
	Status string `json:"status"` // "completed"
}

// IncomingTransfer represents a single pending inbound transfer offer. Fields
// downstream of the on-ledger TransferOffer are always populated; token-metadata
// fields (Symbol, Decimals, ContractAddress, Name) are populated when the
// instrument is in the api-server's supported_tokens config and omitted otherwise.
type IncomingTransfer struct {
	ContractID      string `json:"contract_id"`
	SenderPartyID   string `json:"sender_party_id"`
	ReceiverPartyID string `json:"receiver_party_id"`
	Amount          string `json:"amount"`
	InstrumentAdmin string `json:"instrument_admin"`
	InstrumentID    string `json:"instrument_id"`
	Symbol          string `json:"symbol,omitempty"`
	Decimals        int    `json:"decimals,omitempty"`
	Name            string `json:"name,omitempty"`
	ContractAddress string `json:"contract_address,omitempty"`
}

// IncomingTransfersList is the HTTP response body for GET /api/v2/transfer/incoming.
// Pagination is page/limit-based to match the indexer's underlying envelope
// (`pkg/indexer.Page[T]`): callers ask for a specific page rather than carrying
// an opaque cursor, since the indexer is keyed by ledger_offset and a stable
// numeric offset is the natural cursor. HasMore is derived from page*limit < total
// so clients can stop iterating without needing arithmetic.
type IncomingTransfersList struct {
	Items   []IncomingTransfer `json:"items"`
	Total   int64              `json:"total"`
	Page    int                `json:"page"`
	Limit   int                `json:"limit"`
	HasMore bool               `json:"has_more"`
}

// OutgoingTransfer is a single TransferOffer the queried party sent. Mirrors
// IncomingTransfer but adds Status (pending/expired/accepted) and ExpiresAt so
// callers can track an outbound offer through its lifecycle.
type OutgoingTransfer struct {
	ContractID      string `json:"contract_id"`
	SenderPartyID   string `json:"sender_party_id"`
	ReceiverPartyID string `json:"receiver_party_id"`
	Amount          string `json:"amount"`
	InstrumentAdmin string `json:"instrument_admin"`
	InstrumentID    string `json:"instrument_id"`
	Status          string `json:"status"`
	ExpiresAt       string `json:"expires_at,omitempty"` // RFC3339; omitted when the offer never expires
	Symbol          string `json:"symbol,omitempty"`
	Decimals        int    `json:"decimals,omitempty"`
	Name            string `json:"name,omitempty"`
	ContractAddress string `json:"contract_address,omitempty"`
}

// OutgoingTransfersList is the HTTP response body for GET /api/v2/transfer/outgoing.
type OutgoingTransfersList struct {
	Items   []OutgoingTransfer `json:"items"`
	Total   int64              `json:"total"`
	Page    int                `json:"page"`
	Limit   int                `json:"limit"`
	HasMore bool               `json:"has_more"`
}

// CompletedTransfer is a single settled transfer in the unified history view,
// generalized across all tokens (our CIP-56 tokens and external ones like USDCx).
type CompletedTransfer struct {
	ContractID      string `json:"contract_id"`
	Source          string `json:"source"` // "event" | "offer"
	FromPartyID     string `json:"from_party_id"`
	ToPartyID       string `json:"to_party_id"`
	Amount          string `json:"amount"`
	InstrumentAdmin string `json:"instrument_admin"`
	InstrumentID    string `json:"instrument_id"`
	Timestamp       string `json:"timestamp"`       // RFC3339
	TxID            string `json:"tx_id,omitempty"` // ledger update id (events only)
	Symbol          string `json:"symbol,omitempty"`
	Decimals        int    `json:"decimals,omitempty"`
	Name            string `json:"name,omitempty"`
	ContractAddress string `json:"contract_address,omitempty"`
}

// CompletedTransfersList is the HTTP response body for GET /api/v2/transfer/completed.
type CompletedTransfersList struct {
	Items   []CompletedTransfer `json:"items"`
	Total   int64               `json:"total"`
	Page    int                 `json:"page"`
	Limit   int                 `json:"limit"`
	HasMore bool                `json:"has_more"`
}

// PrepareAcceptRequest is the HTTP request body for preparing a non-custodial accept.
type PrepareAcceptRequest struct {
	InstrumentAdmin string `json:"instrument_admin"` // Canton party ID of the instrument admin
}
