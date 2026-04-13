//go:build e2e

// Package stack defines the service interfaces and shared types for the E2E
// test framework. Every layer above this one (shim, system, dsl, presets)
// depends only on these interfaces — never on concrete implementations —
// so test code remains decoupled from network transport details.
package stack

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/chainsafe/canton-middleware/pkg/indexer"
	"github.com/chainsafe/canton-middleware/pkg/registry"
	"github.com/chainsafe/canton-middleware/pkg/relayer"
	"github.com/chainsafe/canton-middleware/pkg/transfer"
	"github.com/chainsafe/canton-middleware/pkg/user"
)

// Anvil is the interface for the local Anvil Ethereum node.
// It provides EVM interaction helpers used by deposit and balance checks.
type Anvil interface {
	// Endpoint returns the HTTP JSON-RPC URL (e.g. "http://localhost:8545").
	Endpoint() string

	// RPC returns a connected go-ethereum client for direct low-level calls.
	RPC() *ethclient.Client

	// ChainID returns the Anvil chain ID (31337 for local devnet).
	ChainID() *big.Int

	// ERC20Balance returns the on-chain ERC-20 balance of owner for the given
	// token contract address.
	ERC20Balance(ctx context.Context, tokenAddr, owner common.Address) (*big.Int, error)

	// ApproveAndDeposit approves the bridge contract to spend amount, then
	// submits the deposit transaction. Returns the transaction hash.
	ApproveAndDeposit(ctx context.Context, account *Account, amount *big.Int) (common.Hash, error)
}

// Canton is the interface for the Canton ledger node.
// It provides endpoint accessors, a liveness check, and token operations.
type Canton interface {
	// GRPCEndpoint returns the gRPC endpoint (e.g. "localhost:5011").
	GRPCEndpoint() string

	// HTTPEndpoint returns the Canton HTTP JSON API endpoint
	// (e.g. "http://localhost:5013").
	HTTPEndpoint() string

	// IsHealthy returns true when Canton has connected to the synchronizer
	// and is ready to accept commands.
	IsHealthy(ctx context.Context) bool

	// MintToken mints amount of tokenSymbol to recipientParty via the
	// IssuerMint DAML choice on the TokenConfig contract. Used by E2E tests
	// to seed DEMO balances before exercising the transfer API.
	MintToken(ctx context.Context, recipientParty, tokenSymbol, amount string) error

	// GetCantonBalance returns the total balance of tokenSymbol held by
	// partyID, expressed as a decimal string. Returns "0" when the party
	// has no holdings for that token.
	GetCantonBalance(ctx context.Context, partyID, tokenSymbol string) (string, error)

	// AllocateParty allocates a fresh internal Canton party with the given hint
	// and returns its fully-qualified party ID. Use this to create unique parties
	// per test without relying on manifest fixtures.
	AllocateParty(ctx context.Context, hint string) (string, error)
}

// APIServer is the interface for the canton-middleware api-server.
// It covers user registration (web3 custodial and non-custodial external
// modes), the two-step Canton transfer flow, ERC-20 balance queries through
// the Ethereum JSON-RPC facade, and the Splice registry endpoint.
//
// Request and response types are reused directly from the service packages
// (pkg/user, pkg/transfer) to avoid duplication and ensure the E2E layer
// always reflects the real API contract.
type APIServer interface {
	// Endpoint returns the base HTTP URL (e.g. "http://localhost:8081").
	Endpoint() string

	// RPC returns the go-ethereum ethclient connected to the api-server's
	// /eth JSON-RPC facade. Callers can use it for arbitrary eth_ calls
	// without going through the shim's typed methods.
	RPC() *ethclient.Client

	// Health returns nil when the api-server is ready to accept requests.
	Health(ctx context.Context) error

	// Register registers an EVM account in custodial web3 mode via
	// POST /register. req.Signature is an EIP-191 personal_sign signature;
	// req.Message is the plain-text string that was signed. The server
	// recovers the EVM address and allocates a Canton party.
	Register(ctx context.Context, req *user.RegisterRequest) (*user.RegisterResponse, error)

	// PrepareTopology is the first step of non-custodial (external key) user
	// registration via POST /register/prepare-topology. req.CantonPublicKey
	// is the hex-encoded compressed secp256k1 Canton public key. The response
	// carries the topology hash the client must sign and a short-lived
	// registration token.
	PrepareTopology(ctx context.Context, req *user.RegisterRequest) (*user.PrepareTopologyResponse, error)

	// RegisterExternal completes non-custodial registration via POST /register
	// with key_mode=external. req must include the RegistrationToken and
	// TopologySignature from PrepareTopology.
	RegisterExternal(ctx context.Context, req *user.RegisterRequest) (*user.RegisterResponse, error)

	// PrepareTransfer initiates a Canton token transfer via
	// POST /api/v2/transfer/prepare. account is used by the shim to produce
	// the required timed EIP-191 auth headers (X-Signature, X-Message);
	// req carries the transfer body (To, Amount, Token).
	// Returns the transaction hash the sender must sign and an opaque
	// transfer ID.
	PrepareTransfer(ctx context.Context, account *Account, req *transfer.PrepareRequest) (*transfer.PrepareResponse, error)

	// ExecuteTransfer completes a prepared transfer via
	// POST /api/v2/transfer/execute. account is used by the shim to produce
	// the timed EIP-191 auth headers; req carries the DER-encoded Canton
	// signature of the transaction hash from PrepareTransfer.
	ExecuteTransfer(ctx context.Context, account *Account, req *transfer.ExecuteRequest) (*transfer.ExecuteResponse, error)

	// ERC20Balance returns the ERC-20 balance of ownerAddr for tokenAddr by
	// calling balanceOf through the api-server's Ethereum JSON-RPC facade at
	// /eth. Uses go-ethereum's ethclient so the call exercises full
	// JSON-RPC compatibility of the facade, not just a single raw method.
	ERC20Balance(ctx context.Context, tokenAddr, ownerAddr common.Address) (*big.Int, error)

	// TransferFactory calls POST /registry/transfer-instruction/v1/transfer-factory
	// and returns the base64-encoded CreatedEventBlob used for Splice contract
	// discovery.
	TransferFactory(ctx context.Context) (*registry.TransferFactoryResponse, error)
}

// Relayer is the interface for the canton-bridge relayer service.
// It exposes health, readiness, and transfer query operations.
type Relayer interface {
	// Endpoint returns the base HTTP URL (e.g. "http://localhost:8080").
	Endpoint() string

	// Health returns nil when the relayer HTTP server is up.
	Health(ctx context.Context) error

	// IsReady returns true when the relayer engine has synced both the Canton
	// and Ethereum event streams and is actively processing transfers.
	IsReady(ctx context.Context) bool

	// ListTransfers returns all bridge transfers tracked by the relayer
	// (up to the server-side limit of 100).
	ListTransfers(ctx context.Context) ([]*relayer.Transfer, error)

	// GetTransfer returns a single transfer by its opaque ID.
	GetTransfer(ctx context.Context, id string) (*relayer.Transfer, error)

	// Status returns the relayer's reported status string (e.g. "running").
	Status(ctx context.Context) (string, error)
}

// Indexer is the interface for the Canton token-transfer event indexer.
// All methods target the unauthenticated admin read API at
// /indexer/v1/admin, which is intended for internal backend access only.
//
// Paginated list methods accept 1-based page numbers and a limit between
// 1 and 200 (server-enforced). Response types are reused directly from
// pkg/indexer.
type Indexer interface {
	// Endpoint returns the base HTTP URL (e.g. "http://localhost:8082").
	Endpoint() string

	// Health returns nil when the indexer is running and streaming from the
	// Canton ledger.
	Health(ctx context.Context) error

	// GetToken returns the current state of a token identified by its issuer
	// party (admin) and instrument ID (id).
	GetToken(ctx context.Context, admin, id string) (*indexer.Token, error)

	// TotalSupply returns the current total supply as a decimal string for
	// the token identified by admin and id.
	TotalSupply(ctx context.Context, admin, id string) (string, error)

	// ListTokens returns a paginated list of all tokens indexed so far.
	ListTokens(ctx context.Context, page, limit int) (*indexer.Page[*indexer.Token], error)

	// GetBalance returns the current holding of partyID for the token
	// identified by admin and id.
	GetBalance(ctx context.Context, partyID, admin, id string) (*indexer.Balance, error)

	// ListBalancesForParty returns all instrument balances held by partyID.
	ListBalancesForParty(ctx context.Context, partyID string, page, limit int) (*indexer.Page[*indexer.Balance], error)

	// GetBalanceForToken returns a paginated list of all party balances for
	// the token identified by admin and id.
	GetBalanceForToken(ctx context.Context, admin, id string, page, limit int) (*indexer.Page[*indexer.Balance], error)

	// GetEvent returns a single indexed event by its Canton contract ID.
	GetEvent(ctx context.Context, contractID string) (*indexer.ParsedEvent, error)

	// ListPartyEvents returns events in which partyID appears as sender or
	// receiver. eventType filters to indexer.EventMint, EventBurn,
	// EventTransfer, or "" for all types.
	ListPartyEvents(
		ctx context.Context,
		partyID string,
		eventType indexer.EventType,
		page, limit int,
	) (*indexer.Page[*indexer.ParsedEvent], error)

	// ListTokenEvents returns all events for the token identified by admin
	// and id. eventType filters to indexer.EventMint, EventBurn,
	// EventTransfer, or "" for all types.
	ListTokenEvents(
		ctx context.Context,
		admin, id string,
		eventType indexer.EventType,
		page, limit int,
	) (*indexer.Page[*indexer.ParsedEvent], error)
}

// APIDatabase is the interface for direct access to the api-server's database
// during E2E tests. It is used for setup (whitelisting test addresses) and
// assertions (verifying user records written by the api-server).
type APIDatabase interface {
	// DSN returns the api-server PostgreSQL connection string
	// (e.g. "postgres://postgres:p@ssw0rd@localhost:5432/erc20_api").
	DSN() string

	// WhitelistAddress inserts evmAddress into the whitelist table, granting
	// it permission to register with the api-server.
	WhitelistAddress(ctx context.Context, evmAddress string) error
}
