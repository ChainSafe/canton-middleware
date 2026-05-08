//go:build e2e

package stack

import (
	"github.com/ethereum/go-ethereum/common"

	"github.com/chainsafe/canton-middleware/pkg/token"
)

// Token bundles an ERC-20 contract address with its ERC20Token metadata
// (symbol, decimals) for use in E2E test helpers such as WaitForAPIBalance.
type Token struct {
	token.ERC20Token
	Address common.Address
}

const (
	// DemoTokenVirtualAddr is the virtual EVM address of the DEMO Canton-native
	// token as recognized by the api-server's /eth JSON-RPC facade. It is a
	// well-known constant derived from the api-server's contract mapping — update
	// here if that mapping ever changes.
	DemoTokenVirtualAddr = "0xDE30000000000000000000000000000000000001"

	// USDCxTokenVirtualAddr is the well-known virtual EVM address mapped to the
	// USDCx external token in the api-server configuration.
	// #nosec G101 -- public deterministic EVM contract address, not a credential.
	USDCxTokenVirtualAddr = "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48"
)

// CantonHolding is a minimal view of a CIP56Holding contract used by E2E tests.
// It carries only the fields needed for bridge withdrawal operations.
type CantonHolding struct {
	ContractID string
	Amount     string
	Symbol     string
}

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
	AnvilAccount0 = Account{
		Address:    common.HexToAddress("0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"),
		PrivateKey: "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80",
	}
	AnvilAccount1 = Account{
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

	// APIDatabaseDSN is the connection string for the api-server database,
	// used only for whitelisting addresses during test setup
	// (e.g. "postgres://postgres:p@ssw0rd@localhost:5432/erc20_api").
	APIDatabaseDSN string

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

	// CantonDomainID is the synchronizer/domain ID of the Canton network,
	// resolved at discovery time from the Canton HTTP API.
	CantonDomainID string

	// DemoTokenAddr is the virtual EVM address of the DEMO Canton-native token
	// as recognized by the api-server's /eth JSON-RPC facade.
	// It is a well-known constant: 0xDE30000000000000000000000000000000000001.
	DemoTokenAddr string

	// Canton2GRPC is the Canton Ledger API gRPC endpoint for Participant 2
	// (the USDCx issuer participant), e.g. "localhost:5021".
	Canton2GRPC string

	// Canton2HTTP is the Canton HTTP JSON API endpoint for Participant 2
	// (e.g. "http://localhost:5023").
	Canton2HTTP string

	// USDCxInstrumentAdmin is the Canton party ID of the USDCx token issuer on
	// Participant 2.
	USDCxInstrumentAdmin string

	// USDCxInstrumentID is the instrument identifier of the USDCx token
	// (e.g. "USDCx").
	USDCxInstrumentID string

	// USDCxRegistryHTTP is the base URL of the usdcx-registry service
	// (e.g. "http://localhost:8090"). Used to look up the accept choice-context
	// when exercising TransferInstruction_Accept for USDCx transfers.
	USDCxRegistryHTTP string
}
