//go:build e2e

package stack

import "github.com/ethereum/go-ethereum/common"

// ---------------------------------------------------------------------------
// Test accounts
// ---------------------------------------------------------------------------

// Account represents an EVM test account used in E2E scenarios.
// It is passed to shim methods that need to produce EIP-191 signatures or
// submit Ethereum transactions on behalf of a test user.
type Account struct {
	// Address is the 20-byte EVM address derived from PrivateKey.
	Address common.Address

	// PrivateKey is the hex-encoded raw private key without a 0x prefix.
	PrivateKey string
}

// AnvilAccount0 and AnvilAccount1 are the first two deterministic accounts
// produced by Anvil from the standard test mnemonic
// "test test test … test junk". Their keys are publicly known and must
// never be used outside local dev environments.
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

// ---------------------------------------------------------------------------
// Service manifest
// ---------------------------------------------------------------------------

// ServiceManifest holds the localhost endpoints and contract addresses
// resolved by ServiceDiscovery after Docker Compose reports healthy. Tests
// never hard-code addresses; they always read from the manifest.
type ServiceManifest struct {
	// AnvilRPC is the Anvil HTTP JSON-RPC URL (e.g. "http://localhost:8545").
	AnvilRPC string

	// CantonGRPC is the Canton Ledger API gRPC endpoint (e.g. "localhost:5011").
	CantonGRPC string

	// CantonHTTP is the Canton HTTP JSON API endpoint (e.g. "http://localhost:5013").
	CantonHTTP string

	// APIHTTP is the api-server base URL (e.g. "http://localhost:8081").
	APIHTTP string

	// RelayerHTTP is the relayer base URL (e.g. "http://localhost:8080").
	RelayerHTTP string

	// IndexerHTTP is the indexer base URL (e.g. "http://localhost:8082").
	IndexerHTTP string

	// OAuthHTTP is the mock OAuth2 server base URL (e.g. "http://localhost:8088").
	OAuthHTTP string

	// PostgresDSN is the connection string for the shared PostgreSQL instance
	// (e.g. "postgres://postgres:p@ssw0rd@localhost:5432/erc20_api").
	PostgresDSN string

	// PromptTokenAddr is the address of the deployed PromptToken ERC-20
	// contract (e.g. "0x5FbDB2315678afecb367f032d93F642f64180aa3").
	PromptTokenAddr string

	// BridgeAddr is the address of the deployed CantonBridge contract.
	BridgeAddr string

	// PromptInstrumentAdmin is the Canton party ID of the PROMPT token admin,
	// used as the first key component for indexer queries.
	PromptInstrumentAdmin string

	// PromptInstrumentID is the instrument identifier of the PROMPT token
	// (e.g. "PROMPT"), matching InstrumentKey.ID in the indexer config.
	PromptInstrumentID string

	// DemoInstrumentAdmin is the Canton party ID of the DEMO token admin.
	DemoInstrumentAdmin string

	// DemoInstrumentID is the instrument identifier of the DEMO token
	// (e.g. "DEMO").
	DemoInstrumentID string
}

// ---------------------------------------------------------------------------
// Registry types
//
// TransferFactoryResponse and TransferFactoryTemplateID have no equivalent in
// the service packages and are defined here. JSON tags are required because
// the shim decodes HTTP response bodies directly into these structs.
// ---------------------------------------------------------------------------

// TransferFactoryResponse is the response body from
// POST /registry/transfer-instruction/v1/transfer-factory.
type TransferFactoryResponse struct {
	// ContractID is the DAML contract ID of the transfer factory.
	ContractID string `json:"contract_id"`

	// CreatedEventBlob is the base64-encoded serialised CreatedEvent, used
	// for Splice contract discovery.
	CreatedEventBlob string `json:"created_event_blob"`

	// TemplateID identifies the DAML template backing the factory contract.
	TemplateID TransferFactoryTemplateID `json:"template_id"`
}

// TransferFactoryTemplateID is the fully qualified DAML template identifier
// embedded in TransferFactoryResponse.
type TransferFactoryTemplateID struct {
	PackageID  string `json:"package_id"`
	ModuleName string `json:"module_name"`
	EntityName string `json:"entity_name"`
}

// ---------------------------------------------------------------------------
// Postgres assertion types
//
// UserRow is a lightweight read-only view of the api-server's users table.
// pkg/userstore.UserDao is not reused here because it embeds bun.BaseModel,
// which would pull the ORM and database drivers into the test package.
// ---------------------------------------------------------------------------

// UserRow represents a row from the api-server's users table, used for
// post-registration assertions in E2E tests.
type UserRow struct {
	// EVMAddress is the 0x-prefixed Ethereum address, the primary lookup key.
	EVMAddress string

	// CantonPartyID is the allocated Canton party ID.
	CantonPartyID string

	// Fingerprint is the keccak256 hash of the EVM address bytes (without the
	// 0x1220 prefix), used as the bytes32 canton_recipient in bridge deposits.
	Fingerprint string

	// KeyMode is "custodial" or "external".
	KeyMode string
}
