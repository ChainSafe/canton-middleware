// Package transfer implements the non-custodial prepare/execute transfer API.
package transfer

// PrepareRequest is the HTTP request body for preparing a non-custodial transfer.
type PrepareRequest struct {
	To     string `json:"to"`     // Recipient EVM address (0x...)
	Amount string `json:"amount"` // Token amount (decimal string)
	Token  string `json:"token"`  // "DEMO" or "PROMPT"
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
// Shape mirrors pkg/token.TokensPage so clients can reuse list-handling code.
type IncomingTransfersList struct {
	Items      []IncomingTransfer `json:"items"`
	NextCursor string             `json:"next_cursor,omitempty"`
	HasMore    bool               `json:"has_more"`
}

// PrepareAcceptRequest is the HTTP request body for preparing a non-custodial accept.
type PrepareAcceptRequest struct {
	InstrumentAdmin string `json:"instrument_admin"` // Canton party ID of the instrument admin
}
