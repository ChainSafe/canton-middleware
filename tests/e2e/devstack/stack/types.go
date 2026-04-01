//go:build e2e

package stack

import "github.com/ethereum/go-ethereum/common"

// Account represents a test account (EVM key + address).
type Account struct {
	Address    common.Address
	PrivateKey string // hex, no 0x prefix
}

// ServiceManifest holds all localhost endpoints discovered after Docker compose is up.
type ServiceManifest struct {
	AnvilRPC    string // "http://localhost:8545"
	CantonGRPC  string // "localhost:5011"
	CantonHTTP  string // "http://localhost:5013"
	APIHTTP     string // "http://localhost:8081"
	RelayerHTTP string // "http://localhost:8080"
	IndexerHTTP string // "http://localhost:8082"
	OAuthHTTP   string // "http://localhost:8088"
	PostgresDSN string // "postgres://postgres:p@ssw0rd@localhost:5432/erc20_api"

	// Contract addresses (extracted from deployer logs or env)
	PromptTokenAddr string // "0x5FbDB..."
	BridgeAddr      string // "0xe7f172..."
	DemoTokenAddr   string // virtual: "0xDE3000..."

	// Canton instrument keys for indexer queries (populated after bootstrap)
	PromptInstrumentAdmin string // Canton party ID of the PROMPT token admin
	PromptInstrumentID    string // "PROMPT" — matches InstrumentKey.ID in indexer config
	DemoInstrumentAdmin   string // Canton party ID of the DEMO token admin
	DemoInstrumentID      string // "DEMO"
}

// IndexerToken mirrors indexer.Token — the ERC-20-like state tracked per instrument.
type IndexerToken struct {
	InstrumentAdmin string `json:"instrument_admin"`
	InstrumentID    string `json:"instrument_id"`
	Issuer          string `json:"issuer"`
	TotalSupply     string `json:"total_supply"`
	HolderCount     int64  `json:"holder_count"`
}

// IndexerBalance mirrors indexer.Balance — per-party holding for one instrument.
type IndexerBalance struct {
	PartyID         string `json:"party_id"`
	InstrumentAdmin string `json:"instrument_admin"`
	InstrumentID    string `json:"instrument_id"`
	Amount          string `json:"amount"`
}

// IndexerEvent mirrors indexer.ParsedEvent — a decoded TokenTransferEvent from the ledger.
type IndexerEvent struct {
	ContractID      string  `json:"contract_id"`
	TxID            string  `json:"tx_id"`
	InstrumentAdmin string  `json:"instrument_admin"`
	InstrumentID    string  `json:"instrument_id"`
	EventType       string  `json:"event_type"` // "MINT" | "BURN" | "TRANSFER"
	Amount          string  `json:"amount"`
	FromPartyID     *string `json:"from_party_id,omitempty"`
	ToPartyID       *string `json:"to_party_id,omitempty"`
	ExternalTxID    *string `json:"external_tx_id,omitempty"`
	ExternalAddress *string `json:"external_address,omitempty"`
	LedgerOffset    int64   `json:"ledger_offset"`
}

// IndexerTokenPage is the paginated response for token list queries.
type IndexerTokenPage struct {
	Items []*IndexerToken `json:"items"`
	Total int64           `json:"total"`
	Page  int             `json:"page"`
	Limit int             `json:"limit"`
}

// IndexerBalancePage is the paginated response for balance list queries.
type IndexerBalancePage struct {
	Items []*IndexerBalance `json:"items"`
	Total int64             `json:"total"`
	Page  int               `json:"page"`
	Limit int               `json:"limit"`
}

// IndexerEventPage is the paginated response for event list queries.
type IndexerEventPage struct {
	Items []*IndexerEvent `json:"items"`
	Total int64           `json:"total"`
	Page  int             `json:"page"`
	Limit int             `json:"limit"`
}

// Preconfigured Anvil test accounts (deterministic from mnemonic).
var (
	AnvilAccount0 = &Account{
		Address:    common.HexToAddress("0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"),
		PrivateKey: "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80",
	}
	AnvilAccount1 = &Account{
		Address:    common.HexToAddress("0x70997970C51812dc3A010C7d01b50e0d17dc79C8"),
		PrivateKey: "59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d",
	}
)

type RegisterRequest struct {
	EVMAddress string
	Signature  string
	Message    string
}

type RegisterResponse struct {
	EVMAddress    string
	CantonPartyID string
	Fingerprint   string
	MappingCID    string
}

type TransferRequest struct {
	From      common.Address
	To        common.Address
	Amount    string
	TokenAddr string
}

type UserRow struct {
	EVMAddress    string
	CantonPartyID string
	Fingerprint   string
}
