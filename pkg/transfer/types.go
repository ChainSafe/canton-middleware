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
