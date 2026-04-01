//go:build e2e

package stack

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

// ----------------------------------------------------------------------------
// Anvil — local Ethereum node
// ----------------------------------------------------------------------------

type Anvil interface {
	// RPC returns a connected go-ethereum client.
	RPC() *ethclient.Client
	// ChainID returns the Anvil chain ID (31337 for local).
	ChainID() *big.Int
	// Endpoint returns the http RPC URL (e.g. "http://localhost:8545").
	Endpoint() string
	// ERC20Balance returns the on-chain ERC-20 balance of address for tokenAddr.
	ERC20Balance(ctx context.Context, tokenAddr, owner common.Address) (*big.Int, error)
	// ApproveAndDeposit approves the bridge contract then deposits amount.
	ApproveAndDeposit(ctx context.Context, key *Account, amount *big.Int) (common.Hash, error)
}

// ----------------------------------------------------------------------------
// Canton — ledger node
// ----------------------------------------------------------------------------

type Canton interface {
	// GRPCEndpoint returns the gRPC endpoint (e.g. "localhost:5011").
	GRPCEndpoint() string
	// HTTPEndpoint returns the HTTP endpoint (e.g. "http://localhost:5013").
	HTTPEndpoint() string
	// IsHealthy returns true when Canton is ready to accept commands.
	IsHealthy(ctx context.Context) bool
}

// ----------------------------------------------------------------------------
// APIServer — the canton-middleware api-server
// ----------------------------------------------------------------------------

type APIServer interface {
	// Endpoint returns the base HTTP URL.
	Endpoint() string
	// Register registers an EVM account with the api-server.
	Register(ctx context.Context, req *RegisterRequest) (*RegisterResponse, error)
	// GetBalance calls eth_call to get token balance for address.
	GetBalance(ctx context.Context, tokenAddr, ownerAddr string) (string, error)
	// Transfer calls eth_sendTransaction to transfer tokens.
	Transfer(ctx context.Context, req *TransferRequest) (string, error)
	// Health returns nil when the api-server is ready.
	Health(ctx context.Context) error
}

// ----------------------------------------------------------------------------
// Relayer — bridge relayer
// ----------------------------------------------------------------------------

type Relayer interface {
	// Endpoint returns the base HTTP URL.
	Endpoint() string
	// Health returns nil when the relayer is ready.
	Health(ctx context.Context) error
	// IsReady returns true when both Canton and Ethereum streams are synced.
	IsReady(ctx context.Context) bool
}

// ----------------------------------------------------------------------------
// Indexer — Canton token transfer event indexer
// ----------------------------------------------------------------------------

type Indexer interface {
	// Endpoint returns the base HTTP URL (e.g. "http://localhost:8082").
	Endpoint() string
	// Health returns nil when the indexer is ready and synced.
	Health(ctx context.Context) error

	// GetToken returns token state (total supply, holder count) for {admin, id}.
	GetToken(ctx context.Context, admin, id string) (*IndexerToken, error)
	// TotalSupply returns the current total supply decimal string for {admin, id}.
	TotalSupply(ctx context.Context, admin, id string) (string, error)
	// ListTokens returns a paginated list of all indexed tokens.
	ListTokens(ctx context.Context, page, limit int) (*IndexerTokenPage, error)

	// GetBalance returns the current balance for a canton party and instrument.
	GetBalance(ctx context.Context, partyID, admin, id string) (*IndexerBalance, error)
	// ListBalancesForParty returns all balances held by partyID.
	ListBalancesForParty(ctx context.Context, partyID string, page, limit int) (*IndexerBalancePage, error)

	// GetEvent returns a single indexed event by its unique contract ID.
	GetEvent(ctx context.Context, contractID string) (*IndexerEvent, error)
	// ListPartyEvents returns events where partyID is sender or receiver.
	// eventType filters by "MINT", "BURN", "TRANSFER", or "" for all.
	ListPartyEvents(ctx context.Context, partyID, eventType string, page, limit int) (*IndexerEventPage, error)
	// ListTokenEvents returns events for a specific instrument.
	ListTokenEvents(ctx context.Context, admin, id, eventType string, page, limit int) (*IndexerEventPage, error)
}

// ----------------------------------------------------------------------------
// Postgres — database connection
// ----------------------------------------------------------------------------

type Postgres interface {
	// DSN returns the postgres connection string.
	DSN() string
	// WhitelistAddress adds an EVM address to the whitelist table.
	WhitelistAddress(ctx context.Context, evmAddress string) error
	// GetUserByEVMAddress returns a user row or nil.
	GetUserByEVMAddress(ctx context.Context, evmAddress string) (*UserRow, error)
}
