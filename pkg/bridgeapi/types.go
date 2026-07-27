// SPDX-License-Identifier: Apache-2.0

package bridgeapi

// TokenInfo describes one bridgeable token for the dapp's token picker.
type TokenInfo struct {
	Symbol           string `json:"symbol"`
	Mechanism        string `json:"mechanism"`
	EVMAddress       string `json:"evm_address"`
	Decimals         int    `json:"decimals"`
	BridgeFee        string `json:"bridge_fee,omitempty"`
	EstimatedSeconds int    `json:"estimated_seconds,omitempty"`
}

// QuoteRequest asks for the unsigned transactions that bridge `Amount` of
// `Token` from the authenticated user's EVM address to their Canton party.
type QuoteRequest struct {
	Token string `json:"token"`
	// Amount is a decimal string in token units (e.g. "12.5").
	Amount string `json:"amount"`
}

// Step kinds, for display only — wallets execute steps in order regardless.
const (
	StepKindApprove = "approve"
	StepKindDeposit = "deposit"
)

// TxStep is one fully ABI-encoded unsigned transaction the wallet signs and
// sends as-is. The dapp never learns what the calldata encodes.
type TxStep struct {
	// Kind labels the step for display: "approve" or "deposit".
	Kind  string `json:"kind"`
	To    string `json:"to"`
	Data  string `json:"data"`
	Value string `json:"value"`
}

// Fees describes the mechanism's bridging cost.
type Fees struct {
	BridgeFee string `json:"bridge_fee"`
	Currency  string `json:"currency"`
}

// Quote is a prepared deposit: the exact transactions to sign plus indicative
// cost and latency. Quoting is stateless — the server persists nothing, so a
// quote can be re-requested freely.
type Quote struct {
	ChainID          int64    `json:"chain_id"`
	Steps            []TxStep `json:"steps"`
	Fees             Fees     `json:"fees"`
	EstimatedSeconds int      `json:"estimated_seconds"`
}

// RegisterDepositRequest reports a submitted deposit transaction for status
// tracking. Nothing here needs binding to a quote: the chain is the source of
// truth, and the recipient party is re-derived from the authenticated
// session — a registration with wrong parameters only produces a status row
// that never completes.
type RegisterDepositRequest struct {
	Token string `json:"token"`
	// Amount is a decimal string in token units.
	Amount string `json:"amount"`
	TxHash string `json:"tx_hash"`
}
